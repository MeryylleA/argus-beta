package tools

import (
	"fmt"
	"os/exec"
	"strings"
)

// SearchCodeTool uses ripgrep (rg) to search for patterns in source code.
//
// WHY RIPGREP:
// - Respects .gitignore by default (skips vendor, node_modules, etc.)
// - Extremely fast even on large repositories
// - Supports regex patterns that are useful for vulnerability hunting
// - Has built-in safeguards (binary file detection, max filesize)
//
// SECURITY:
// - The search path is validated against the sandbox
// - rg is invoked with --no-follow to prevent symlink traversal
// - Max results are capped to prevent memory exhaustion
// - The pattern is passed as an argument (not shell-interpreted)
type SearchCodeTool struct {
	sandbox *Sandbox
}

func NewSearchCodeTool(sandbox *Sandbox) *SearchCodeTool {
	return &SearchCodeTool{sandbox: sandbox}
}

func (t *SearchCodeTool) Name() string { return "search_code" }

func (t *SearchCodeTool) Description() string {
	return "Search for a regex pattern in source code files using ripgrep. " +
	"Returns matching lines with file paths and line numbers. " +
	"Useful for finding function definitions, API endpoints, auth patterns, " +
	"hardcoded secrets, and vulnerability indicators."
}

func (t *SearchCodeTool) Schema() ToolSchema {
	return ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: map[string]ParamDef{
			"pattern": {
				Type:        "string",
				Description: "Regex pattern to search for (ripgrep syntax)",
			},
			"path": {
				Type:        "string",
				Description: "Directory to search in (relative to repo root, or absolute within repo)",
			},
			"max_results": {
				Type:        "integer",
				Description: "Maximum number of matching lines to return (default: 50, max: 200)",
			},
		},
		Required: []string{"pattern"},
	}
}

func (t *SearchCodeTool) Execute(params map[string]any) (ToolResult, error) {
	// Extract and validate parameters
	pattern, err := extractString(params, "pattern", true)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}, nil
	}

	searchPath, err := extractString(params, "path", false)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}, nil
	}

	maxResults, err := extractInt(params, "max_results", false, 50)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Cap max results to prevent memory exhaustion.
	// An LLM requesting 10000 results would just waste context window anyway.
	if maxResults > 200 {
		maxResults = 200
	}
	if maxResults < 1 {
		maxResults = 50
	}

	// Determine the search directory
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

	// Check that ripgrep is installed
	rgPath, err := exec.LookPath("rg")
	if err != nil {
		return ToolResult{
			Content: "ripgrep (rg) is not installed. Please install it:\n" +
			"  macOS:  brew install ripgrep\n" +
			"  Ubuntu: sudo apt install ripgrep\n" +
			"  Arch:   sudo pacman -S ripgrep\n" +
			"  Or visit: https://github.com/BurntSushi/ripgrep#installation",
			IsError: true,
		}, nil
	}

	ctx := extractContext(params)

	// Build the ripgrep command with security-focused flags.
	// --no-follow: Do NOT follow symlinks (prevents sandbox escape)
	// --max-count: Limit matches per file to prevent one huge file
	//              from consuming all results
	// --max-filesize: Skip files larger than 1MB (likely not source code)
	// --color=never: No ANSI codes in output (clean for LLM consumption)
	// -n: Show line numbers (critical for the agent to reference locations)
	// -i: Case insensitive (more useful for security pattern matching)
	args := []string{
		"--no-follow",    // SECURITY: never follow symlinks
		"--color=never",  // Clean output for parsing
		"-n",             // Show line numbers
		"-i",             // Case insensitive search
		"--max-filesize", "1M", // Skip huge files
		"--max-count", "10", // Max 10 matches per file
		fmt.Sprintf("--max-count=%d", maxResults), // This is per-file, we handle total below
		pattern,
		targetPath,
	}

	// We use exec.CommandContext so the 30s timeout kills the process
	cmd := exec.CommandContext(ctx, rgPath, args...)

	// SECURITY: Do not inherit the current environment wholesale.
	// Set a minimal environment to prevent information leakage.
	cmd.Env = []string{
		"PATH=/usr/bin:/usr/local/bin:/bin",
		"HOME=/tmp",
	}

	output, err := cmd.Output()

	// ripgrep exits with code 1 when no matches are found — this is not an error
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				return ToolResult{
					Content: "No matches found.",
					Metadata: map[string]any{
						"match_count": 0,
						"pattern":     pattern,
						"path":        targetPath,
					},
				}, nil
			}
			// Exit code 2 = error in ripgrep (bad regex, etc.)
			return ToolResult{
				Content: fmt.Sprintf("ripgrep error: %s\nstderr: %s", err, string(exitErr.Stderr)),
				IsError: true,
			}, nil
		}
		return ToolResult{
			Content: fmt.Sprintf("Failed to execute ripgrep: %s", err),
			IsError: true,
		}, nil
	}

	// Process output: limit total lines returned
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	totalMatches := len(lines)
	if totalMatches > maxResults {
		lines = lines[:maxResults]
	}

	result := strings.Join(lines, "\n")

	// Make paths relative to sandbox root for cleaner output.
	// The agent doesn't need to see the full absolute path —
	// it makes the output verbose and leaks host filesystem info.
	result = strings.ReplaceAll(result, t.sandbox.Root()+"/", "")

	return ToolResult{
		Content: result,
		Metadata: map[string]any{
			"match_count":  totalMatches,
			"showing":      len(lines),
			"pattern":      pattern,
			"path":         targetPath,
			"capped":       totalMatches > maxResults,
		},
	}, nil
}
