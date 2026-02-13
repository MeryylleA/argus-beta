# Argus Architecture
Argus is a local-first security intelligence platform designed for autonomous repository investigation under strict safety controls. This document describes the execution architecture and why each major design decision exists.

## Security Model in Architecture
Argus assumes analyzed repositories may be malicious. The architecture therefore treats every tool call as untrusted input and enforces a sandbox boundary before execution.

Security properties:
- Read-only operations only
- Path-restricted access rooted to an allowed project directory
- Symlink-aware canonical path validation
- Auditable execution logs for every tool run

## Tool Execution Flow
```text
User / Agent Intent
        |
        v
LLM proposes tool_call JSON
        |
        v
Executor receives call
        |
        v
Sandbox validation
  - canonicalize path
  - enforce root prefix
  - block symlink escapes
        |
        v
Tool implementation runs
        |
        v
Result returned to agent + audit logger
```

## Data Flow
```text
LLM
  -> tool_call JSON
  -> executor
  -> sandbox validation
  -> tool
  -> ToolResult (stdout/stderr/result data)
  -> logger (audit trail)
  -> agent response loop
```

## Concurrency Model
- Executor uses goroutines to enforce per-tool timeout boundaries.
- Timeout control prevents hung subprocesses from blocking the session.
- Logger uses a mutex to serialize writes and preserve coherent audit trails.
- This model keeps the CLI responsive while preserving deterministic logging.

## Design Decisions and Rationale
### Go over Python
- Single static binary is easier to deploy in hardened environments.
- Concurrency primitives (goroutines/channels) simplify bounded execution.
- Strong standard library support for subprocess and path handling.

### SQLite over Postgres
- Local-first requirement favors embedded storage.
- No runtime service dependency for researchers.
- Atomic transactions are sufficient for session/findings persistence.

### Read-only external tools
- `rg`, `fd/find`, and `git log` provide high-value repository intelligence.
- Restricting to read-only commands reduces attack surface and operational risk.

## Phase 2 Preview
Phase 2 adds provider adapters, an agent loop, and memory-backed context.

### Provider Interface (conceptual)
```go
type Provider interface {
    Name() string
    Send(ctx context.Context, req PromptRequest) (PromptResponse, error)
}
```

### Agent Loop (pseudocode)
```go
for turn := 0; turn < maxTurns; turn++ {
    resp := provider.Send(ctx, promptWithContext(memory, bountyScope))
    if resp.HasToolCall() {
        result := executor.Execute(resp.ToolCall)
        memory.AppendToolResult(result)
        continue
    }
    return resp.FinalAnswer
}
```

This keeps model interaction explicit, bounded, and auditable.
