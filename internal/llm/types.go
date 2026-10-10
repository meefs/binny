// Package llm is BINNY's provider-agnostic LLM layer. The agent loop speaks
// ONE internal message + tool-call representation; each adapter (OpenAI-compat,
// Anthropic-native, Bedrock) only translates to/from its own wire schema. This
// is the design crux: adding a provider means adding a translator, never
// touching the loop.
package llm

import (
	"context"
	"encoding/json"
)

// Role is the author of a message in the internal representation.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// BlockKind discriminates the content-block union.
type BlockKind string

const (
	BlockText       BlockKind = "text"        // plain text
	BlockToolUse    BlockKind = "tool_use"    // assistant asks to call a tool
	BlockToolResult BlockKind = "tool_result" // user returns a tool's output
)

// Block is one content block. Only the fields relevant to Kind are populated.
type Block struct {
	Kind BlockKind

	// BlockText
	Text string

	// BlockToolUse (assistant → us)
	ToolID   string          // provider-assigned call id (echoed back in the result)
	ToolName string          // which tool to run
	Input    json.RawMessage // tool arguments as raw JSON

	// BlockToolResult (us → assistant)
	ToolUseID string // the ToolID this result answers
	Content   string // stringified tool output
	IsError   bool   // true if the tool failed
}

// TextBlock is a convenience constructor.
func TextBlock(s string) Block { return Block{Kind: BlockText, Text: s} }

// ToolResult is a convenience constructor for returning a tool's output.
func ToolResult(toolUseID, content string, isErr bool) Block {
	return Block{Kind: BlockToolResult, ToolUseID: toolUseID, Content: content, IsError: isErr}
}

// Message is one turn in the conversation.
type Message struct {
	Role   Role
	Blocks []Block
}

// UserText builds a simple user text message.
func UserText(s string) Message {
	return Message{Role: RoleUser, Blocks: []Block{TextBlock(s)}}
}

// Tool is an LLM-callable tool definition. InputSchema is a JSON Schema object.
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// ToolCall is a normalized tool invocation extracted from a response.
type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// Usage reports token accounting for budget tracking. With prompt caching,
// InputTokens counts only the UNCACHED (freshly-billed) input; CacheReadTokens
// is the prefix served from cache (~0.1x price) and CacheCreationTokens is what
// was written to cache this turn (~1.25x price). Providers without caching
// report everything in InputTokens and leave the cache fields at zero.
type Usage struct {
	InputTokens         int
	OutputTokens        int
	CacheReadTokens     int
	CacheCreationTokens int
}

// Request is a provider-agnostic completion request.
type Request struct {
	System    string    // system prompt
	Messages  []Message // conversation so far
	Tools     []Tool    // tools the model may call
	Model     string    // provider-specific model id
	MaxTokens int       // output cap for this turn
}

// StopReason normalizes why generation stopped.
type StopReason string

const (
	StopEndTurn   StopReason = "end_turn"   // model finished its turn
	StopToolUse   StopReason = "tool_use"   // model wants tool results
	StopMaxTokens StopReason = "max_tokens" // hit the output cap
	StopRefusal   StopReason = "refusal"    // safety refusal (handle before reading content)
	StopOther     StopReason = "other"
)

// Response is a provider-agnostic completion response. Blocks holds the
// assistant's output verbatim (text + tool_use) so the loop can append it to
// history unchanged before returning tool results.
type Response struct {
	Blocks     []Block
	StopReason StopReason
	Usage      Usage
	Model      string
}

// Text concatenates all text blocks in the response.
func (r *Response) Text() string {
	var s string
	for _, b := range r.Blocks {
		if b.Kind == BlockText {
			s += b.Text
		}
	}
	return s
}

// ToolCalls extracts the tool_use blocks as normalized ToolCalls.
func (r *Response) ToolCalls() []ToolCall {
	var calls []ToolCall
	for _, b := range r.Blocks {
		if b.Kind == BlockToolUse {
			calls = append(calls, ToolCall{ID: b.ToolID, Name: b.ToolName, Input: b.Input})
		}
	}
	return calls
}

// AsMessage wraps the response's blocks as an assistant message for history.
func (r *Response) AsMessage() Message {
	return Message{Role: RoleAssistant, Blocks: r.Blocks}
}

// Provider is one configured LLM backend. Stream runs a single turn: given the
// conversation + tools, it streams assistant TEXT deltas to onText as they
// arrive (for live rendering) and returns the fully-accumulated response (text
// + tool_use blocks) when the turn ends. onText may be nil. The agent loop
// calls this repeatedly, feeding tool results back.
type Provider interface {
	// Name is the provider id (for display / logs).
	Name() string
	// Stream runs one turn, emitting text deltas to onText (if non-nil).
	Stream(ctx context.Context, req Request, onText func(string)) (*Response, error)
}
