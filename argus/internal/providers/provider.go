package providers

import "context"

// Provider is the interface every LLM adapter must implement.
// Designed to be minimal — each provider translates its native API
// into this common event stream.
type Provider interface {
	// Name returns the provider identifier ("anthropic", "openai_compat")
	Name() string

	// ModelID returns the model string sent to the API
	ModelID() string

	// Complete sends a conversation to the LLM and returns a stream of events.
	// The caller reads from the channel until it is closed.
	// On error, an Event with Type="error" is sent before closing.
	Complete(ctx context.Context, req CompletionRequest) (<-chan Event, error)

	// MaxContextTokens returns the model's context window size
	MaxContextTokens() int
}

// CompletionRequest is the provider-agnostic request format.
type CompletionRequest struct {
	SystemPrompt string
	Messages     []Message
	Tools        []ToolDefinition // tool schemas from executor.Schemas()
	MaxTokens    int
}

// Message is a single turn in the conversation.
type Message struct {
	Role    string // "user" | "assistant"
	Content []Block
}

// Block is one content item within a message.
type Block struct {
	Type       string      // "text" | "tool_call" | "tool_result"
	Text       string      // for type="text"
	ToolCall   *ToolCall   // for type="tool_call"
	ToolResult *ToolResult // for type="tool_result"
}

// ToolCall represents a tool invocation requested by the LLM.
type ToolCall struct {
	ID     string
	Name   string
	Params map[string]any
}

// ToolResult is the result of a tool execution, attached to a message.
type ToolResult struct {
	ToolCallID string
	Content    string
	IsError    bool
}

// ToolDefinition is the schema sent to the LLM so it knows how to call each tool.
// Populated directly from tools.ToolSchema.
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any // JSON Schema object
}

// Event is one item in the completion stream.
type Event struct {
	Type     string    // "text_delta" | "tool_call" | "done" | "error"
	Text     string    // for type="text_delta"
	ToolCall *ToolCall // for type="tool_call"
	Error    string    // for type="error"
	Usage    *Usage    // for type="done"
}

// Usage contains token consumption for the completed request.
type Usage struct {
	InputTokens  int
	OutputTokens int
	CostUSD      float64
}
