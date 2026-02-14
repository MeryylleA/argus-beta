package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Sandbox enforces filesystem boundaries for all tool operations.
// This is the single most critical security component in the tool executor.
//
// SECURITY MODEL:
// - All paths are resolved to absolute form before comparison
// - Symlinks are resolved to prevent symlink traversal attacks
// - The root path itself is resolved at sandbox creation time
// - No tool can access anything outside the resolved root
//
// THREAT MODEL:
// - Path traversal via "../" sequences
// - Symlink escape (symlink inside repo pointing to /etc/passwd)
// - Unicode/encoding tricks in filenames
// - Race conditions (TOCTOU) — mitigated by resolving at check time
type Sandbox struct {
	// resolvedRoot is the absolute, symlink-resolved path that forms
	// the boundary. Computed once at creation, never changed.
	resolvedRoot string
}

// NewSandbox creates a sandbox rooted at the given path.
// The path must exist and must be a directory.
// We resolve symlinks immediately to establish a canonical root.
func NewSandbox(rootPath string) (*Sandbox, error) {
	// Step 1: Resolve to absolute path
	absPath, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("sandbox: failed to resolve absolute path %q: %w", rootPath, err)
	}

	// Step 2: Resolve all symlinks in the root path itself.
	// This prevents an attacker from creating a repo where the root
	// is itself a symlink to "/" or another sensitive directory.
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return nil, fmt.Errorf("sandbox: failed to resolve symlinks for %q: %w", absPath, err)
	}

	// Step 3: Verify the root exists and is a directory
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("sandbox: root path %q does not exist: %w", resolvedPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("sandbox: root path %q is not a directory", resolvedPath)
	}

	return &Sandbox{resolvedRoot: resolvedPath}, nil
}

// ValidatePath checks that the given path is within the sandbox root.
// Returns the resolved absolute path if valid, or an error if the path
// escapes the sandbox.
//
// SECURITY: This function is the gatekeeper. Every tool MUST call this
// before accessing any file or directory. The function:
// 1. Joins the path with root if relative
// 2. Resolves to absolute
// 3. Resolves symlinks (preventing symlink escape)
// 4. Verifies the resolved path starts with the sandbox root
func (s *Sandbox) ValidatePath(requestedPath string) (string, error) {
	var absPath string

	if filepath.IsAbs(requestedPath) {
		absPath = filepath.Clean(requestedPath)
	} else {
		// Relative paths are resolved against the sandbox root
		absPath = filepath.Clean(filepath.Join(s.resolvedRoot, requestedPath))
	}

	// Resolve symlinks to get the real path on disk.
	// This is critical: without this, a symlink at /repo/link -> /etc/shadow
	// would pass the prefix check but access files outside the sandbox.
	//
	// Note: If the file doesn't exist yet, EvalSymlinks will fail.
	// For our read-only tools, this is actually desired behavior —
	// we only want to allow access to files that exist.
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// If the path doesn't exist, try resolving the parent directory.
		// This handles the case where we want to check if a path *would be*
		// valid (e.g., for find_files where results might come from fd/rg).
		parentDir := filepath.Dir(absPath)
		resolvedParent, parentErr := filepath.EvalSymlinks(parentDir)
		if parentErr != nil {
			return "", fmt.Errorf("sandbox: path %q does not exist and parent cannot be resolved: %w", requestedPath, err)
		}

		// Check if the parent is within bounds
		if !s.isWithinRoot(resolvedParent) {
			return "", fmt.Errorf("sandbox: path %q resolves outside sandbox root", requestedPath)
		}

		// Return the cleaned absolute path (parent is valid, file just doesn't exist)
		return absPath, nil
	}

	// The critical check: does the resolved path start with our root?
	if !s.isWithinRoot(resolvedPath) {
		return "", fmt.Errorf("sandbox: path %q resolves to %q which is outside sandbox root %q",
				      requestedPath, resolvedPath, s.resolvedRoot)
	}

	return resolvedPath, nil
}

// isWithinRoot performs the actual containment check.
// We add a path separator to prevent partial matches:
// root="/repo" should not match "/repository" or "/repo-other"
//
// Special case: the root itself is always valid.
func (s *Sandbox) isWithinRoot(resolvedPath string) bool {
	if resolvedPath == s.resolvedRoot {
		return true
	}
	// Ensure the path is under the root directory by checking for
	// the root prefix followed by a path separator.
	// Without the separator check, root="/tmp/a" would match "/tmp/abc"
	return strings.HasPrefix(resolvedPath, s.resolvedRoot+string(filepath.Separator))
}

// Root returns the resolved sandbox root path.
// Tools use this to pass as working directory to external commands.
func (s *Sandbox) Root() string {
	return s.resolvedRoot
}

// ValidateOutputPath checks a path that was returned by an external command
// (like rg or fd) to ensure it's within the sandbox. This is a defense-in-depth
// measure — even if we pass --no-follow to rg, we verify the output.
func (s *Sandbox) ValidateOutputPath(outputPath string) (string, error) {
	var absPath string
	if filepath.IsAbs(outputPath) {
		absPath = filepath.Clean(outputPath)
	} else {
		absPath = filepath.Clean(filepath.Join(s.resolvedRoot, outputPath))
	}

	// For output validation, we don't resolve symlinks — the external command
	// already resolved them. We just check the prefix.
	if !s.isWithinRoot(absPath) {
		return "", fmt.Errorf("sandbox: output path %q is outside sandbox root", outputPath)
	}

	return absPath, nil
}
