package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/argus-beta/argus/internal/core/channel"
	"github.com/argus-beta/argus/internal/core/tools"
	"github.com/argus-beta/argus/internal/memory"
	"github.com/argus-beta/argus/internal/providers"
)

// RunnerConfig configures one agent instance.
type RunnerConfig struct {
	SessionID string
	ProjectID string
	AgentName string // "single" | "agent_a" | "agent_b"
	Provider  providers.Provider
	Executor  *tools.ToolExecutor
	Store     memory.Store
	Prompt    PromptConfig
	Budget    Budget
	Channel   *channel.Channel // nil in single mode
	Verbose   bool
}

// Runner executes the agent loop for one agent.
type Runner struct {
	cfg       RunnerConfig
	budget    *BudgetTracker
	provider  providers.Provider
	executor  *tools.ToolExecutor
	store     memory.Store
	channel   *channel.Channel
	promptCfg PromptConfig
}

// NewRunner creates a runner from configuration.
func NewRunner(cfg RunnerConfig) *Runner {
	return &Runner{
		cfg:       cfg,
		budget:    NewBudgetTracker(cfg.Budget),
		provider:  cfg.Provider,
		executor:  cfg.Executor,
		store:     cfg.Store,
		channel:   cfg.Channel,
		promptCfg: cfg.Prompt,
	}
}

// Run starts the agent loop and blocks until completion.
// Returns a channel of RunEvents. The channel is closed when the agent finishes.
func (r *Runner) Run(ctx context.Context) <-chan RunEvent {
	events := make(chan RunEvent, 64)
	go r.run(ctx, events)
	return events
}

// Special tool names handled by the runner, not the ToolExecutor.
const (
	toolRecordFinding    = "record_finding"
	toolReadChannel      = "read_channel"
	toolPostChannel      = "post_channel"
	toolMarkInvestigated = "mark_investigated"
)

func (r *Runner) run(ctx context.Context, events chan<- RunEvent) {
	defer close(events)

	messages := []providers.Message{}

	for {
		// 1. Check context cancellation
		select {
		case <-ctx.Done():
			r.emit(events, RunEvent{Type: EventError, Text: "cancelled"})
			_ = r.store.UpdateSessionStatus(ctx, r.cfg.SessionID, "cancelled")
			return
		default:
		}

		// 2. Check budget before each turn
		if reason := r.budget.Exceeded(); reason != "" {
			r.emit(events, RunEvent{Type: EventBudgetExceeded, Text: reason})
			_ = r.store.UpdateSessionStatus(ctx, r.cfg.SessionID, "completed")
			return
		}

		// 3. Build request
		req := providers.CompletionRequest{
			SystemPrompt: BuildSystemPrompt(r.promptCfg),
			Messages:     messages,
			Tools:        r.buildToolDefinitions(),
			MaxTokens:    4096,
		}

		// 4. Call provider, stream response
		stream, err := r.provider.Complete(ctx, req)
		if err != nil {
			r.emit(events, RunEvent{Type: EventError, Text: fmt.Sprintf("provider error: %s", err)})
			_ = r.store.UpdateSessionStatus(ctx, r.cfg.SessionID, "failed")
			return
		}

		// 5. Process stream events
		var assistantText strings.Builder
		var toolCalls []providers.ToolCall
		var usage *providers.Usage
		var streamError bool

		for event := range stream {
			switch event.Type {
			case "text_delta":
				assistantText.WriteString(event.Text)
				r.emit(events, RunEvent{Type: EventText, Text: event.Text})

			case "tool_call":
				if event.ToolCall != nil {
					toolCalls = append(toolCalls, *event.ToolCall)
					r.emit(events, RunEvent{Type: EventToolCall, ToolName: event.ToolCall.Name, ToolCall: event.ToolCall})
				}

			case "done":
				usage = event.Usage

			case "error":
				r.emit(events, RunEvent{Type: EventError, Text: event.Error})
				streamError = true
			}
		}

		if streamError {
			_ = r.store.UpdateSessionStatus(ctx, r.cfg.SessionID, "failed")
			return
		}

		// 6. Record usage
		if usage != nil {
			r.budget.Record(usage.InputTokens, usage.OutputTokens, usage.CostUSD, len(toolCalls))
			_ = r.store.UpdateSessionCost(ctx, r.cfg.SessionID, usage.CostUSD)
		}

		// 7. Append assistant message to history
		assistantMsg := buildAssistantMessage(assistantText.String(), toolCalls)
		messages = append(messages, assistantMsg)

		// 8. If no tool calls, agent is done
		if len(toolCalls) == 0 {
			r.emit(events, RunEvent{Type: EventDone, Text: r.budget.Summary()})
			_ = r.store.UpdateSessionStatus(ctx, r.cfg.SessionID, "completed")
			return
		}

		// 9. Execute each tool call and build results
		toolResults := []providers.Block{}
		for _, tc := range toolCalls {
			result := r.executeTool(ctx, tc, events)
			toolResults = append(toolResults, providers.Block{
				Type: "tool_result",
				ToolResult: &providers.ToolResult{
					ToolCallID: tc.ID,
					Content:    result.Content,
					IsError:    result.IsError,
				},
			})
		}

		// 10. Append tool results as user message
		messages = append(messages, providers.Message{
			Role:    "user",
			Content: toolResults,
		})

		// Loop back to call LLM again with results
	}
}

// executeTool handles both special runner tools and ToolExecutor tools.
func (r *Runner) executeTool(ctx context.Context, tc providers.ToolCall, events chan<- RunEvent) tools.ToolResult {
	switch tc.Name {
	case toolRecordFinding:
		return r.handleRecordFinding(ctx, tc.Params, events)
	case toolReadChannel:
		return r.handleReadChannel(ctx)
	case toolPostChannel:
		return r.handlePostChannel(ctx, tc.Params)
	case toolMarkInvestigated:
		return r.handleMarkInvestigated(ctx, tc.Params)
	default:
		// Regular tool — delegate to executor
		result, err := r.executor.Execute(tc.Name, tc.Params)
		if err != nil {
			result = tools.ToolResult{Content: fmt.Sprintf("execution error: %s", err), IsError: true}
		}
		r.emit(events, RunEvent{Type: EventToolResult, ToolName: tc.Name, Text: truncateResult(result.Content), IsError: result.IsError})
		return result
	}
}

// handleRecordFinding validates and stores a finding.
func (r *Runner) handleRecordFinding(ctx context.Context, params map[string]any, events chan<- RunEvent) tools.ToolResult {
	title, _ := params["title"].(string)
	location, _ := params["location"].(string)
	severity, _ := params["severity"].(string)
	confidence, _ := params["confidence"].(string)
	description, _ := params["description"].(string)
	dataFlow, _ := params["data_flow"].(string)
	category, _ := params["category"].(string)

	if title == "" || location == "" || description == "" {
		return tools.ToolResult{Content: "record_finding requires title, location, and description", IsError: true}
	}

	// Check for duplicates
	exists, err := r.store.FindingExists(ctx, r.cfg.ProjectID, location, title)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("error checking duplicates: %s", err), IsError: true}
	}
	if exists {
		return tools.ToolResult{Content: fmt.Sprintf("duplicate finding: %q at %s already exists", title, location), IsError: false}
	}

	finding := &memory.Finding{
		SessionID:   r.cfg.SessionID,
		ProjectID:   r.cfg.ProjectID,
		Title:       title,
		Location:    location,
		Category:    category,
		Severity:    severity,
		Confidence:  confidence,
		Description: description,
		DataFlow:    dataFlow,
		FoundBy:     r.cfg.AgentName,
	}

	if err := r.store.CreateFinding(ctx, finding); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("error recording finding: %s", err), IsError: true}
	}

	r.emit(events, RunEvent{Type: EventFindingRecorded, Text: fmt.Sprintf("[%s] %s @ %s", severity, title, location)})
	return tools.ToolResult{Content: fmt.Sprintf("Finding recorded: %s (severity: %s)", title, severity)}
}

// handleReadChannel polls for messages from the partner agent.
func (r *Runner) handleReadChannel(ctx context.Context) tools.ToolResult {
	if r.channel == nil {
		return tools.ToolResult{Content: "channel not available in single mode", IsError: true}
	}
	msgs, err := r.channel.Poll(ctx, r.cfg.AgentName)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("error reading channel: %s", err), IsError: true}
	}
	return tools.ToolResult{Content: channel.FormatMessages(msgs)}
}

// handlePostChannel sends a message to the partner agent.
func (r *Runner) handlePostChannel(ctx context.Context, params map[string]any) tools.ToolResult {
	if r.channel == nil {
		return tools.ToolResult{Content: "channel not available in single mode", IsError: true}
	}
	msgType, _ := params["msg_type"].(string)
	content, _ := params["content"].(string)
	if msgType == "" || content == "" {
		return tools.ToolResult{Content: "post_channel requires msg_type and content", IsError: true}
	}

	// Determine partner
	toAgent := "agent_b"
	if r.cfg.AgentName == "agent_b" {
		toAgent = "agent_a"
	}

	if err := r.channel.Post(ctx, r.cfg.AgentName, toAgent, msgType, content); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("error posting to channel: %s", err), IsError: true}
	}
	return tools.ToolResult{Content: fmt.Sprintf("Message posted to %s (%s)", toAgent, msgType)}
}

// handleMarkInvestigated records that an area has been analyzed.
func (r *Runner) handleMarkInvestigated(ctx context.Context, params map[string]any) tools.ToolResult {
	path, _ := params["path"].(string)
	pattern, _ := params["pattern"].(string)
	if path == "" || pattern == "" {
		return tools.ToolResult{Content: "mark_investigated requires path and pattern", IsError: true}
	}

	area := &memory.InvestigatedArea{
		ProjectID: r.cfg.ProjectID,
		SessionID: r.cfg.SessionID,
		Path:      path,
		Pattern:   pattern,
		Agent:     r.cfg.AgentName,
	}

	if err := r.store.MarkInvestigated(ctx, area); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("error marking investigated: %s", err), IsError: true}
	}
	return tools.ToolResult{Content: fmt.Sprintf("Marked as investigated: %s (%s)", path, pattern)}
}

// buildToolDefinitions combines executor tools with runner special tools.
func (r *Runner) buildToolDefinitions() []providers.ToolDefinition {
	// Convert executor schemas to provider ToolDefinitions
	schemas := r.executor.Schemas()
	defs := make([]providers.ToolDefinition, 0, len(schemas)+4)

	for _, s := range schemas {
		// Convert tools.ToolSchema (map[string]ParamDef) to JSON Schema (map[string]any)
		properties := make(map[string]any)
		for name, pd := range s.Parameters {
			properties[name] = map[string]any{
				"type":        pd.Type,
				"description": pd.Description,
			}
		}

		defs = append(defs, providers.ToolDefinition{
			Name:        s.Name,
			Description: s.Description,
			Parameters: map[string]any{
				"type":       "object",
				"properties": properties,
				"required":   s.Required,
			},
		})
	}

	// Add special runner tools
	defs = append(defs, providers.ToolDefinition{
		Name:        toolRecordFinding,
		Description: "Record a confirmed vulnerability finding to persistent storage.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":       map[string]any{"type": "string", "description": "Clear, descriptive title for the finding"},
				"location":    map[string]any{"type": "string", "description": "file:line or file:function where the vulnerability exists"},
				"severity":    map[string]any{"type": "string", "description": "critical|high|medium|low|info"},
				"confidence":  map[string]any{"type": "string", "description": "confirmed|likely|suspected"},
				"description": map[string]any{"type": "string", "description": "Detailed description of the vulnerability"},
				"data_flow":   map[string]any{"type": "string", "description": "How to trigger the vulnerability (attack vector)"},
				"category":    map[string]any{"type": "string", "description": "CWE ID or custom category"},
			},
			"required": []string{"title", "location", "severity", "confidence", "description"},
		},
	})

	defs = append(defs, providers.ToolDefinition{
		Name:        toolMarkInvestigated,
		Description: "Mark a code area as investigated to avoid redundant analysis in future sessions.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string", "description": "File or directory path that was investigated"},
				"pattern": map[string]any{"type": "string", "description": "What was looked for (e.g., 'SQL injection patterns')"},
			},
			"required": []string{"path", "pattern"},
		},
	})

	// Channel tools only in collaborative mode
	if r.channel != nil {
		defs = append(defs, providers.ToolDefinition{
			Name:        toolReadChannel,
			Description: "Read unread messages from your partner agent. Call every ~8 tool invocations.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		})

		defs = append(defs, providers.ToolDefinition{
			Name:        toolPostChannel,
			Description: "Send a message to your partner agent.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"msg_type": map[string]any{"type": "string", "description": "finding|question|context|duplicate"},
					"content":  map[string]any{"type": "string", "description": "Message content to send"},
				},
				"required": []string{"msg_type", "content"},
			},
		})
	}

	return defs
}

// emit sends a RunEvent with the agent name and timestamp.
func (r *Runner) emit(events chan<- RunEvent, evt RunEvent) {
	evt.AgentName = r.cfg.AgentName
	evt.Timestamp = time.Now()
	events <- evt
}

// buildAssistantMessage creates a provider Message from text and tool calls.
func buildAssistantMessage(text string, toolCalls []providers.ToolCall) providers.Message {
	var blocks []providers.Block

	if text != "" {
		blocks = append(blocks, providers.Block{Type: "text", Text: text})
	}

	for i := range toolCalls {
		tc := toolCalls[i]
		blocks = append(blocks, providers.Block{
			Type:     "tool_call",
			ToolCall: &tc,
		})
	}

	return providers.Message{Role: "assistant", Content: blocks}
}

// truncateResult shortens tool output for display.
func truncateResult(s string) string {
	if len(s) <= 200 {
		return s
	}
	return s[:200] + "..."
}
