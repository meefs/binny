package llm

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

// Transient provider failures — rate limits (429) and overload (500/502/503/529)
// — arrive as the *initial* HTTP status, before any SSE bytes are emitted. So we
// can retry the POST with backoff safely: nothing has streamed yet. Once a 200
// starts streaming we never retry (a mid-stream failure surfaces normally).
//
// This is what turns OpenAI's "Rate limit reached … try again in 8.762s" from a
// hard stop into an invisible pause — the spinner keeps turning and the run
// continues once the provider's window clears.

const (
	maxAttempts  = 6
	maxBackoff   = 60 * time.Second
	drainCeiling = 64 << 10 // bytes read from a retryable error body to find a hint
)

func retryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests, // 429
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		529:                            // Anthropic "overloaded"
		return true
	}
	return false
}

// postSSE issues a streaming POST with bounded retry+backoff on transient
// provider errors. It rebuilds the request each attempt (fresh body reader) and
// honors the provider's retry hint (Retry-After header or the "try again in Xs"
// in the error body). It returns the first 200 response, or — once retries are
// exhausted or the status isn't retryable — the last response for the caller to
// format as an error. A ctx cancellation (Esc interrupt) stops the wait.
func postSSE(ctx context.Context, client *http.Client, url string, headers map[string]string, body []byte) (*http.Response, error) {
	for attempt := 1; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == 200 || !retryableStatus(resp.StatusCode) || attempt >= maxAttempts {
			return resp, nil // success, non-retryable error, or out of attempts
		}

		// Retryable: read the hint out of the body, close it, back off, retry.
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, drainCeiling))
		resp.Body.Close()
		wait := retryDelay(resp.Header, errBody, attempt)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
}

var tryAgainRe = regexp.MustCompile(`try again in ([0-9.]+)\s*s`)

// retryDelay picks a backoff: the provider's explicit hint if present, else an
// exponential fallback, both capped at maxBackoff.
func retryDelay(h http.Header, body []byte, attempt int) time.Duration {
	if d := parseRetryAfter(h.Get("Retry-After")); d > 0 {
		return clampWait(d)
	}
	if m := tryAgainRe.FindSubmatch(body); m != nil {
		if secs, err := strconv.ParseFloat(string(m[1]), 64); err == nil && secs > 0 {
			// Round up a hair so we clear the window rather than skim its edge.
			return clampWait(time.Duration((secs + 0.5) * float64(time.Second)))
		}
	}
	// Exponential fallback: 2s, 4s, 8s, …
	return clampWait(time.Duration(1<<attempt) * time.Second)
}

func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

func clampWait(d time.Duration) time.Duration {
	if d > maxBackoff {
		return maxBackoff
	}
	if d < time.Second {
		return time.Second
	}
	return d
}
