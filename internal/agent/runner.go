package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/argus-sec/argus/internal/llm"
	"github.com/argus-sec/argus/internal/logger"
	"github.com/argus-sec/argus/internal/sse"
)

// Runner executes the main agent loop: it communicates with the LLM,
// executes sandboxed tools, and pushes events to the SSE channel.
// Fully stateless — no database dependency.
type Runner struct {
	sandbox  *Sandbox
	provider llm.Provider
	eventCh  chan sse.Event
}

// NewRunner creates a new agent runner.
func NewRunner(sandbox *Sandbox, provider llm.Provider, eventCh chan sse.Event) *Runner {
	return &Runner{
		sandbox:  sandbox,
		provider: provider,
		eventCh:  eventCh,
	}
}

// Run is the main agent loop. It sends prompts to the LLM, processes tool calls,
// and streams everything to the SSE channel.
func (r *Runner) Run(ctx context.Context, sessionID string, agentRole string) error {
	r.publishEvent(sessionID, "session_start", map[string]string{
		"session_id": sessionID,
		"agent_role": agentRole,
		"model":      r.provider.ModelID(),
	})

	logger.Info("Starting new %s session (ID: %s)", agentRole, sessionID)

	// Initialize persistent scratchpad (Infinite Memory whiteboard).
	scratchpad := ""

	// Initial user message to kick off the agent.
	messages := []llm.Message{
		{
			Role:    llm.RoleUser,
			Content: r.buildInitialPrompt(agentRole),
		},
	}

	baseSystemPrompt := r.buildSystemPrompt(agentRole)
	tools := r.getToolDefinitions()

	// Main agent loop: iterate until done, error, or context cancellation.
	maxIterations := 50
	for i := 0; i < maxIterations; i++ {
		select {
		case <-ctx.Done():
			r.publishEvent(sessionID, "completed", map[string]string{
				"message": "Scan cancelled",
			})
			return nil
		default:
		}

		// --- Context Compaction (The Janitor) ---
		// When the conversation grows too long, keep the first message (initial
		// prompt) and the last 10 messages (most recent context), discarding
		// everything in between to prevent context window overflow.
		if len(messages) > 30 {
			originalLen := len(messages)
			tail := 10
			if tail > len(messages)-1 {
				tail = len(messages) - 1
			}
			compacted := make([]llm.Message, 0, 1+tail)
			compacted = append(compacted, messages[0])
			compacted = append(compacted, messages[len(messages)-tail:]...)
			messages = compacted
			logger.Memory("Context compacted: removed %d old messages to prevent overflow", (originalLen - len(messages)))
		}

		r.publishEvent(sessionID, "thinking", map[string]string{
			"iteration": fmt.Sprintf("%d", i+1),
		})

		// --- Inject Whiteboard into System Prompt ---
		// Build a per-iteration system prompt that always includes the latest
		// scratchpad contents so the LLM never loses persistent memory.
		systemPrompt := baseSystemPrompt
		if scratchpad != "" {
			systemPrompt += "\n\n=== SHARED WHITEBOARD (PERSISTENT MEMORY) ===\n" + scratchpad + "\n===============================================\n"
		}

		// Stream LLM response.
		streamCh, err := r.provider.StreamChat(ctx, systemPrompt, messages, tools)
		if err != nil {
			return fmt.Errorf("agent: LLM stream error: %w", err)
		}

		// Accumulate response and detect tool calls.
		var fullText strings.Builder
		var toolCalls []toolCall
		var currentToolCall *toolCall
		var toolInputBuf strings.Builder

		for event := range streamCh {
			switch event.Type {
			case "text_delta":
				fullText.WriteString(event.Content)
				r.publishEvent(sessionID, "thought", map[string]string{
					"delta": event.Content,
				})

			case "thinking_delta":
				r.publishEvent(sessionID, "thinking", map[string]string{
					"delta": event.Content,
				})

			case "tool_use":
				if currentToolCall != nil {
					currentToolCall.Input = toolInputBuf.String()
					toolCalls = append(toolCalls, *currentToolCall)
					toolInputBuf.Reset()
				}
				currentToolCall = &toolCall{
					ID:   event.ToolID,
					Name: event.Tool,
				}
				if event.Input != "" {
					toolInputBuf.WriteString(event.Input)
				}
				r.publishEvent(sessionID, "tool_call", map[string]string{
					"tool": event.Tool,
				})

			case "tool_input_delta":
				toolInputBuf.WriteString(event.Input)

			case "done":
				if currentToolCall != nil {
					currentToolCall.Input = toolInputBuf.String()
					toolCalls = append(toolCalls, *currentToolCall)
				}

			case "error":
				return fmt.Errorf("agent: LLM error: %s", event.Content)
			}
		}

		// Add assistant response to conversation.
		assistantContent := fullText.String()
		if assistantContent != "" {
			messages = append(messages, llm.Message{
				Role:    llm.RoleAssistant,
				Content: assistantContent,
			})

			// Hallucination Fallback Parser: catch findings in plain text.
			if strings.Contains(assistantContent, "report_finding") && strings.Contains(assistantContent, "{") {
				re := regexp.MustCompile(`(?s)(?:\[Tool Result:\s*report_finding\]|report_finding).*?(\{.*?\})`)
				matches := re.FindStringSubmatch(assistantContent)
				if len(matches) > 1 {
					var finding struct {
						Title       string `json:"title"`
						Severity    string `json:"severity"`
						Description string `json:"description"`
						FilePath    string `json:"file_path"`
					}
					if err := json.Unmarshal([]byte(matches[1]), &finding); err == nil {
						r.publishEvent(sessionID, "finding_reported", map[string]string{
							"title":    finding.Title,
							"severity": finding.Severity,
							"file":     finding.FilePath,
							"desc":     finding.Description,
						})
					}
				}
			}
		}

		// If no tool calls, the agent is done.
		if len(toolCalls) == 0 {
			r.publishEvent(sessionID, "completed", map[string]string{
				"message": "Agent completed analysis",
			})
			return nil
		}

		// Execute tool calls within the sandbox.
		for _, tc := range toolCalls {
			result, err := r.executeTool(sessionID, tc, &scratchpad)
			if err != nil {
				r.publishEvent(sessionID, "tool_error", map[string]string{
					"tool":  tc.Name,
					"error": err.Error(),
				})
				result = fmt.Sprintf("Error: %s", err.Error())
			} else {
				r.publishEvent(sessionID, "tool_result", map[string]string{
					"tool":   tc.Name,
					"result": truncate(result, 500),
				})
			}

			// Feed tool result back to the conversation.
			messages = append(messages, llm.Message{
				Role:    llm.RoleUser,
				Content: fmt.Sprintf("[Tool Result: %s]\n%s", tc.Name, result),
			})
		}
	}

	// Max iterations reached.
	r.publishEvent(sessionID, "completed", map[string]string{
		"message": "Max iterations reached",
	})
	return nil
}

// publishEvent sends a structured SSE event on the channel (non-blocking).
func (r *Runner) publishEvent(sessionID, event string, data map[string]string) {
	jsonData, _ := json.Marshal(data)
	r.eventCh <- sse.Event{
		Event: event,
		Data:  string(jsonData),
	}
}

// --- Tool execution ---

type toolCall struct {
	ID    string
	Name  string
	Input string // JSON string
}

// executeTool dispatches a tool call to the appropriate sandboxed handler.
func (r *Runner) executeTool(sessionID string, tc toolCall, scratchpad *string) (string, error) {
	var params map[string]any
	if tc.Input != "" {
		if err := json.Unmarshal([]byte(tc.Input), &params); err != nil {
			return "", fmt.Errorf("invalid tool input JSON: %w", err)
		}
	}

	switch tc.Name {
	case "read_file":
		path, _ := params["path"].(string)
		if path == "" {
			return "", fmt.Errorf("read_file: 'path' parameter is required")
		}
		data, err := r.sandbox.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(data), nil

	case "list_directory":
		path, _ := params["path"].(string)
		if path == "" {
			path = "."
		}
		entries, err := r.sandbox.ListDir(path)
		if err != nil {
			return "", err
		}
		result, _ := json.MarshalIndent(entries, "", "  ")
		return string(result), nil

	case "file_exists":
		path, _ := params["path"].(string)
		if path == "" {
			return "", fmt.Errorf("file_exists: 'path' parameter is required")
		}
		exists, err := r.sandbox.FileExists(path)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%t", exists), nil

	case "search_code":
		pattern, _ := params["pattern"].(string)
		path, _ := params["path"].(string)
		if pattern == "" {
			return "", fmt.Errorf("search_code: 'pattern' parameter is required")
		}
		if path == "" {
			path = "."
		}
		return r.searchCode(path, pattern)

	case "grep_search":
		pattern, _ := params["pattern"].(string)
		if pattern == "" {
			return "", fmt.Errorf("grep_search: 'pattern' parameter is required")
		}
		// targetPath could be passed in if the original script tracked it, but for our implementation,
		// we'll use root "." or just let the LLM define it. We'll stick to root "." for broad sweeps via grep_search.
		return r.sandbox.GrepSearch(".", pattern)

	case "report_finding":
		title, _ := params["title"].(string)
		severity, _ := params["severity"].(string)
		description, _ := params["description"].(string)
		filePath, _ := params["file_path"].(string)
		r.publishEvent(sessionID, "finding_reported", map[string]string{
			"title":    title,
			"severity": severity,
			"file":     filePath,
			"desc":     description,
		})
		return "Finding successfully reported.", nil

	case "submit_summary":
		overallRisk, _ := params["overall_risk"].(string)
		summary, _ := params["summary"].(string)
		attackChain, _ := params["attack_chain"].(string)
		r.publishEvent(sessionID, "scan_summary", map[string]string{
			"overall_risk": overallRisk,
			"summary":      summary,
			"attack_chain": attackChain,
		})
		return "Summary successfully submitted.", nil

	case "update_memory":
		content, _ := params["content"].(string)
		if content == "" {
			return "", fmt.Errorf("update_memory: 'content' parameter is required")
		}
		*scratchpad += "\n" + content
		logger.Memory("Whiteboard updated. Total scratchpad size: %d bytes", len(*scratchpad))
		return "Memory updated successfully.", nil

	default:
		return "", fmt.Errorf("unknown tool: %s", tc.Name)
	}
}

// searchCode performs a simple recursive text search within the sandbox.
func (r *Runner) searchCode(relDir, pattern string) (string, error) {
	safePath, err := r.sandbox.ValidatePath(relDir)
	if err != nil {
		return "", err
	}

	var results []string
	maxResults := 100

	err = walkFiles(safePath, func(path string, content []byte) {
		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			if len(results) >= maxResults {
				return
			}
			if strings.Contains(line, pattern) {
				relPath := strings.TrimPrefix(path, r.sandbox.Root()+"/")
				results = append(results, fmt.Sprintf("%s:%d: %s", relPath, i+1, strings.TrimSpace(line)))
			}
		}
	})
	if err != nil {
		return "", fmt.Errorf("search_code: %w", err)
	}

	if len(results) == 0 {
		return "No matches found.", nil
	}
	return strings.Join(results, "\n"), nil
}

// getToolDefinitions returns the tool schemas available to the agent.
func (r *Runner) getToolDefinitions() []llm.ToolParam {
	return []llm.ToolParam{
		{
			Name:        "read_file",
			Description: "Read the contents of a file within the workspace. Path must be relative to the workspace root. Files larger than 2MB will be rejected.",
			Parameters: map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Relative path to the file from the workspace root.",
				},
			},
		},
		{
			Name:        "list_directory",
			Description: "List the contents of a directory within the workspace. Returns file names, types, and sizes.",
			Parameters: map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Relative path to the directory. Use '.' for the workspace root.",
				},
			},
		},
		{
			Name:        "file_exists",
			Description: "Check if a file or directory exists within the workspace.",
			Parameters: map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Relative path to check.",
				},
			},
		},
		{
			Name:        "search_code",
			Description: "Search for a text pattern in source files within the workspace. Returns matching lines with file paths and line numbers.",
			Parameters: map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "The text pattern to search for (case-sensitive substring match).",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Relative directory path to search within. Defaults to workspace root.",
				},
			},
		},
		{
			Name:        "grep_search",
			Description: "High-speed radar for the AI agent. Search for an exact string or regex pattern across the entire codebase.",
			Parameters: map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "The exact string or regex pattern to search for across the codebase (e.g., 'password', 'os.system', 'SELECT \\*').",
				},
			},
		},
		{
			Name:        "report_finding",
			Description: "Report a verified security vulnerability. Use this ONLY when you have confirmed a finding.",
			Parameters: map[string]any{
				"title": map[string]any{
					"type":        "string",
					"description": "Short, descriptive title of the vulnerability.",
				},
				"severity": map[string]any{
					"type":        "string",
					"description": "Severity level: critical, high, medium, low, info.",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Detailed explanation of the vulnerability and its impact.",
				},
				"file_path": map[string]any{
					"type":        "string",
					"description": "Path to the file containing the vulnerability.",
				},
				"evidence": map[string]any{
					"type":        "string",
					"description": "Code snippet or proof-of-concept demonstrating the issue.",
				},
			},
		},
		{
			Name:        "submit_summary",
			Description: "Submit an executive overview of the security posture. Use this EXACTLY ONCE when you have completely finished reporting all individual findings.",
			Parameters: map[string]any{
				"overall_risk": map[string]any{
					"type":        "string",
					"description": "The overall risk level: Critical, High, Medium, Low.",
				},
				"summary": map[string]any{
					"type":        "string",
					"description": "A concise executive explanation of the overall security posture and what was found.",
				},
				"attack_chain": map[string]any{
					"type":        "string",
					"description": "A step-by-step hypothetical scenario showing how an attacker could chain these findings together.",
				},
			},
		},
		{
			Name:        "update_memory",
			Description: "Update your persistent shared whiteboard. Use this to save important discoveries, file paths, or state that you need to remember across a long session. Writing here prevents you from forgetting things when the conversation history gets truncated.",
			Parameters: map[string]any{
				"content": map[string]any{
					"type":        "string",
					"description": "The text to add or update on the whiteboard.",
				},
			},
		},
	}
}

// --- System & initial prompts ---

func (r *Runner) buildSystemPrompt(agentRole string) string {
	switch agentRole {
	case "recon":
		return `You are the Argus Recon Agent, a fast and focused security reconnaissance AI.
Your mission is to map the codebase structure, identify technology stacks, discover entry points,
and catalog potential attack surfaces. You are thorough but efficient.

Rules:
- Use the provided tools to explore the workspace filesystem.
- Start by listing the root directory, then systematically explore relevant subdirectories.
- Identify: languages, frameworks, configuration files, API endpoints, authentication mechanisms,
  database connections, third-party dependencies, and any hardcoded secrets or credentials.
- Report your findings clearly and concisely.
- Do NOT attempt to exploit anything — your role is reconnaissance only.
- When you have completed your analysis, provide a structured summary of your findings.

CRITICAL RULE: DO NOT write vulnerability reports or tables in plain text/markdown.
If you find a vulnerability, a hardcoded secret, or a misconfiguration, you MUST IMMEDIATELY call the ` + "`report_finding`" + ` tool.
You must call the ` + "`report_finding`" + ` tool multiple times if you find multiple issues.
Your text response should only be used for your internal thinking process. ALL actual findings MUST be submitted via the ` + "`report_finding`" + ` tool.

When you have completely finished your analysis and reported all individual vulnerabilities via ` + "`report_finding`" + `, you MUST call the ` + "`submit_summary`" + ` tool EXACTLY ONCE to provide an executive overview and a realistic attack chain. Do NOT use ` + "`report_finding`" + ` to output your final summary.

CRITICAL: You are a fully autonomous security agent. NEVER ask the user for permission to proceed.
NEVER ask questions, make suggestions, or wait for human input.
If you see a file or directory of interest, IMMEDIATELY use the read_file or list_directory tools to investigate it.
You must chain your tool calls continuously. Only stop and provide a final summary when you have completely exhausted all possible paths of investigation.`

	case "exploit":
		return `You are the Argus Exploit Agent, a deep-analysis security AI specializing in vulnerability
verification and source-to-sink tracing.

Your mission is to perform thorough security analysis including:
- Identify and verify potential vulnerabilities (SQL injection, XSS, SSRF, path traversal, etc.)
- Trace data flow from sources (user input) to sinks (dangerous operations)
- Assess the severity and exploitability of each finding
- Provide proof-of-concept evidence where possible
- Check for authentication/authorization bypasses

Rules:
- Use the provided tools to read and search source code.
- Be methodical: trace each potential vulnerability from input to output.
- Classify findings by severity: critical, high, medium, low, info.
- Minimize false positives — verify before reporting.
- When you have completed your analysis, provide a structured report of verified findings.

CRITICAL RULE: DO NOT write vulnerability reports or tables in plain text/markdown.
If you find a vulnerability, a hardcoded secret, or a misconfiguration, you MUST IMMEDIATELY call the ` + "`report_finding`" + ` tool.
You must call the ` + "`report_finding`" + ` tool multiple times if you find multiple issues.
Your text response should only be used for your internal thinking process. ALL actual findings MUST be submitted via the ` + "`report_finding`" + ` tool.

When you have completely finished your analysis and reported all individual vulnerabilities via ` + "`report_finding`" + `, you MUST call the ` + "`submit_summary`" + ` tool EXACTLY ONCE to provide an executive overview and a realistic attack chain. Do NOT use ` + "`report_finding`" + ` to output your final summary.

CRITICAL: You are a fully autonomous security agent. NEVER ask the user for permission to proceed.
NEVER ask questions, make suggestions, or wait for human input.
If you see a file or directory of interest, IMMEDIATELY use the read_file or list_directory tools to investigate it.
You must chain your tool calls continuously. Only stop and provide a final summary when you have completely exhausted all possible paths of investigation.`

	default:
		return "You are a security analysis AI agent."
	}
}

func (r *Runner) buildInitialPrompt(agentRole string) string {
	switch agentRole {
	case "recon":
		return `Begin reconnaissance of the workspace. Start by listing the root directory structure,
then systematically explore the codebase to map out the technology stack, architecture,
entry points, and potential attack surfaces. Provide a comprehensive recon report when done.`

	case "exploit":
		return `Begin deep security analysis of the workspace. Systematically examine the codebase
for vulnerabilities. Trace data flows from user inputs to dangerous operations.
Verify each potential finding and classify by severity. Provide a detailed security
assessment report with evidence when done.`

	default:
		return "Analyze the workspace for security issues."
	}
}

// --- Helpers ---

func walkFiles(root string, fn func(path string, content []byte)) error {
	entries, err := readDirRecursive(root, 5)
	if err != nil {
		return err
	}
	for _, path := range entries {
		data, err := readFileSafe(path, maxFileReadSize)
		if err != nil {
			continue
		}
		if isBinary(data) {
			continue
		}
		fn(path, data)
	}
	return nil
}

func readDirRecursive(dir string, maxDepth int) ([]string, error) {
	if maxDepth <= 0 {
		return nil, nil
	}
	entries, err := readDir(dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		full := dir + "/" + e
		info, err := statFile(full)
		if err != nil {
			continue
		}
		if info.isDir {
			if strings.HasPrefix(e, ".") || e == "node_modules" || e == "vendor" || e == "__pycache__" {
				continue
			}
			sub, _ := readDirRecursive(full, maxDepth-1)
			paths = append(paths, sub...)
		} else {
			paths = append(paths, full)
		}
	}
	return paths, nil
}

type fileStat struct {
	isDir bool
	size  int64
}

func readDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}

func statFile(path string) (*fileStat, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return &fileStat{isDir: info.IsDir(), size: info.Size()}, nil
}

func readFileSafe(path string, maxSize int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxSize {
		return nil, fmt.Errorf("file too large: %d bytes", info.Size())
	}
	return os.ReadFile(path)
}

func isBinary(data []byte) bool {
	checkLen := len(data)
	if checkLen > 512 {
		checkLen = 512
	}
	for i := 0; i < checkLen; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
