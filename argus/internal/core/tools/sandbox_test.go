package tools

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSandbox validates the core security boundary.
// These tests are critical — a failure here means potential sandbox escape.

func TestNewSandbox_ValidDirectory(t *testing.T) {
	dir := t.TempDir()
	sandbox, err := NewSandbox(dir)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if sandbox.Root() == "" {
		t.Fatal("sandbox root should not be empty")
	}
}

func TestNewSandbox_NonexistentPath(t *testing.T) {
	_, err := NewSandbox("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
}

func TestNewSandbox_FileNotDirectory(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "somefile.txt")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	_, err := NewSandbox(filePath)
	if err == nil {
		t.Fatal("expected error when root is a file, not a directory")
	}
}

func TestValidatePath_ValidSubdirectory(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "src")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	sandbox, err := NewSandbox(dir)
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}

	result, err := sandbox.ValidatePath(subdir)
	if err != nil {
		t.Fatalf("expected valid path, got error: %v", err)
	}
	if result == "" {
		t.Fatal("result should not be empty")
	}
}

func TestValidatePath_ValidFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(filePath, []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	sandbox, err := NewSandbox(dir)
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}

	result, err := sandbox.ValidatePath(filePath)
	if err != nil {
		t.Fatalf("expected valid path, got error: %v", err)
	}
	if result != filePath {
		t.Fatalf("expected %q, got %q", filePath, result)
	}
}

func TestValidatePath_RelativePath(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "src", "main.go")
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	sandbox, err := NewSandbox(dir)
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}

	// Relative path should be resolved against sandbox root
	result, err := sandbox.ValidatePath("src/main.go")
	if err != nil {
		t.Fatalf("expected valid path, got error: %v", err)
	}
	if result != filePath {
		t.Fatalf("expected %q, got %q", filePath, result)
	}
}

func TestValidatePath_PathTraversal_DotDot(t *testing.T) {
	dir := t.TempDir()
	sandbox, err := NewSandbox(dir)
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}

	// These should ALL be rejected — they attempt to escape the sandbox
	maliciousPaths := []string{
		"../../../etc/passwd",
		"..\\..\\..\\etc\\passwd",
		"src/../../../etc/shadow",
		"/etc/passwd",
		"/etc/shadow",
		"../",
		"..",
	}

	for _, p := range maliciousPaths {
		_, err := sandbox.ValidatePath(p)
		if err == nil {
			t.Errorf("SECURITY FAILURE: path %q should have been rejected but was allowed", p)
		}
	}
}

func TestValidatePath_AbsolutePathOutsideSandbox(t *testing.T) {
	dir := t.TempDir()
	sandbox, err := NewSandbox(dir)
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}

	// Absolute paths outside the sandbox must be rejected
	_, err = sandbox.ValidatePath("/usr/bin/bash")
	if err == nil {
		t.Error("SECURITY FAILURE: absolute path outside sandbox should be rejected")
	}
}

func TestValidatePath_SymlinkEscape(t *testing.T) {
	dir := t.TempDir()

	// Create a symlink inside the sandbox that points outside
	symlinkPath := filepath.Join(dir, "escape_link")
	err := os.Symlink("/etc", symlinkPath)
	if err != nil {
		t.Skipf("Cannot create symlinks (permissions?): %v", err)
	}

	sandbox, err := NewSandbox(dir)
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}

	// Attempting to access through the symlink should be rejected
	_, err = sandbox.ValidatePath("escape_link/passwd")
	if err == nil {
		t.Error("SECURITY FAILURE: symlink escape should be detected and rejected")
	}
}

func TestValidatePath_SymlinkToFileOutside(t *testing.T) {
	dir := t.TempDir()

	// Create a file outside the sandbox
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret data"), 0644); err != nil {
		t.Fatalf("failed to create outside file: %v", err)
	}

	// Symlink from inside sandbox to outside file
	symlinkPath := filepath.Join(dir, "innocent.txt")
	if err := os.Symlink(outsideFile, symlinkPath); err != nil {
		t.Skipf("Cannot create symlinks: %v", err)
	}

	sandbox, err := NewSandbox(dir)
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}

	_, err = sandbox.ValidatePath("innocent.txt")
	if err == nil {
		t.Error("SECURITY FAILURE: symlink pointing outside sandbox should be rejected")
	}
}

func TestValidatePath_PartialNameMatch(t *testing.T) {
	// Create two directories where one name is a prefix of the other
	// e.g., /tmp/repo and /tmp/repo-evil
	// Access to /tmp/repo-evil should NOT be allowed when sandbox is /tmp/repo
	baseDir := t.TempDir()
	repoDir := filepath.Join(baseDir, "repo")
	evilDir := filepath.Join(baseDir, "repo-evil")

	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}
	if err := os.MkdirAll(evilDir, 0755); err != nil {
		t.Fatalf("failed to create evil dir: %v", err)
	}

	// Create a file in the evil directory
	evilFile := filepath.Join(evilDir, "secrets.txt")
	if err := os.WriteFile(evilFile, []byte("stolen"), 0644); err != nil {
		t.Fatalf("failed to create evil file: %v", err)
	}

	sandbox, err := NewSandbox(repoDir)
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}

	// This should be rejected — evilDir is NOT under repoDir
	_, err = sandbox.ValidatePath(evilFile)
	if err == nil {
		t.Error("SECURITY FAILURE: path with matching prefix but different directory should be rejected")
	}
}

func TestValidatePath_RootItself(t *testing.T) {
	dir := t.TempDir()
	sandbox, err := NewSandbox(dir)
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}

	// The root itself should be valid
	result, err := sandbox.ValidatePath(dir)
	if err != nil {
		t.Fatalf("sandbox root itself should be valid, got error: %v", err)
	}
	if result != sandbox.Root() {
		t.Fatalf("expected %q, got %q", sandbox.Root(), result)
	}
}

func TestValidateOutputPath(t *testing.T) {
	dir := t.TempDir()
	sandbox, err := NewSandbox(dir)
	if err != nil {
		t.Fatalf("failed to create sandbox: %v", err)
	}

	// Valid output path
	validPath := filepath.Join(dir, "src/main.go")
	_, err = sandbox.ValidateOutputPath(validPath)
	if err != nil {
		t.Fatalf("expected valid output path, got error: %v", err)
	}

	// Invalid output path
	_, err = sandbox.ValidateOutputPath("/etc/passwd")
	if err == nil {
		t.Error("output path outside sandbox should be rejected")
	}
}
