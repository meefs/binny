package llm

import (
	"strings"
	"testing"
)

// TestParseOpenAIUsageCacheFields verifies BINNY reads the cache-hit tokens each
// compatible provider reports under its own field name, and bills only the
// UNCACHED remainder — so auto-cached providers (Kimi, DeepSeek, OpenAI, …) get
// the same budget benefit as Anthropic.
func TestParseOpenAIUsageCacheFields(t *testing.T) {
	cases := []struct {
		name         string
		usageJSON    string
		wantInput    int // uncached (billed) input
		wantCacheHit int
	}{
		{
			name:         "openai/kimi prompt_tokens_details",
			usageJSON:    `{"prompt_tokens":1000,"completion_tokens":50,"prompt_tokens_details":{"cached_tokens":800}}`,
			wantInput:    200,
			wantCacheHit: 800,
		},
		{
			name:         "deepseek hit/miss fields",
			usageJSON:    `{"prompt_tokens":1000,"completion_tokens":50,"prompt_cache_hit_tokens":900,"prompt_cache_miss_tokens":100}`,
			wantInput:    100,
			wantCacheHit: 900,
		},
		{
			name:         "top-level cached_tokens fallback",
			usageJSON:    `{"prompt_tokens":1000,"completion_tokens":50,"cached_tokens":300}`,
			wantInput:    700,
			wantCacheHit: 300,
		},
		{
			name:         "no cache (provider without caching)",
			usageJSON:    `{"prompt_tokens":1000,"completion_tokens":50}`,
			wantInput:    1000,
			wantCacheHit: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sse := "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}],\"usage\":" +
				tc.usageJSON + "}\n\ndata: [DONE]\n\n"
			resp, err := parseOpenAISSE(strings.NewReader(sse), "test-model", nil)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if resp.Usage.InputTokens != tc.wantInput {
				t.Errorf("InputTokens = %d, want %d", resp.Usage.InputTokens, tc.wantInput)
			}
			if resp.Usage.CacheReadTokens != tc.wantCacheHit {
				t.Errorf("CacheReadTokens = %d, want %d", resp.Usage.CacheReadTokens, tc.wantCacheHit)
			}
		})
	}
}

// TestBuildAntReqCacheBreakpoints locks in the three prompt-cache breakpoints —
// most importantly the one on the last message, which is what stops a long agent
// loop from re-billing the whole conversation every round.
func TestBuildAntReqCacheBreakpoints(t *testing.T) {
	req := Request{
		System:   "you are BINNY",
		Model:    "claude-fable-5",
		Messages: sampleConversation(),
		Tools: []Tool{
			{Name: "list_functions", InputSchema: map[string]any{"type": "object"}},
			{Name: "get_strings", InputSchema: map[string]any{"type": "object"}},
		},
	}
	body := buildAntReq(req)

	// System prompt cached.
	if len(body.System) != 1 || body.System[0].CacheControl == nil {
		t.Error("system prompt should carry a cache breakpoint")
	}
	// Last tool cached; earlier tools not (one breakpoint covers the whole list).
	if body.Tools[len(body.Tools)-1].CacheControl == nil {
		t.Error("last tool should carry a cache breakpoint")
	}
	if body.Tools[0].CacheControl != nil {
		t.Error("only the LAST tool should carry the breakpoint, not every tool")
	}
	// The conversation breakpoint sits on the last block of the last message.
	last := body.Messages[len(body.Messages)-1]
	lastBlock := last.Content[len(last.Content)-1]
	if lastBlock.CacheControl == nil {
		t.Error("last message block should carry a cache breakpoint (conversation prefix cache)")
	}
	// Total breakpoints must stay within Anthropic's limit of 4.
	if got := countBreakpoints(body); got > 4 {
		t.Errorf("placed %d cache breakpoints, Anthropic allows at most 4", got)
	}
}

func TestBuildAntReqNoToolsNoMessages(t *testing.T) {
	// Degenerate request must not panic (empty tools/messages).
	body := buildAntReq(Request{System: "s", Model: "m"})
	if len(body.System) != 1 || body.System[0].CacheControl == nil {
		t.Error("system breakpoint expected even with no tools/messages")
	}
}

func countBreakpoints(b antReq) int {
	n := 0
	for _, s := range b.System {
		if s.CacheControl != nil {
			n++
		}
	}
	for _, t := range b.Tools {
		if t.CacheControl != nil {
			n++
		}
	}
	for _, m := range b.Messages {
		for _, blk := range m.Content {
			if blk.CacheControl != nil {
				n++
			}
		}
	}
	return n
}
