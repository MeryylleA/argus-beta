package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DirectoryTreeTool generates a tree-style directory listing.
//
// This tool is implemented in pure Go (no external dependencies)
// because it's simple enough and avoids requiring the `tree` command.
//
// SECURITY:
// - Path validated against sandbox
// - Depth is capped to prevent excessive traversal
// - Symlinks are NOT followed (os.ReadDir returns DirEntry, we check type)
// - Maximum number of entries to prevent memory exhaustion on huge repos
type DirectoryTreeTool struct {
	sandbox *Sandbox
}

func NewDirectoryTreeTool(sandbox *Sandbox) *DirectoryTreeTool {
	return &DirectoryTreeTool{sandbox: sandbox}
}

func (t *DirectoryTreeTool) Name() string { return "directory_tree" }

func (t *DirectoryTreeTool) Description() string {
	return "Show the directory structure as a tree. " +
	"Useful for understanding project layout, finding important directories " +
	"(src, lib, config, test, etc.), and planning investigation strategy."
}

func (t *DirectoryTreeTool) Schema() ToolSchema {
	return ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: map[string]ParamDef{
			"path": {
				Type:        "string",
				Description: "Directory to show (relative to repo root)",
			},
			"depth": {
				Type:        "integer",
				Description: "Maximum depth to traverse (default: 3, max: 6)",
			},
		},
		Required: []string{},
	}
}

const (
	maxTreeDepth   = 6
	maxTreeEntries = 500 // Prevent memory exhaustion on monorepos
)

func (t *DirectoryTreeTool) Execute(params map[string]any) (ToolResult, error) {
	dirPath, err := extractString(params, "path", false)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}, nil
	}

	depth, err := extractInt(params, "depth", false, 3)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Cap depth to prevent excessive traversal.
	// Depth > 6 on a real repo would produce thousands of lines
	// that would waste the LLM's context window.
	if depth > maxTreeDepth {
		depth = maxTreeDepth
	}
	if depth < 1 {
		depth = 3
	}

	// Resolve and validate path
	targetPath := t.sandbox.Root()
	if dirPath != "" {
		targetPath, err = t.sandbox.ValidatePath(dirPath)
		if err != nil {
			return ToolResult{
				Content: fmt.Sprintf("Path validation failed: %s", err),
				IsError: true,
			}, nil
		}
	}

	// Verify it's a directory
	info, err := os.Stat(targetPath)
	if err != nil {
		return ToolResult{
			Content: fmt.Sprintf("Cannot access path: %s", err),
			IsError: true,
		}, nil
	}
	if !info.IsDir() {
		return ToolResult{
			Content: fmt.Sprintf("%q is not a directory", dirPath),
			IsError: true,
		}, nil
	}

	// Build the tree
	var builder strings.Builder
	entryCount := 0
	displayRoot := strings.TrimPrefix(targetPath, t.sandbox.Root()+"/")
	if displayRoot == t.sandbox.Root() {
		displayRoot = "."
	}
	builder.WriteString(displayRoot + "/\n")

	t.buildTree(&builder, targetPath, "", depth, &entryCount)

	if entryCount >= maxTreeEntries {
		builder.WriteString(fmt.Sprintf("\n... truncated (showing %d entries, use a narrower path or smaller depth)\n", maxTreeEntries))
	}

	return ToolResult{
		Content: builder.String(),
		Metadata: map[string]any{
			"path":        targetPath,
			"depth":       depth,
			"entry_count": entryCount,
			"truncated":   entryCount >= maxTreeEntries,
		},
	}, nil
}

// buildTree recursively builds the tree output.
// Uses standard tree-drawing characters for familiar formatting.
func (t *DirectoryTreeTool) buildTree(builder *strings.Builder, dirPath, prefix string, remainingDepth int, entryCount *int) {
	if remainingDepth <= 0 || *entryCount >= maxTreeEntries {
		return
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		builder.WriteString(prefix + "└── [error reading directory]\n")
		return
	}

	// Sort entries: directories first, then files, both alphabetically.
	// This gives the agent a consistent, predictable layout.
	sort.Slice(entries, func(i, j int) bool {
		iDir := entries[i].IsDir()
		jDir := entries[j].IsDir()
		if iDir != jDir {
			return iDir // directories first
		}
		return entries[i].Name() < entries[j].Name()
	})

	// Filter out common noise directories that aren't useful for analysis.
	// These can contain thousands of files and waste context.
	filtered := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if !shouldSkipEntry(entry.Name()) {
			filtered = append(filtered, entry)
		}
	}

	for i, entry := range filtered {
		*entryCount++
		if *entryCount >= maxTreeEntries {
			return
		}

		isLast := i == len(filtered)-1
		connector := "├── "
		childPrefix := "│   "
		if isLast {
			connector = "└── "
			childPrefix = "    "
		}

		name := entry.Name()

		// SECURITY: Check if entry is a symlink and mark it.
		// We intentionally do NOT follow symlinks in the tree.
		if entry.Type()&os.ModeSymlink != 0 {
			// Read the symlink target for informational purposes only
			target, err := os.Readlink(filepath.Join(dirPath, name))
			if err != nil {
				name += " -> [unreadable symlink]"
			} else {
				name += " -> " + target + " [symlink, not followed]"
			}
			builder.WriteString(prefix + connector + name + "\n")
			continue
		}

		if entry.IsDir() {
			builder.WriteString(prefix + connector + name + "/\n")
			t.buildTree(builder, filepath.Join(dirPath, name), prefix+childPrefix, remainingDepth-1, entryCount)
		} else {
			builder.WriteString(prefix + connector + name + "\n")
		}
	}
}

// shouldSkipEntry returns true for directories that are typically noise
// in a security analysis context. These contain dependencies, build artifacts,
// or VCS internals that aren't part of the project's own codebase.
func shouldSkipEntry(name string) bool {
	skipDirs := map[string]bool{
		"node_modules": true,
		".git":         true,
		"vendor":       true,
		"__pycache__":  true,
		".idea":        true,
		".vscode":      true,
		".DS_Store":    true,
		"dist":         true,
		"build":        true,
		".next":        true,
		".nuxt":        true,
		".cache":       true,
	}
	return skipDirs[name]
}
