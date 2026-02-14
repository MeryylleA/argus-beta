package agent

import (
	"time"

	"github.com/argus-beta/argus/internal/providers"
)

// RunEvent is emitted by the runner during execution.
// The AgentName field distinguishes events from different agents in collaborative mode.
type RunEvent struct {
	AgentName string             // "single" | "agent_a" | "agent_b"
	Type      string             // see constants below
	Text      string             // for "text", "error", "done", "budget_exceeded"
	ToolName  string             // for "tool_call" and "tool_result"
	ToolCall  *providers.ToolCall // for "tool_call"
	IsError   bool               // for "tool_result"
	Timestamp time.Time
}

const (
	EventText            = "text"
	EventToolCall        = "tool_call"
	EventToolResult      = "tool_result"
	EventFindingRecorded = "finding_recorded"
	EventDone            = "done"
	EventError           = "error"
	EventBudgetExceeded  = "budget_exceeded"
)
