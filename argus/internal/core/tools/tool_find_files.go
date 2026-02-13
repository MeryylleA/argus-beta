package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// FindFilesTool uses fd (preferred) or find (fallback) to locate files
// by name pattern or extension.
//
// This is typically the agent's first tool call — understanding the
// repository structure by finding configuration files, source files,
// and security-relevant files (Dockerfiles, CI configs, etc.)
//
// SECURITY:
// - Search path validated against sandbox
// - --no-follow flag prevents symlink traversal
// - Output paths are validated against sandbox (defense in depth)
type FindFilesTool struct {
	sandbox *Sandbox
}

func NewFindFilesTool(sandbox *Sandbox) *FindFilesTool {
	return &FindFilesTool{sandbox: sandbox}
}

func (t *FindFilesTool) Name() string { return "find_files" }

func (t *FindFilesTool) Description() string {
	return "Find files by name pattern or extension. " +
	"Uses fd if available, falls back to find. " +
	"Useful for discovering project structure, config files, " +
	"and security-relevant files (Dockerfile, .env.example, etc.)"
}

func (t *FindFilesTool) Schema() ToolSchema {
	return ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: map[string]ParamDef{
			"pattern": {
				Type:        "string",
				Description: "Filename pattern to search for (e.g., 'auth', 'middleware', 'config')",
			},
			"path": {
				Type:        "string",
				Description: "Directory to search in (relative to repo root)",
			},
			"extension": {
				Type:        "string",
				Description: "Filter by file extension without dot (e.g., 'go', 'py', 'yaml')",
			},
		},
		Required: []string{},
	}
}

func (t *FindFilesTool) Execute(params map[string]any) (ToolResult, error) {
	pattern, err := extractString(params, "pattern", false)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}, nil
	}

	searchPath, err := extractString(params, "path", false)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}, nil
	}

	extension, err := extractString(params, "extension", false)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// At least one filter must be provided
	if pattern == "" && extension == "" {
		return ToolResult{
			Content: "At least one of 'pattern' or 'extension' must be provided.",
			IsError: true,
		}, nil
	}

	// Resolve search path
	targetPath := t.sandbox.Root()
	if searchPath != "" {
		targetPath, err = t.sandbox.ValidatePath(searchPath)
		if err != nil {
			return ToolResult{
				Content: fmt.Sprintf("Path validation failed: %s", err),
				IsError: true,
			}, nil
		}
	}

	ctx := extractContext(params)

	// Try fd first, fall back to find.
	var output []byte
	if fdPath, fdErr := exec.LookPath("fd"); fdErr == nil {
		output, err = t.executeWithFd(ctx, fdPath, pattern, extension, targetPath)
	} else if findPath, findErr := exec.LookPath("find"); findErr == nil {
		output, err = t.executeWithFind(ctx, findPath, pattern, extension, targetPath)
	} else {
		return ToolResult{
			Content: "Neither fd nor find is available. Please install fd:\n" +
			"  macOS:  brew install fd\n" +
			"  Ubuntu: sudo apt install fd-find\n" +
			"  Arch:   sudo pacman -S fd\n" +
			"  Or visit: https://github.com/sharkdp/fd#installation",
			IsError: true,
		}, nil
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				return ToolResult{
					Content:  "No files found matching the criteria.",
					Metadata: map[string]any{"count": 0},
				}, nil
			}
			return ToolResult{
				Content: fmt.Sprintf("Search error: %s\nstderr: %s", err, string(exitErr.Stderr)),
				IsError: true,
			}, nil
		}
		return ToolResult{
			Content: fmt.Sprintf("Failed to execute file search: %s", err),
			IsError: true,
		}, nil
	}

	result := strings.TrimSpace(string(output))
	if result == "" {
		return ToolResult{
			Content:  "No files found matching the criteria.",
			Metadata: map[string]any{"count": 0},
		}, nil
	}

	// Make paths relative to sandbox root
	result = strings.ReplaceAll(result, t.sandbox.Root()+"/", "")

	lines := strings.Split(result, "\n")

	const maxFiles = 200
	if len(lines) > maxFiles {
		lines = lines[:maxFiles]
		result = strings.Join(lines, "\n")
		result += fmt.Sprintf("\n\n... and more (showing first %d results, narrow your search)", maxFiles)
	}

	return ToolResult{
		Content: result,
		Metadata: map[string]any{
			"count":     len(lines),
			"pattern":   pattern,
			"extension": extension,
			"path":      targetPath,
		},
	}, nil
}

func (t *FindFilesTool) executeWithFd(ctx context.Context, fdPath, pattern, extension, searchPath string) ([]byte, error) {
	args := []string{
		"--no-follow",   // SECURITY: don't follow symlinks
		"--color=never", // Clean output
		"--type=f",      // Files only
	}

	if extension != "" {
		args = append(args, "--extension", extension)
	}

	if pattern != "" {
		args = append(args, pattern)
	}

	args = append(args, searchPath)

	cmd := exec.CommandContext(ctx, fdPath, args...)
	cmd.Env = []string{
		"PATH=/usr/bin:/usr/local/bin:/bin",
		"HOME=/tmp",
	}

	return cmd.Output()
}

func (t *FindFilesTool) executeWithFind(ctx context.Context, findPath, pattern, extension, searchPath string) ([]byte, error) {
	// SECURITY: -P flag = never follow symlinks
	args := []string{
		"-P",
		searchPath,
		"-type", "f",
	}

	if pattern != "" && extension != "" {
		args = append(args, "-name", fmt.Sprintf("*%s*.%s", pattern, extension))
	} else if pattern != "" {
		args = append(args, "-name", fmt.Sprintf("*%s*", pattern))
	} else if extension != "" {
		args = append(args, "-name", fmt.Sprintf("*.%s", extension))
	}

	args = append(args, "-maxdepth", "20")

	cmd := exec.CommandContext(ctx, findPath, args...)
	cmd.Env = []string{
		"PATH=/usr/bin:/usr/local/bin:/bin",
		"HOME=/tmp",
	}

	return cmd.Output()
}
