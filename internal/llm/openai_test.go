package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"openbin.ai/binny/internal/config"
)

func TestOpenAIAdapterSurfacesStreamErrors(t *testing.T) {
	for _, tt := range []struct {
		name     string
		prefix   string
		error    string
		wantText string
	}{
		{
			name:  "before output",
			error: `{"error":{"message":"provider unavailable","code":503}}`,
		},
		{
			name:     "after text",
			prefix:   "data: " + `{"choices":[{"delta":{"content":"partial answer"}}]}` + "\n\n",
			error:    `{"error":{"message":"provider unavailable","code":"server_error"},"choices":[{"index":0,"delta":{"content":""},"finish_reason":"error"}]}`,
			wantText: "partial answer",
		},
		{
			name:   "after tool call",
			prefix: "data: " + `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"get_strings","arguments":"{}"}}]}}]}` + "\n\n",
			error:  `{"error":{"message":"provider unavailable","type":"server_error"}}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("Content-Type", "text/event-stream")
				w.Write([]byte(tt.prefix + "data: " + tt.error + "\n\n" +
					"data: [DONE]\n\n"))
			}))
			defer srv.Close()

			p, err := New(config.Provider{ID: "router", Kind: config.KindOpenAI, BaseURL: srv.URL + "/v1", APIKey: "test-key"})
			if err != nil {
				t.Fatal(err)
			}
			var streamed strings.Builder
			resp, err := p.Stream(context.Background(), Request{Model: "test-model", MaxTokens: 16, Messages: []Message{UserText("hello")}}, func(s string) {
				streamed.WriteString(s)
			})
			if err == nil || !strings.Contains(err.Error(), "provider unavailable") {
				t.Fatalf("want provider error, got response=%+v err=%v", resp, err)
			}
			if resp != nil {
				t.Fatalf("failed stream must not return a usable response: %+v", resp)
			}
			if got := streamed.String(); got != tt.wantText {
				t.Errorf("streamed text = %q, want %q", got, tt.wantText)
			}
			if requests != 1 {
				t.Errorf("stream error was retried: %d requests", requests)
			}
		})
	}
}
