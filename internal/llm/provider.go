package llm

import (
	"fmt"
	"net/http"
	"time"

	"openbin.ai/binny/internal/config"
)

// New builds a Provider from a configured provider entry. The key is resolved
// (BYOK) via Provider.Key() — a stored key, else the env var — at construction
// time; Bedrock uses the ambient AWS credential chain instead.
func New(p config.Provider) (Provider, error) {
	switch p.Kind {
	case config.KindOpenAI:
		key := p.Key()
		if key == "" {
			return nil, fmt.Errorf("provider %q has no key — set one with /key (BYOK)", p.ID)
		}
		return &openAIProvider{id: p.ID, baseURL: p.BaseURL, apiKey: key, http: sharedClient()}, nil
	case config.KindAnthropic:
		key := p.Key()
		if key == "" {
			return nil, fmt.Errorf("provider %q has no key — set one with /key (BYOK)", p.ID)
		}
		return &anthropicProvider{id: p.ID, baseURL: p.BaseURL, apiKey: key, http: sharedClient()}, nil
	case config.KindBedrock:
		return &bedrockProvider{id: p.ID, region: p.Region}, nil
	default:
		return nil, fmt.Errorf("unknown provider kind %q", p.Kind)
	}
}

// sharedClient returns an HTTP client with a generous timeout. Agent turns are
// bounded by MaxTokens so a single call returns well within this window; the
// long ceiling only protects against a slow network, not a runaway model.
func sharedClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Minute}
}
