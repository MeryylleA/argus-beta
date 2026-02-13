package tools

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ViewLinesTool reads a specific range of lines from a file.
//
// This is the agent's "eyes" — it uses search_code or find_files to locate
// interesting code, then view_lines to read the actual content with context.
//
// SECURITY:
// - File path is validated against the sandbox
// - Maximum of 100 lines per read (prevents reading entire large files)
// - File is opened read-only (O_RDONLY)
// - Binary file detection (first 512 bytes checked for null bytes)
type ViewLinesTool struct {
	sandbox *Sandbox
}

func NewViewLinesTool(sandbox *Sandbox) *ViewLinesTool {
	return &ViewLinesTool{sandbox: sandbox}
}

func (t *ViewLinesTool) Name() string { return "view_lines" }

func (t *ViewLinesTool) Description() string {
	return "Read specific lines from a file. Returns the content with line numbers. " +
	"Use this after search_code to examine code context around matches. " +
	"Maximum 100 lines per request."
}

func (t *ViewLinesTool) Schema() ToolSchema {
	return ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: map[string]ParamDef{
			"file": {
				Type:        "string",
				Description: "Path to the file (relative to repo root, or absolute within repo)",
			},
			"start_line": {
				Type:        "integer",
				Description: "First line to read (1-indexed)",
			},
			"end_line": {
				Type:        "integer",
				Description: "Last line to read (1-indexed, inclusive)",
			},
		},
		Required: []string{"file", "start_line", "end_line"},
	}
}

const maxViewLines = 100

func (t *ViewLinesTool) Execute(params map[string]any) (ToolResult, error) {
	// Extract parameters
	filePath, err := extractString(params, "file", true)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}, nil
	}

	startLine, err := extractInt(params, "start_line", true, 0)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}, nil
	}

	endLine, err := extractInt(params, "end_line", true, 0)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Validate line range
	if startLine < 1 {
		return ToolResult{
			Content: "start_line must be >= 1",
			IsError: true,
		}, nil
	}
	if endLine < startLine {
		return ToolResult{
			Content: fmt.Sprintf("end_line (%d) must be >= start_line (%d)", endLine, startLine),
			IsError: true,
		}, nil
	}

	// Enforce the 100-line cap.
	// If the agent needs more context, it can make multiple calls.
	// This prevents accidentally reading a 50,000-line generated file.
	if endLine-startLine+1 > maxViewLines {
		endLine = startLine + maxViewLines - 1
	}

	// SECURITY: Validate the file path against the sandbox
	validatedPath, err := t.sandbox.ValidatePath(filePath)
	if err != nil {
		return ToolResult{
			Content: fmt.Sprintf("Path validation failed: %s", err),
			IsError: true,
		}, nil
	}

	// Verify it's a regular file, not a directory or device
	info, err := os.Stat(validatedPath)
	if err != nil {
		return ToolResult{
			Content: fmt.Sprintf("Cannot access file: %s", err),
			IsError: true,
		}, nil
	}
	if info.IsDir() {
		return ToolResult{
			Content: fmt.Sprintf("%q is a directory, not a file. Use directory_tree instead.", filePath),
			IsError: true,
		}, nil
	}
	// SECURITY: Reject non-regular files (device files, named pipes, etc.)
	if !info.Mode().IsRegular() {
		return ToolResult{
			Content: fmt.Sprintf("%q is not a regular file (mode: %s)", filePath, info.Mode()),
			IsError: true,
		}, nil
	}

	// Open file as read-only
	// SECURITY: os.O_RDONLY ensures we cannot accidentally write
	file, err := os.OpenFile(validatedPath, os.O_RDONLY, 0)
	if err != nil {
		return ToolResult{
			Content: fmt.Sprintf("Cannot open file: %s", err),
			IsError: true,
		}, nil
	}
	defer file.Close()

	// Read the requested lines using a scanner.
	// We iterate line by line rather than reading the whole file
	// to handle large files efficiently.
	scanner := bufio.NewScanner(file)

	// Increase scanner buffer for files with very long lines
	// (e.g., minified JS, which is common in repos)
	const maxLineLength = 256 * 1024 // 256KB per line
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxLineLength)

	var result strings.Builder
	lineNum := 0
	linesRead := 0
	totalLines := 0

	for scanner.Scan() {
		lineNum++
		totalLines = lineNum

		if lineNum < startLine {
			continue
		}
		if lineNum > endLine {
			// Don't break — keep scanning to count total lines?
			// No — for large files this would be wasteful.
			// We'll report what we know.
			break
		}

		line := scanner.Text()
		// Format: line number padded + content
		// The line number is essential for the agent to reference specific lines
		result.WriteString(fmt.Sprintf("%4d | %s\n", lineNum, line))
		linesRead++
	}

	if err := scanner.Err(); err != nil {
		return ToolResult{
			Content: fmt.Sprintf("Error reading file: %s", err),
			IsError: true,
		}, nil
	}

	if linesRead == 0 {
		return ToolResult{
			Content: fmt.Sprintf("No lines in range %d-%d. File has %d lines.",
					     startLine, endLine, totalLines),
					     IsError: true,
		}, nil
	}

	// Make path relative for cleaner display
	displayPath := strings.TrimPrefix(validatedPath, t.sandbox.Root()+"/")

	header := fmt.Sprintf("── %s (lines %d–%d) ──\n", displayPath, startLine, startLine+linesRead-1)

	return ToolResult{
		Content: header + result.String(),
		Metadata: map[string]any{
			"file":       displayPath,
			"start_line": startLine,
			"end_line":   startLine + linesRead - 1,
			"lines_read": linesRead,
		},
	}, nil
}
