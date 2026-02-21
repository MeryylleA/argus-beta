// Package agent implements the AI agent execution logic, including the
// filesystem sandbox and the main agent runner loop.
package agent

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"bufio"
)

var bannedDirs = map[string]bool{
	".git": true, "node_modules": true, "venv": true, ".venv": true,
	"env": true, "__pycache__": true, "dist": true, "build": true,
	"vendor": true, ".idea": true, ".vscode": true, "coverage": true,
}

var bannedExts = map[string]bool{
	".exe": true, ".dll": true, ".so": true, ".png": true, ".jpg": true,
	".jpeg": true, ".gif": true, ".pdf": true, ".zip": true, ".tar": true,
	".gz": true, ".mp4": true, ".mp3": true, ".wav": true, ".ico": true,
	".svg": true, ".woff": true, ".ttf": true,
}

// maxFileReadSize is the maximum number of bytes we will read from a single file.
// This protects the LLM context window from being flooded by large files.
const maxFileReadSize = 500 * 1024 // 500 KB limit.

// Sandbox provides a strict filesystem jail rooted at a workspace directory.
// All file operations are validated to prevent path traversal, symlink escapes,
// and oversized reads.
type Sandbox struct {
	// root is the resolved absolute path of the workspace directory.
	// All operations are constrained to this directory and its descendants.
	root string
}

// NewSandbox creates a new Sandbox for the given workspace root directory.
// The root path is resolved to an absolute, symlink-evaluated path.
func NewSandbox(root string) (*Sandbox, error) {
	// Resolve to absolute path first.
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("sandbox: resolve absolute path %q: %w", root, err)
	}

	// Evaluate symlinks so the root itself is canonical.
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return nil, fmt.Errorf("sandbox: eval symlinks for root %q: %w", absRoot, err)
	}

	// Verify the root is actually a directory.
	info, err := os.Stat(resolvedRoot)
	if err != nil {
		return nil, fmt.Errorf("sandbox: stat root %q: %w", resolvedRoot, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("sandbox: root %q is not a directory", resolvedRoot)
	}

	return &Sandbox{root: resolvedRoot}, nil
}

// Root returns the resolved root path of this sandbox.
func (s *Sandbox) Root() string {
	return s.root
}

// ValidatePath resolves a relative path within the sandbox and returns the
// safe absolute path. It rejects:
//   - Paths containing ".." segments (path traversal)
//   - Symlinks that resolve outside the sandbox root
//   - Absolute paths (must be relative to sandbox root)
func (s *Sandbox) ValidatePath(relPath string) (string, error) {
	// SECURITY: Reject absolute paths — all access must be relative.
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("sandbox: absolute paths are not allowed: %q", relPath)
	}

	// SECURITY: Reject any path containing ".." to prevent traversal.
	// We check both the raw string and cleaned path components.
	cleaned := filepath.Clean(relPath)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("sandbox: path traversal detected: %q", relPath)
	}

	// Join with root and resolve to absolute.
	joined := filepath.Join(s.root, cleaned)

	// SECURITY: Evaluate symlinks to catch symlink-based escapes.
	// If the file doesn't exist yet, we check the parent directory.
	resolved, err := filepath.EvalSymlinks(joined)
	if err != nil {
		// If the file doesn't exist, validate the parent directory instead.
		if os.IsNotExist(err) {
			parentDir := filepath.Dir(joined)
			resolvedParent, parentErr := filepath.EvalSymlinks(parentDir)
			if parentErr != nil {
				return "", fmt.Errorf("sandbox: resolve parent path %q: %w", parentDir, parentErr)
			}
			if !strings.HasPrefix(resolvedParent, s.root) {
				return "", fmt.Errorf("sandbox: path escapes sandbox via parent symlink: %q", relPath)
			}
			// Parent is safe; return the original joined path.
			return joined, nil
		}
		return "", fmt.Errorf("sandbox: resolve path %q: %w", joined, err)
	}

	// SECURITY: Verify resolved path is still under the sandbox root.
	if !strings.HasPrefix(resolved, s.root) {
		return "", fmt.Errorf("sandbox: path %q resolves outside sandbox (to %q)", relPath, resolved)
	}

	return resolved, nil
}

// ReadFile reads a file within the sandbox, enforcing the max read limit.
// Returns the file contents as bytes. Files larger than 2MB are rejected
// to protect the LLM context window.
func (s *Sandbox) ReadFile(relPath string) ([]byte, error) {
	safePath, err := s.ValidatePath(relPath)
	if err != nil {
		return nil, err
	}
	ext := strings.ToLower(filepath.Ext(relPath))
	if bannedExts[ext] {
		return nil, fmt.Errorf("Error: Cannot read binary or media files. Focus on source code.")
	}

	f, err := os.Open(safePath)
	if err != nil {
		return nil, fmt.Errorf("sandbox: open %q: %w", relPath, err)
	}
	defer f.Close()

	// Check file size before reading.
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("sandbox: stat %q: %w", relPath, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("sandbox: %q is a directory, not a file", relPath)
	}
	if info.Size() > maxFileReadSize {
		return nil, fmt.Errorf("Error: File is too large to read (exceeds 500KB limit).")
	}

	// Read with a hard limit as a safety net.
	data, err := io.ReadAll(io.LimitReader(f, maxFileReadSize+1))
	if err != nil {
		return nil, fmt.Errorf("sandbox: read %q: %w", relPath, err)
	}
	if int64(len(data)) > maxFileReadSize {
		return nil, fmt.Errorf("sandbox: file %q exceeds %d byte read limit", relPath, maxFileReadSize)
	}

	return data, nil
}

// ListDir lists the contents of a directory within the sandbox.
// Returns a slice of DirEntry information.
func (s *Sandbox) ListDir(relPath string) ([]FileInfo, error) {
	safePath, err := s.ValidatePath(relPath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(safePath)
	if err != nil {
		return nil, fmt.Errorf("sandbox: list directory %q: %w", relPath, err)
	}

	var result []FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue // Skip entries we can't stat.
		}

		if entry.IsDir() && bannedDirs[entry.Name()] {
			continue // Skip banned directories
		}
		if !entry.IsDir() && bannedExts[strings.ToLower(filepath.Ext(entry.Name()))] {
			continue // Skip banned extensions
		}

		result = append(result, FileInfo{
			Name:  entry.Name(),
			IsDir: entry.IsDir(),
			Size:  info.Size(),
		})
	}
	return result, nil
}

// FileExists checks if a file or directory exists within the sandbox.
func (s *Sandbox) FileExists(relPath string) (bool, error) {
	safePath, err := s.ValidatePath(relPath)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(safePath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("sandbox: stat %q: %w", relPath, err)
	}
	return true, nil
}

// StatFile returns file metadata for a path within the sandbox.
func (s *Sandbox) StatFile(relPath string) (*FileInfo, error) {
	safePath, err := s.ValidatePath(relPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(safePath)
	if err != nil {
		return nil, fmt.Errorf("sandbox: stat %q: %w", relPath, err)
	}
	return &FileInfo{
		Name:  info.Name(),
		IsDir: info.IsDir(),
		Size:  info.Size(),
	}, nil
}

// FileInfo holds basic metadata about a file or directory.
type FileInfo struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

// GrepSearch searches for a regex or string pattern across the sandbox directory.
// It applies the "Junk Shield" to skip banned directories and extensions,
// and enforces a hard limit of 50 matches to protect LLM context windows.
func (s *Sandbox) GrepSearch(relDir, pattern string) (string, error) {
	safePath, err := s.ValidatePath(relDir)
	if err != nil {
		return "", err
	}

	// Try to compile as regex, fallback to string contains if invalid
	re, regexpErr := regexp.Compile(pattern)

	var results []string
	matchCount := 0
	maxMatches := 50

	err = filepath.WalkDir(safePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		if d.IsDir() {
			if bannedDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(d.Name()))
		if bannedExts[ext] {
			return nil
		}

		// Open and scan the file line-by-line
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		relFilePath := strings.TrimPrefix(path, s.root+string(filepath.Separator))
		scanner := bufio.NewScanner(f)
		lineNum := 1

		for scanner.Scan() {
			if matchCount >= maxMatches {
				return nil // Stop searching this file if we hit the limit
			}

			line := scanner.Text()
			isMatch := false

			if regexpErr == nil {
				isMatch = re.MatchString(line)
			} else {
				isMatch = strings.Contains(strings.ToLower(line), strings.ToLower(pattern))
			}

			if isMatch {
				snippet := strings.TrimSpace(line)
				results = append(results, fmt.Sprintf("File: %s | Line %d: %s", relFilePath, lineNum, snippet))
				matchCount++
			}
			lineNum++
		}

		return nil
	})

	if err != nil {
		return "", fmt.Errorf("grep_search: %w", err)
	}

	if matchCount == 0 {
		return "No matches found for pattern: " + pattern, nil
	}

	output := strings.Join(results, "\n")
	if matchCount >= maxMatches {
		output += "\n... [Search truncated. Maximum of 50 matches reached. Be more specific.]"
	}

	return output, nil
}
