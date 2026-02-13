package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewToolExecutor_ValidPath(t *testing.T) {
	dir := t.TempDir()
	logger := NewToolLogger(false)

	executor, err := NewToolExecutor(dir, logger)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Should have all 5 tools registered
	names := executor.ToolNames()
	if len(names) != 5 {
		t.Fatalf("expected 5 tools, got %d: %v", len(names), names)
	}
}

func TestNewToolExecutor_InvalidPath(t *testing.T) {
	logger := NewToolLogger(false)
	_, err := NewToolExecutor("/nonexistent/path", logger)
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
}

func TestExecutor_UnknownTool(t *testing.T) {
	dir := t.TempDir()
	logger := NewToolLogger(false)
	executor, err := NewToolExecutor(dir, logger)
	if err != nil {
		t.Fatalf("failed to create executor: %v", err)
	}

	result, err := executor.Execute("nonexistent_tool", map[string]any{})
	if err != nil {
		t.Fatalf("unknown tool should return result error, not Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("result should be marked as error for unknown tool")
	}
}

func TestExecutor_LogsEveryCall(t *testing.T) {
	dir := t.TempDir()

	// Create a file so view_lines has something to read
	testFile := filepath.Join(dir, "test.go")
	if err := os.WriteFile(testFile, []byte("line 1\nline 2\nline 3\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	logger := NewToolLogger(false)
	executor, err := NewToolExecutor(dir, logger)
	if err != nil {
		t.Fatalf("failed to create executor: %v", err)
	}

	// Execute a tool
	_, err = executor.Execute("view_lines", map[string]any{
		"file":       testFile,
		"start_line": 1,
		"end_line":   3,
	})
	if err != nil {
		t.Fatalf("execution failed: %v", err)
	}

	// Verify it was logged
	entries := logger.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0].ToolName != "view_lines" {
		t.Fatalf("expected tool name 'view_lines', got %q", entries[0].ToolName)
	}
	if entries[0].DurationMs < 0 {
		t.Fatal("duration should not be negative")
	}
}

func TestExecutor_Schemas(t *testing.T) {
	dir := t.TempDir()
	logger := NewToolLogger(false)
	executor, err := NewToolExecutor(dir, logger)
	if err != nil {
		t.Fatalf("failed to create executor: %v", err)
	}

	schemas := executor.Schemas()
	if len(schemas) != 5 {
		t.Fatalf("expected 5 schemas, got %d", len(schemas))
	}

	// Verify each schema has required fields
	for _, schema := range schemas {
		if schema.Name == "" {
			t.Error("schema name should not be empty")
		}
		if schema.Description == "" {
			t.Errorf("schema %q should have a description", schema.Name)
		}
	}
}

func TestExecutor_ViewLines_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	logger := NewToolLogger(false)
	executor, err := NewToolExecutor(dir, logger)
	if err != nil {
		t.Fatalf("failed to create executor: %v", err)
	}

	// Try to read /etc/passwd via path traversal
	result, err := executor.Execute("view_lines", map[string]any{
		"file":       "../../../etc/passwd",
		"start_line": 1,
		"end_line":   10,
	})
	if err != nil {
		t.Fatalf("should return tool error, not Go error: %v", err)
	}
	if !result.IsError {
		t.Error("SECURITY FAILURE: path traversal in view_lines should be rejected")
	}
}

func TestExecutor_DirectoryTree_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	logger := NewToolLogger(false)
	executor, err := NewToolExecutor(dir, logger)
	if err != nil {
		t.Fatalf("failed to create executor: %v", err)
	}

	result, err := executor.Execute("directory_tree", map[string]any{
		"path":  "../../../",
		"depth": 1,
	})
	if err != nil {
		t.Fatalf("should return tool error, not Go error: %v", err)
	}
	if !result.IsError {
		t.Error("SECURITY FAILURE: path traversal in directory_tree should be rejected")
	}
}
