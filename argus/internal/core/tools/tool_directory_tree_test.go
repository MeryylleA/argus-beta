package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectoryTree_BasicStructure(t *testing.T) {
	dir := t.TempDir()

	// Create a small project structure
	dirs := []string{"src", "src/auth", "src/api", "tests", "config"}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(dir, d), 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
	}

	files := []string{"README.md", "src/main.go", "src/auth/handler.go", "config/app.yaml"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("content"), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
	}

	sandbox, err := NewSandbox(dir)
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}

	tool := NewDirectoryTreeTool(sandbox)
	result, err := tool.Execute(map[string]any{
		"depth": 3,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}

	// Verify key elements are present
	if !strings.Contains(result.Content, "src/") {
		t.Error("should contain src/ directory")
	}
	if !strings.Contains(result.Content, "README.md") {
		t.Error("should contain README.md")
	}
}

func TestDirectoryTree_SkipsNodeModules(t *testing.T) {
	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "node_modules", "some-package"), 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	sandbox, err := NewSandbox(dir)
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}

	tool := NewDirectoryTreeTool(sandbox)
	result, err := tool.Execute(map[string]any{"depth": 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(result.Content, "node_modules") {
		t.Error("should skip node_modules directory")
	}
	if !strings.Contains(result.Content, "src/") {
		t.Error("should include src/ directory")
	}
}

func TestDirectoryTree_DepthCap(t *testing.T) {
	dir := t.TempDir()
	sandbox, err := NewSandbox(dir)
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}

	tool := NewDirectoryTreeTool(sandbox)

	// Request depth 100 — should be capped to 6
	result, err := tool.Execute(map[string]any{"depth": 100})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	depth := result.Metadata["depth"].(int)
	if depth > maxTreeDepth {
		t.Errorf("depth should be capped to %d, got %d", maxTreeDepth, depth)
	}
}

func TestDirectoryTree_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	sandbox, err := NewSandbox(dir)
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}

	tool := NewDirectoryTreeTool(sandbox)
	result, err := tool.Execute(map[string]any{
		"path":  "../../../",
		"depth": 1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("SECURITY FAILURE: path traversal should be rejected")
	}
}
