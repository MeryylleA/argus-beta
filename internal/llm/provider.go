// Package llm provides a unified interface for LLM providers and a native
// implementation for Ollama (local and cloud models via open-weights).
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// --- Unified Types ---

// Role represents the role of a message participant.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message represents a single message in a conversation.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// ToolParam defines a tool that the LLM can invoke.
type ToolParam struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema
}

// StreamEvent represents a single event in a streaming LLM response.
type StreamEvent struct {
	Type    string `json:"type"` // "text_delta", "thinking_delta", "tool_use", "done", "error"
	Content string `json:"content,omitempty"`
	ToolID  string `json:"tool_id,omitempty"`
	Tool    string `json:"tool,omitempty"`
	Input   string `json:"input,omitempty"`

	Raw any `json:"-"`
}

// Provider defines the interface for an LLM backend.
type Provider interface {
	// StreamChat sends a conversation with tool definitions and streams the response.
	StreamChat(ctx context.Context, systemPrompt string, messages []Message, tools []ToolParam) (<-chan StreamEvent, error)

	// Name returns the human-readable provider name.
	Name() string

	// ModelID returns the model identifier used by this provider.
	ModelID() string
}

// --- Ollama Provider ---

// OllamaProvider implements Provider for Ollama instances (local or cloud).
type OllamaProvider struct {
	modelID string
	baseURL string
	client  *http.Client
}

// NewOllamaProvider creates a provider for Ollama.
// Reads OLLAMA_HOST from environment, defaulting to http://localhost:11434.
func NewOllamaProvider(modelID string) *OllamaProvider {
	baseURL := os.Getenv("OLLAMA_HOST")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	return &OllamaProvider{
		modelID: modelID,
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

func (p *OllamaProvider) Name() string    { return "ollama" }
func (p *OllamaProvider) ModelID() string { return p.modelID }

// ollamaMessage is the wire format for a message in the Ollama /api/chat endpoint.
type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ollamaTool is the wire format for a tool definition in the Ollama API.
type ollamaTool struct {
	Type     string             `json:"type"`
	Function ollamaToolFunction `json:"function"`
}

type ollamaToolFunction struct {
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// ollamaStreamChunk is the wire format for a single NDJSON line from Ollama.
type ollamaStreamChunk struct {
	Message struct {
		Role      string `json:"role"`
		Content   string `json:"content"`
		ToolCalls []struct {
			Function struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	} `json:"message"`
	Done  bool   `json:"done"`
	Error string `json:"error"`
}

// StreamChat streams a chat completion from an Ollama server.
// Reads NDJSON from /api/chat and maps each chunk to StreamEvent.
func (p *OllamaProvider) StreamChat(ctx context.Context, systemPrompt string, messages []Message, tools []ToolParam) (<-chan StreamEvent, error) {
	// Build message list with system prompt prepended.
	ollamaMsgs := make([]ollamaMessage, 0, len(messages)+1)
	if systemPrompt != "" {
		ollamaMsgs = append(ollamaMsgs, ollamaMessage{Role: "system", Content: systemPrompt})
	}
	for _, m := range messages {
		role := "user"
		if m.Role == RoleAssistant {
			role = "assistant"
		}
		ollamaMsgs = append(ollamaMsgs, ollamaMessage{Role: role, Content: m.Content})
	}

	// Convert tool definitions to Ollama format.
	ollamaTools := make([]ollamaTool, 0, len(tools))
	for _, t := range tools {
		ollamaTools = append(ollamaTools, ollamaTool{
			Type: "function",
			Function: ollamaToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters: map[string]any{
					"type":       "object",
					"properties": t.Parameters,
				},
			},
		})
	}

	// Build request body.
	reqBody := map[string]any{
		"model":    p.modelID,
		"messages": ollamaMsgs,
		"stream":   true,
	}
	if len(ollamaTools) > 0 {
		reqBody["tools"] = ollamaTools
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	endpoint := p.baseURL + "/api/chat"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("ollama: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("ollama: unexpected status %d from %s", resp.StatusCode, endpoint)
	}

	ch := make(chan StreamEvent, 64)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		// Increase buffer for large responses (1 MB).
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			var chunk ollamaStreamChunk
			if err := json.Unmarshal(line, &chunk); err != nil {
				continue // Skip malformed NDJSON lines.
			}

			// Handle server-side errors.
			if chunk.Error != "" {
				ch <- StreamEvent{Type: "error", Content: chunk.Error}
				return
			}

			// Emit text deltas.
			if chunk.Message.Content != "" {
				ch <- StreamEvent{Type: "text_delta", Content: chunk.Message.Content}
			}

			// Emit tool calls.
			for _, tc := range chunk.Message.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Function.Arguments)
				ch <- StreamEvent{
					Type:  "tool_use",
					Tool:  tc.Function.Name,
					Input: string(argsJSON),
				}
			}

			// Terminal chunk.
			if chunk.Done {
				ch <- StreamEvent{Type: "done"}
				return
			}
		}

		if err := scanner.Err(); err != nil {
			ch <- StreamEvent{Type: "error", Content: err.Error()}
		}
	}()

	return ch, nil
}
