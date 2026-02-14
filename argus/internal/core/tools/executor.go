package tools

import (
	"context"
	"fmt"
	"time"
)

const (
	// DefaultTimeout is the maximum duration for any single tool call.
	// 30 seconds is generous for read-only operations — if a search takes
	// longer than this, the pattern is probably too broad and should be refined.
	// This also prevents runaway processes from blocking the agent loop.
	DefaultTimeout = 30 * time.Second
)

// ToolExecutor is the central coordinator for all tool operations.
// It enforces the sandbox, applies timeouts, and logs every invocation.
//
// The executor is the ONLY way tools should be called — direct tool
// execution bypasses security controls and audit logging.
type ToolExecutor struct {
	sandbox *Sandbox
	tools   map[string]Tool
	logger  *ToolLogger
	timeout time.Duration
}

// NewToolExecutor creates an executor bound to a specific root path.
// All tool operations are restricted to files under this root.
func NewToolExecutor(rootPath string, logger *ToolLogger) (*ToolExecutor, error) {
	sandbox, err := NewSandbox(rootPath)
	if err != nil {
		return nil, fmt.Errorf("tool executor: failed to create sandbox: %w", err)
	}

	executor := &ToolExecutor{
		sandbox: sandbox,
		tools:   make(map[string]Tool),
		logger:  logger,
		timeout: DefaultTimeout,
	}

	// Register all built-in tools.
	// Each tool receives a reference to the sandbox so it can validate paths.
	executor.registerBuiltinTools()

	return executor, nil
}

// registerBuiltinTools initializes and registers all available tools.
// Adding a new tool requires only creating the tool file and adding
// a line here — the executor handles everything else.
func (e *ToolExecutor) registerBuiltinTools() {
	builtins := []Tool{
		NewSearchCodeTool(e.sandbox),
		NewViewLinesTool(e.sandbox),
		NewFindFilesTool(e.sandbox),
		NewDirectoryTreeTool(e.sandbox),
		NewGitLogTool(e.sandbox),
	}

	for _, tool := range builtins {
		e.tools[tool.Name()] = tool
	}
}

// Execute runs a named tool with the given parameters.
// This is the main entry point called by the agent loop.
//
// Security controls applied:
// 1. Tool name validation (only registered tools can execute)
// 2. Timeout enforcement (prevents runaway processes)
// 3. Full audit logging (params + result + duration)
func (e *ToolExecutor) Execute(toolName string, params map[string]any) (ToolResult, error) {
	tool, exists := e.tools[toolName]
	if !exists {
		return ToolResult{
			Content: fmt.Sprintf("Unknown tool: %q. Available tools: %v", toolName, e.ToolNames()),
			IsError: true,
		}, nil
	}

	start := time.Now()

	// Execute with timeout context.
	// The context is passed implicitly — tools that shell out to external
	// commands (rg, fd, git) use exec.CommandContext for cancellation.
	// We use a channel-based pattern here because the Tool interface
	// doesn't take a context (keeping it simple for tool implementors).
	resultCh := make(chan toolExecResult, 1)

	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	go func() {
		// Store context in params so tools can use it for exec.CommandContext
		// This is a pragmatic choice — adding context to the Tool interface
		// would be cleaner but would complicate every tool implementation.
		params["__ctx"] = ctx
		result, err := tool.Execute(params)
		resultCh <- toolExecResult{result: result, err: err}
	}()

	var result ToolResult
	var err error

	select {
		case <-ctx.Done():
			result = ToolResult{
				Content: fmt.Sprintf("Tool %q timed out after %s", toolName, e.timeout),
				IsError: true,
			}
			err = nil // Timeout is not a system error — it's reported to the agent
		case execResult := <-resultCh:
			result = execResult.result
			err = execResult.err
	}

	duration := time.Since(start)

	// Clean internal params before logging — don't log the context object
	cleanParams := make(map[string]any, len(params))
	for k, v := range params {
		if k != "__ctx" {
			cleanParams[k] = v
		}
	}

	// Log every invocation regardless of success/failure.
	// The audit trail must be complete.
	e.logger.Log(ToolLogEntry{
		Timestamp:  start,
		ToolName:   toolName,
		Params:     cleanParams,
		Result:     result,
		DurationMs: duration.Milliseconds(),
	})

	return result, err
}

// GetTool returns a tool by name, if it exists.
func (e *ToolExecutor) GetTool(name string) (Tool, bool) {
	tool, exists := e.tools[name]
	return tool, exists
}

// ToolNames returns the names of all registered tools.
func (e *ToolExecutor) ToolNames() []string {
	names := make([]string, 0, len(e.tools))
	for name := range e.tools {
		names = append(names, name)
	}
	return names
}

// Schemas returns all tool schemas, ready to be sent to any LLM provider.
func (e *ToolExecutor) Schemas() []ToolSchema {
	schemas := make([]ToolSchema, 0, len(e.tools))
	for _, tool := range e.tools {
		schemas = append(schemas, tool.Schema())
	}
	return schemas
}

// RootPath returns the sandbox root for display purposes.
func (e *ToolExecutor) RootPath() string {
	return e.sandbox.Root()
}

// toolExecResult is used internally for channel-based timeout pattern.
type toolExecResult struct {
	result ToolResult
	err    error
}
