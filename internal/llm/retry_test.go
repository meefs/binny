package llm

import (
	"net/http"
	"testing"
	"time"
)

func TestRetryableStatus(t *testing.T) {
	for _, code := range []int{429, 500, 502, 503, 529} {
		if !retryableStatus(code) {
			t.Errorf("status %d should be retryable", code)
		}
	}
	for _, code := range []int{200, 400, 401, 403, 404} {
		if retryableStatus(code) {
			t.Errorf("status %d should NOT be retryable", code)
		}
	}
}

func TestRetryDelayHonorsBodyHint(t *testing.T) {
	// OpenAI TPM limits put the delay in the message, not the header.
	body := []byte(`{"error":{"message":"Rate limit reached … Please try again in 8.762s.","code":"rate_limit_exceeded"}}`)
	got := retryDelay(http.Header{}, body, 1)
	// 8.762 + 0.5 rounding ≈ 9.26s.
	if got < 9*time.Second || got > 10*time.Second {
		t.Fatalf("expected ~9.3s from body hint, got %s", got)
	}
}

func TestRetryDelayHonorsRetryAfterHeader(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "12")
	if got := retryDelay(h, nil, 1); got != 12*time.Second {
		t.Fatalf("expected 12s from Retry-After, got %s", got)
	}
}

func TestRetryDelayExponentialFallback(t *testing.T) {
	// No hint anywhere → exponential fallback, capped.
	if got := retryDelay(http.Header{}, nil, 2); got != 4*time.Second {
		t.Fatalf("attempt 2 fallback: expected 4s, got %s", got)
	}
	if got := retryDelay(http.Header{}, nil, 10); got != maxBackoff {
		t.Fatalf("large attempt should clamp to %s, got %s", maxBackoff, got)
	}
}
