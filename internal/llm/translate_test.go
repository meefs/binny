package llm

import (
	"encoding/json"
	"testing"
)

// A representative conversation: user asks, assistant calls a tool, we return
// the result. Both adapters must translate this faithfully to their wire shape.
func sampleConversation() []Message {
	return []Message{
		UserText("list the functions"),
		{Role: RoleAssistant, Blocks: []Block{
			{Kind: BlockText, Text: "Let me look."},
			{Kind: BlockToolUse, ToolID: "call_1", ToolName: "list_functions", Input: json.RawMessage(`{"filter":"init"}`)},
		}},
		{Role: RoleUser, Blocks: []Block{
			ToolResult("call_1", "main, init_config", false),
		}},
	}
}

func TestToAntMessages(t *testing.T) {
	got := toAntMessages(sampleConversation())
	if len(got) != 3 {
		t.Fatalf("want 3 messages, got %d", len(got))
	}
	// assistant turn should carry a text block + a tool_use block
	asst := got[1]
	if asst.Role != "assistant" || len(asst.Content) != 2 {
		t.Fatalf("assistant turn wrong: %+v", asst)
	}
	if asst.Content[1].Type != "tool_use" || asst.Content[1].ID != "call_1" {
		t.Errorf("tool_use block wrong: %+v", asst.Content[1])
	}
	// user turn returning the result should be a tool_result block
	tr := got[2].Content[0]
	if tr.Type != "tool_result" || tr.ToolUseID != "call_1" || tr.Content != "main, init_config" {
		t.Errorf("tool_result block wrong: %+v", tr)
	}
}

func TestToOAIMessages(t *testing.T) {
	got := toOAIMessages("you are BINNY", sampleConversation())
	// Expect: system, user, assistant(with tool_calls), tool
	if got[0].Role != "system" || got[0].Content != "you are BINNY" {
		t.Fatalf("system message wrong: %+v", got[0])
	}
	// find the assistant message with tool_calls
	var asst *oaiMessage
	var toolMsg *oaiMessage
	for i := range got {
		switch got[i].Role {
		case "assistant":
			asst = &got[i]
		case "tool":
			toolMsg = &got[i]
		}
	}
	if asst == nil || len(asst.ToolCalls) != 1 {
		t.Fatalf("assistant tool_calls missing: %+v", got)
	}
	if asst.ToolCalls[0].ID != "call_1" || asst.ToolCalls[0].Function.Name != "list_functions" {
		t.Errorf("tool call wrong: %+v", asst.ToolCalls[0])
	}
	if asst.ToolCalls[0].Function.Arguments != `{"filter":"init"}` {
		t.Errorf("tool args wrong: %q", asst.ToolCalls[0].Function.Arguments)
	}
	if toolMsg == nil || toolMsg.ToolCallID != "call_1" || toolMsg.Content != "main, init_config" {
		t.Errorf("tool result message wrong: %+v", toolMsg)
	}
}

func TestResponseHelpers(t *testing.T) {
	r := &Response{Blocks: []Block{
		{Kind: BlockText, Text: "found "},
		{Kind: BlockText, Text: "two"},
		{Kind: BlockToolUse, ToolID: "c1", ToolName: "get_strings", Input: json.RawMessage(`{}`)},
	}}
	if r.Text() != "found two" {
		t.Errorf("Text() = %q", r.Text())
	}
	calls := r.ToolCalls()
	if len(calls) != 1 || calls[0].Name != "get_strings" {
		t.Errorf("ToolCalls() = %+v", calls)
	}
}

func TestOrEmptyObject(t *testing.T) {
	if orEmptyObject("") != "{}" || orEmptyObject("   ") != "{}" {
		t.Error("empty args should become {}")
	}
	if orEmptyObject(`{"a":1}`) != `{"a":1}` {
		t.Error("non-empty args should pass through")
	}
}
