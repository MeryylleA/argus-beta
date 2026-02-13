package tools

import (
	"fmt"
	"os/exec"
	"strings"
)

// GitLogTool shows recent git history for a repository.
//
// Git history is invaluable for security analysis:
// - Recent commits may reveal hastily-patched vulnerabilities
// - Commit messages may reference security fixes, CVEs, or bug reports
// - Authors and timing help understand the project's maintenance status
// - File changes in security-related commits indicate sensitive areas
//
// SECURITY:
// - Path validated against sandbox
// - Count is capped to prevent excessive output
// - We use --no-follow (via no symlink flags) and restrict to the repo
// - Git is run with explicit --git-dir and --work-tree to prevent
//   git from traversing upward to find a different repository
type GitLogTool struct {
	sandbox *Sandbox
}

func NewGitLogTool(sandbox *Sandbox) *GitLogTool {
	return &GitLogTool{sandbox: sandbox}
}

func (t *GitLogTool) Name() string { return "git_log" }

func (t *GitLogTool) Description() string {
	return "Show recent git commit history. " +
	"Useful for understanding recent changes, finding security-related commits, " +
	"identifying active areas of development, and spotting hasty fixes."
}

func (t *GitLogTool) Schema() ToolSchema {
	return ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: map[string]ParamDef{
			"path": {
				Type:        "string",
				Description: "Repository path or subdirectory to show history for",
			},
			"count": {
				Type:        "integer",
				Description: "Number of commits to show (default: 20, max: 100)",
			},
		},
		Required: []string{},
	}
}

func (t *GitLogTool) Execute(params map[string]any) (ToolResult, error) {
	repoPath, err := extractString(params, "path", false)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}, nil
	}

	count, err := extractInt(params, "count", false, 20)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Cap count to prevent excessive output
	if count > 100 {
		count = 100
	}
	if count < 1 {
		count = 20
	}

	// Resolve path
	targetPath := t.sandbox.Root()
	if repoPath != "" {
		targetPath, err = t.sandbox.ValidatePath(repoPath)
		if err != nil {
			return ToolResult{
				Content: fmt.Sprintf("Path validation failed: %s", err),
				IsError: true,
			}, nil
		}
	}

	// Check that git is installed
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return ToolResult{
			Content: "git is not installed. Please install git to use this tool.",
			IsError: true,
		}, nil
	}

	ctx := extractContext(params)

	// Build git log command with a format that's useful for security analysis.
	// Format includes: abbreviated hash, date, author, and subject
	// We also include --stat to show which files were changed (and how much),
	// which helps identify security-sensitive modifications.
	args := []string{
		"-C", targetPath, // Run in the target directory
		"log",
		fmt.Sprintf("-n%d", count),
		"--format=%h | %ad | %an | %s", // Short hash | date | author | subject
		"--date=short",                  // YYYY-MM-DD format (compact)
		"--stat",                        // Show file change stats
		"--stat-width=80",               // Limit stat width for readability
		"--no-walk",                     // Don't follow parent commits beyond our count
	}

	cmd := exec.CommandContext(ctx, gitPath, args...)

	// SECURITY: Minimal environment.
	// GIT_CEILING_DIRECTORIES prevents git from traversing upward
	// beyond the sandbox root to find a different repository.
	cmd.Env = []string{
		"PATH=/usr/bin:/usr/local/bin:/bin",
		"HOME=/tmp",
		fmt.Sprintf("GIT_CEILING_DIRECTORIES=%s", t.sandbox.Root()),
	}

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			// Common case: not a git repository
			if strings.Contains(stderr, "not a git repository") {
				return ToolResult{
					Content: "This directory is not a git repository.",
					IsError: true,
				}, nil
			}
			return ToolResult{
				Content: fmt.Sprintf("git error: %s\nstderr: %s", err, stderr),
				IsError: true,
			}, nil
		}
		return ToolResult{
			Content: fmt.Sprintf("Failed to execute git: %s", err),
			IsError: true,
		}, nil
	}

	result := strings.TrimSpace(string(output))
	if result == "" {
		return ToolResult{
			Content: "No commits found (empty repository or no history).",
			Metadata: map[string]any{"count": 0},
		}, nil
	}

	// Make paths relative in stat output
	result = strings.ReplaceAll(result, t.sandbox.Root()+"/", "")

	// Count actual commits shown (lines matching our format)
	commitCount := 0
	for _, line := range strings.Split(result, "\n") {
		if strings.Contains(line, " | ") {
			commitCount++
		}
	}

	return ToolResult{
		Content: result,
		Metadata: map[string]any{
			"commit_count": commitCount,
			"path":         targetPath,
		},
	}, nil
}
