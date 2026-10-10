package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"openbin.ai/binny/internal/config"
)

// Live model discovery — the thing that lets BINNY stop shipping a hardcoded
// model allowlist that goes stale every other day (mirrors the backend's
// LlmModelCatalog). Anthropic and every OpenAI-compatible provider expose a
// GET /models endpoint; we query it with the user's BYOK key and cache the
// result briefly. The setup model picker populates from this.
//
// Bedrock is the exception: its control-plane list doesn't reflect per-account
// model access, so we serve the curated set from config.
//
// Failures never block: an empty/errored fetch falls back to the provider's
// configured Models, so a flaky /models call never breaks provider selection.

const modelCacheTTL = 30 * time.Minute

type cachedModels struct {
	models    []string
	fetchedAt time.Time
}

var (
	modelCacheMu sync.Mutex
	modelCache   = map[string]cachedModels{}
)

// ListModels returns the model ids selectable for a provider. It prefers a live
// GET /models fetch (cached for modelCacheTTL); on empty/failure it falls back
// to the provider's configured Models. The bool reports whether the returned
// list came from a live fetch (true) or the static config (false), so the UI
// can show a "live" vs "configured" hint.
func ListModels(ctx context.Context, p config.Provider) ([]string, bool) {
	if p.Kind == config.KindBedrock {
		return p.Models, false // curated; no queryable per-account list
	}
	if p.Key() == "" {
		return p.Models, false // can't query without a key
	}

	key := cacheKey(p)
	modelCacheMu.Lock()
	if c, ok := modelCache[key]; ok && time.Since(c.fetchedAt) < modelCacheTTL {
		modelCacheMu.Unlock()
		return c.models, true
	}
	modelCacheMu.Unlock()

	models := fetchModels(ctx, p)
	if len(models) == 0 {
		return p.Models, false // don't cache a transient empty; fall back
	}
	modelCacheMu.Lock()
	modelCache[key] = cachedModels{models: models, fetchedAt: time.Now()}
	modelCacheMu.Unlock()
	return models, true
}

func cacheKey(p config.Provider) string { return string(p.Kind) + "|" + p.BaseURL + "|" + p.Key() }

func fetchModels(ctx context.Context, p config.Provider) []string {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	base := strings.TrimRight(p.BaseURL, "/")
	var url string
	req, err := func() (*http.Request, error) {
		switch p.Kind {
		case config.KindAnthropic:
			url = base + "/v1/models" // base is the bare host (no /v1)
		default: // OpenAI-compatible: base already carries /v1
			url = base + "/models"
		}
		return http.NewRequestWithContext(ctx, "GET", url, nil)
	}()
	if err != nil {
		return nil
	}
	switch p.Kind {
	case config.KindAnthropic:
		req.Header.Set("x-api-key", p.Key())
		req.Header.Set("anthropic-version", "2023-06-01")
	default:
		req.Header.Set("Authorization", "Bearer "+p.Key())
	}

	resp, err := sharedClient().Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil
	}
	return parseModelList(body)
}

// parseModelList reads the {"data":[{"id":...}]} shape shared by Anthropic and
// OpenAI-compatible /models responses.
func parseModelList(body []byte) []string {
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(payload.Data))
	for _, m := range payload.Data {
		if m.ID == "" || seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		out = append(out, m.ID)
	}
	return out
}
