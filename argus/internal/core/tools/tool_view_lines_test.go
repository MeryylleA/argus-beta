package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestViewLines_BasicRead(t *testing.T) {
	dir := t.TempDir()
	content := "line 1\nline 2\nline 3\nline 4\nline 5\n"
	filePath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	sandbox, err := NewSandbox(dir)
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}

	tool := NewViewLinesTool(sandbox)
	result, err := tool.Execute(map[string]any{
		"file":       filePath,
		"start_line": 2,
		"end_line":   4,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}

	if !strings.Contains(result.Content, "line 2") {
		t.Error("result should contain 'line 2'")
	}
	if !strings.Contains(result.Content, "line 4") {
		t.Error("result should contain 'line 4'")
	}
	if strings.Contains(result.Content, "line 1\n") {
		t.Error("result should NOT contain 'line 1' (before start_line)")
	}
}

func TestViewLines_MaxLinesCap(t *testing.T) {
	dir := t.TempDir()

	// Create a file with 200 lines
	var content strings.Builder
	for i := 1; i <= 200; i++ {
		content.WriteString("line content\n")
	}
	filePath := filepath.Join(dir, "big.txt")
	if err := os.WriteFile(filePath, []byte(content.String()), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	sandbox, err := NewSandbox(dir)
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}

	tool := NewViewLinesTool(sandbox)
	result, err := tool.Execute(map[string]any{
		"file":       filePath,
		"start_line": 1,
		"end_line":   200, // Exceeds max of 100
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}

	// Should only return 100 lines (the cap)
	linesRead := result.Metadata["lines_read"].(int)
	if linesRead > 100 {
		t.Errorf("should cap at 100 lines, got %d", linesRead)
	}
}

func TestViewLines_InvalidRange(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	sandbox, err := NewSandbox(dir)
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}

	tool := NewViewLinesTool(sandbox)

	// end_line < start_line
	result, err := tool.Execute(map[string]any{
		"file":       filePath,
		"start_line": 10,
		"end_line":   5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("should reject end_line < start_line")
	}
}

func TestViewLines_Directory(t *testing.T) {
	dir := t.TempDir()
	sandbox, err := NewSandbox(dir)
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}

	tool := NewViewLinesTool(sandbox)
	result, err := tool.Execute(map[string]any{
		"file":       dir,
		"start_line": 1,
		"end_line":   10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("should reject directories")
	}
}

func TestViewLines_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	sandbox, err := NewSandbox(dir)
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}

	tool := NewViewLinesTool(sandbox)
	result, err := tool.Execute(map[string]any{
		"file":       "../../../etc/passwd",
		"start_line": 1,
		"end_line":   10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("SECURITY FAILURE: path traversal should be rejected")
	}
}
