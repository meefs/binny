package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"openbin.ai/binny/internal/config"
)

// A gateway that fronts the Messages API (anthropic.<yourhost>) must receive
// the same path and key header the real API gets, just on its own host.
func TestAnthropicAdapterHonoursCustomEndpoint(t *testing.T) {
	var gotPath, gotKey, gotVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":3}}}\n\n" +
			"data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\"}}\n\n" +
			"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"pong\"}}\n\n" +
			"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\n" +
			"data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer srv.Close()

	p, err := New(config.Provider{ID: "anthropic", Kind: config.KindAnthropic, BaseURL: srv.URL + "/", APIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := p.Stream(context.Background(), Request{Model: "claude-x", MaxTokens: 16, Messages: []Message{UserText("ping")}}, nil)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if gotPath != "/v1/messages" {
		t.Errorf("path = %q, want /v1/messages", gotPath)
	}
	if gotKey != "sk-test" || gotVersion == "" {
		t.Errorf("headers: x-api-key=%q anthropic-version=%q", gotKey, gotVersion)
	}
	if resp.Text() != "pong" {
		t.Errorf("text = %q", resp.Text())
	}
}

// Same for an OpenAI-compatible router: the root keeps its /v1 and the key
// travels as a Bearer token.
func TestOpenAIAdapterHonoursCustomEndpoint(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"pong\"},\"finish_reason\":null}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
			"data: [DONE]\n\n"))
	}))
	defer srv.Close()

	p, err := New(config.Provider{ID: "router", Kind: config.KindOpenAI, BaseURL: srv.URL + "/v1", APIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := p.Stream(context.Background(), Request{Model: "gpt-x", MaxTokens: 16, Messages: []Message{UserText("ping")}}, nil)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if gotPath != "/v1/chat/completions" {
		t.Errorf("path = %q, want /v1/chat/completions", gotPath)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("auth = %q", gotAuth)
	}
	if resp.Text() != "pong" {
		t.Errorf("text = %q", resp.Text())
	}
}
