package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/argus-beta/argus/internal/core/tools"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
		case "tool":
			handleToolCommand(os.Args[2:])
		case "tools":
			handleListTools(os.Args[2:])
		default:
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
			printUsage()
			os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Argus — Security Intelligence Platform

	Usage:
	argus tool <tool_name> [args...]   Execute a tool directly
	argus tools [repo_path]            List available tools and their schemas

	Tool Commands:
	argus tool search_code <pattern> <path> [max_results]
	argus tool view_lines <file> <start_line> <end_line>
	argus tool find_files <path> [pattern] [extension]
	argus tool directory_tree <path> [depth]
	argus tool git_log <path> [count]

	Examples:
	argus tool search_code "auth|middleware" /path/to/repo 20
	argus tool view_lines /path/to/repo/main.go 1 50
	argus tool directory_tree /path/to/repo 3
	argus tool find_files /path/to/repo "config" "yaml"
	argus tool git_log /path/to/repo 10
	`)
}

func handleListTools(args []string) {
	repoPath := "."
	if len(args) > 0 {
		repoPath = args[0]
	}

	logger := tools.NewToolLogger(false)
	executor, err := tools.NewToolExecutor(repoPath, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	schemas := executor.Schemas()
	output, _ := json.MarshalIndent(schemas, "", "  ")
	fmt.Println(string(output))
}

func handleToolCommand(args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Error: tool name required\n\n")
		printUsage()
		os.Exit(1)
	}

	toolName := args[0]
	params := make(map[string]any)

	// Parse tool-specific arguments into the params map.
	// This is a CLI convenience layer — in production, the agent loop
	// will construct params directly from the LLM's tool call output.
	switch toolName {
		case "search_code":
			if len(args) < 3 {
				fmt.Fprintf(os.Stderr, "Usage: argus tool search_code <pattern> <path> [max_results]\n")
				os.Exit(1)
			}
			params["pattern"] = args[1]
			// For search_code, we need to create the executor with the repo path
			repoPath := args[2]
			maxResults := 50
			if len(args) > 3 {
				if n, err := strconv.Atoi(args[3]); err == nil {
					maxResults = n
				}
			}
			params["max_results"] = maxResults

			executeTool(repoPath, toolName, params)
			return

		case "view_lines":
			if len(args) < 4 {
				fmt.Fprintf(os.Stderr, "Usage: argus tool view_lines <file> <start_line> <end_line>\n")
				os.Exit(1)
			}
			filePath := args[1]
			startLine, err := strconv.Atoi(args[2])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: start_line must be an integer\n")
				os.Exit(1)
			}
			endLine, err := strconv.Atoi(args[3])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: end_line must be an integer\n")
				os.Exit(1)
			}

			// Infer repo root from file path (use directory containing the file)
			repoPath := inferRepoRoot(filePath)
			params["file"] = filePath
			params["start_line"] = startLine
			params["end_line"] = endLine

			executeTool(repoPath, toolName, params)
			return

		case "find_files":
			if len(args) < 2 {
				fmt.Fprintf(os.Stderr, "Usage: argus tool find_files <path> [pattern] [extension]\n")
				os.Exit(1)
			}
			repoPath := args[1]
			if len(args) > 2 {
				params["pattern"] = args[2]
			}
			if len(args) > 3 {
				params["extension"] = args[3]
			}

			executeTool(repoPath, toolName, params)
			return

		case "directory_tree":
			if len(args) < 2 {
				fmt.Fprintf(os.Stderr, "Usage: argus tool directory_tree <path> [depth]\n")
				os.Exit(1)
			}
			repoPath := args[1]
			if len(args) > 2 {
				if n, err := strconv.Atoi(args[2]); err == nil {
					params["depth"] = n
				}
			}

			executeTool(repoPath, toolName, params)
			return

		case "git_log":
			if len(args) < 2 {
				fmt.Fprintf(os.Stderr, "Usage: argus tool git_log <path> [count]\n")
				os.Exit(1)
			}
			repoPath := args[1]
			if len(args) > 2 {
				if n, err := strconv.Atoi(args[2]); err == nil {
					params["count"] = n
				}
			}

			executeTool(repoPath, toolName, params)
			return

		default:
			fmt.Fprintf(os.Stderr, "Unknown tool: %s\n", toolName)
			os.Exit(1)
	}
}

func executeTool(repoPath, toolName string, params map[string]any) {
	logger := tools.NewToolLogger(true)
	executor, err := tools.NewToolExecutor(repoPath, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating executor: %s\n", err)
		os.Exit(1)
	}

	result, err := executor.Execute(toolName, params)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Execution error: %s\n", err)
		os.Exit(1)
	}

	if result.IsError {
		fmt.Fprintf(os.Stderr, "Tool error: %s\n", result.Content)
		os.Exit(1)
	}

	fmt.Println(result.Content)
}

// inferRepoRoot attempts to find the repository root from a file path.
// Walks up the directory tree looking for .git directory.
// Falls back to the file's parent directory.
func inferRepoRoot(filePath string) string {
	// Simple heuristic: walk up looking for .git
	dir := filePath
	if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
		dir = filePath[:strings.LastIndex(filePath, "/")]
	}

	current := dir
	for {
		if _, err := os.Stat(current + "/.git"); err == nil {
			return current
		}
		parent := current[:strings.LastIndex(current, "/")]
		if parent == current || parent == "" {
			break
		}
		current = parent
	}

	// Fallback to the directory containing the file
	return dir
}
