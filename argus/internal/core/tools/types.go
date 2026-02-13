package tools

import "time"

// ToolSchema represents the JSON schema sent to LLM providers
// so they know how to call each tool. This is provider-agnostic;
// each provider adapter will marshal it into the appropriate format
// (Anthropic's tool_use, OpenAI's function calling, etc.)
type ToolSchema struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Parameters  map[string]ParamDef `json:"parameters"`
	Required    []string            `json:"required"`
}

// ParamDef defines a single parameter for a tool.
type ParamDef struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ToolResult encapsulates the output of every tool execution.
// IsError signals to the agent that the result is an error message
// rather than valid content — the agent can decide to retry or adjust.
type ToolResult struct {
	Content  string         `json:"content"`
	IsError  bool           `json:"is_error"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// ToolLogEntry captures every tool invocation for audit trail.
// This is critical for a security platform — we need full traceability
// of what was accessed, when, and what was returned.
type ToolLogEntry struct {
	Timestamp  time.Time      `json:"timestamp"`
	ToolName   string         `json:"tool_name"`
	Params     map[string]any `json:"params"`
	Result     ToolResult     `json:"result"`
	DurationMs int64          `json:"duration_ms"`
}

// Tool is the interface every tool must implement.
// The design is intentionally simple — each tool is a self-contained unit
// that knows how to describe itself (for LLM consumption) and execute.
type Tool interface {
	// Name returns the tool identifier used in LLM tool calls
	Name() string

	// Description returns a human/LLM-readable description of what the tool does
	Description() string

	// Schema returns the full schema definition sent to the LLM provider
	Schema() ToolSchema

	// Execute runs the tool with the given parameters.
	// The executor wraps this with timeout and logging — tools themselves
	// should focus only on their core logic.
	Execute(params map[string]any) (ToolResult, error)
}
