package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// ToolLogger provides structured logging for all tool invocations.
// Every call is logged with full parameters and results for audit purposes.
//
// In a security intelligence platform, the audit trail is non-negotiable.
// If an agent accesses a file, we need to know exactly when, what parameters
// were used, and what was returned.
//
// Thread-safety: The logger uses a mutex because multiple agents may
// invoke tools concurrently in collaborative mode (Phase 3+).
type ToolLogger struct {
	mu      sync.Mutex
	entries []ToolLogEntry

	// printToStdout controls whether log entries are also printed.
	// Useful during development; will be replaced by proper log
	// streaming via SSE in the API layer.
	printToStdout bool
}

// NewToolLogger creates a logger instance.
func NewToolLogger(printToStdout bool) *ToolLogger {
	return &ToolLogger{
		entries:       make([]ToolLogEntry, 0, 256),
		printToStdout: printToStdout,
	}
}

// Log records a tool invocation. Called by the executor after every tool call.
func (l *ToolLogger) Log(entry ToolLogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.entries = append(l.entries, entry)

	if l.printToStdout {
		l.printEntry(entry)
	}
}

// Entries returns a copy of all log entries.
// Returns a copy to prevent external mutation of the audit trail.
func (l *ToolLogger) Entries() []ToolLogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	result := make([]ToolLogEntry, len(l.entries))
	copy(result, l.entries)
	return result
}

// printEntry writes a formatted log entry to stdout.
func (l *ToolLogger) printEntry(entry ToolLogEntry) {
	status := "OK"
	if entry.Result.IsError {
		status = "ERROR"
	}

	// Marshal params for display — ignore error since this is just logging
	paramsJSON, _ := json.Marshal(entry.Params)

	fmt.Fprintf(os.Stderr, "[%s] [%s] %s(%s) → %dms | %s\n",
		    entry.Timestamp.Format("15:04:05.000"),
		    status,
	     entry.ToolName,
	     string(paramsJSON),
		    entry.DurationMs,
	     truncate(entry.Result.Content, 200),
	)
}

// truncate shortens a string for display purposes.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... (truncated)"
}
