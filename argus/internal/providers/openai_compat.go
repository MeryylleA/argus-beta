package providers

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// OpenAICompatProvider implements Provider for all OpenAI-compatible APIs.
// Serves GPT-5.2, GLM-5, Kimi K2.5, and MiniMax M2.5 with configurable BaseURL.
type OpenAICompatProvider struct {
	client      *openai.Client
	modelID     string
	maxCtx      int
	inputCost   float64
	outputCost  float64
	extraParams map[string]any // injected into raw request (e.g. Kimi's thinking: disabled)
}

// NewOpenAICompatProvider creates a provider for any OpenAI-compatible API.
func NewOpenAICompatProvider(apiKey, modelID, baseURL string, extraParams map[string]any) *OpenAICompatProvider {
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
	)
	model := SupportedModels[modelID]
	return &OpenAICompatProvider{
		client:      &client,
		modelID:     modelID,
		maxCtx:      model.MaxContext,
		inputCost:   model.InputCostPerMTok,
		outputCost:  model.OutputCostPerMTok,
		extraParams: extraParams,
	}
}

func (p *OpenAICompatProvider) Name() string           { return "openai_compat" }
func (p *OpenAICompatProvider) ModelID() string         { return p.modelID }
func (p *OpenAICompatProvider) MaxContextTokens() int   { return p.maxCtx }

func (p *OpenAICompatProvider) Complete(ctx context.Context, req CompletionRequest) (<-chan Event, error) {
	messages := p.convertMessages(req.SystemPrompt, req.Messages)
	tools := p.convertTools(req.Tools)

	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(p.modelID),
		Messages: messages,
	}

	if req.MaxTokens > 0 {
		params.MaxCompletionTokens = openai.Int(int64(req.MaxTokens))
	}

	if len(tools) > 0 {
		params.Tools = tools
	}

	// Request usage in streaming response
	params.StreamOptions = &openai.ChatCompletionStreamOptionsParam{
		IncludeUsage: openai.Bool(true),
	}

	// Build request options for extra params (e.g. Kimi's thinking: disabled)
	var reqOpts []option.RequestOption
	for key, val := range p.extraParams {
		reqOpts = append(reqOpts, option.WithJSONSet(key, val))
	}

	stream := p.client.Chat.Completions.NewStreaming(ctx, params, reqOpts...)

	events := make(chan Event, 64)
	go p.processStream(stream, events)
	return events, nil
}

// toolCallAccumulator tracks a streaming tool call being built across chunks.
type toolCallAccumulator struct {
	id   string
	name string
	args strings.Builder
}

func (p *OpenAICompatProvider) processStream(stream *openai.ChatCompletionStream, events chan<- Event) {
	defer close(events)
	defer stream.Close()

	// Accumulate tool calls across streamed chunks.
	// A single response can contain multiple tool calls.
	toolCalls := make(map[int64]*toolCallAccumulator)
	var inputTokens, outputTokens int

	for stream.Next() {
		chunk := stream.Current()

		// Extract usage from the final chunk (when StreamOptions.IncludeUsage is set)
		if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
			inputTokens = int(chunk.Usage.PromptTokens)
			outputTokens = int(chunk.Usage.CompletionTokens)
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]

		// Stream text content
		if choice.Delta.Content != "" {
			events <- Event{Type: "text_delta", Text: choice.Delta.Content}
		}

		// Accumulate tool calls
		for _, tc := range choice.Delta.ToolCalls {
			idx := tc.Index
			acc, exists := toolCalls[idx]
			if !exists {
				acc = &toolCallAccumulator{}
				toolCalls[idx] = acc
			}
			if tc.ID != "" {
				acc.id = tc.ID
			}
			if tc.Function.Name != "" {
				acc.name = tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				acc.args.WriteString(tc.Function.Arguments)
			}
		}

		// Check for finish reason
		if choice.FinishReason == "tool_calls" {
			// Emit all accumulated tool calls
			for _, acc := range toolCalls {
				var params map[string]any
				if err := json.Unmarshal([]byte(acc.args.String()), &params); err != nil {
					params = map[string]any{"_raw": acc.args.String()}
				}
				events <- Event{
					Type: "tool_call",
					ToolCall: &ToolCall{
						ID:     acc.id,
						Name:   acc.name,
						Params: params,
					},
				}
			}
		}
	}

	if err := stream.Err(); err != nil {
		events <- Event{Type: "error", Error: err.Error()}
		return
	}

	cost := (float64(inputTokens)/1_000_000)*p.inputCost +
		(float64(outputTokens)/1_000_000)*p.outputCost

	events <- Event{
		Type: "done",
		Usage: &Usage{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			CostUSD:      cost,
		},
	}
}

// convertMessages translates provider-agnostic messages to OpenAI Chat Completion format.
func (p *OpenAICompatProvider) convertMessages(systemPrompt string, msgs []Message) []openai.ChatCompletionMessageParamUnion {
	var result []openai.ChatCompletionMessageParamUnion

	// System message first
	if systemPrompt != "" {
		result = append(result, openai.ChatCompletionMessageParamUnion{
			OfSystem: &openai.ChatCompletionSystemMessageParam{
				Content: openai.ChatCompletionSystemMessageParamContentUnion{
					OfString: openai.String(systemPrompt),
				},
			},
		})
	}

	for _, msg := range msgs {
		switch msg.Role {
		case "user":
			// Check if the message contains tool results
			hasToolResults := false
			for _, b := range msg.Content {
				if b.Type == "tool_result" {
					hasToolResults = true
					break
				}
			}

			if hasToolResults {
				// In OpenAI format, each tool result is a separate "tool" role message
				for _, b := range msg.Content {
					if b.Type == "tool_result" && b.ToolResult != nil {
						result = append(result, openai.ChatCompletionMessageParamUnion{
							OfTool: &openai.ChatCompletionToolMessageParam{
								ToolCallID: b.ToolResult.ToolCallID,
								Content: openai.ChatCompletionToolMessageParamContentUnion{
									OfString: openai.String(b.ToolResult.Content),
								},
							},
						})
					}
				}
			} else {
				// Regular user text message
				var text strings.Builder
				for _, b := range msg.Content {
					if b.Type == "text" {
						text.WriteString(b.Text)
					}
				}
				result = append(result, openai.ChatCompletionMessageParamUnion{
					OfUser: &openai.ChatCompletionUserMessageParam{
						Content: openai.ChatCompletionUserMessageParamContentUnion{
							OfString: openai.String(text.String()),
						},
					},
				})
			}

		case "assistant":
			assistantMsg := &openai.ChatCompletionAssistantMessageParam{}

			// Collect text
			var text strings.Builder
			for _, b := range msg.Content {
				if b.Type == "text" {
					text.WriteString(b.Text)
				}
			}
			if text.Len() > 0 {
				assistantMsg.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: openai.String(text.String()),
				}
			}

			// Collect tool calls
			for _, b := range msg.Content {
				if b.Type == "tool_call" && b.ToolCall != nil {
					argsJSON, _ := json.Marshal(b.ToolCall.Params)
					assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, openai.ChatCompletionMessageToolCallParam{
						ID: b.ToolCall.ID,
						Function: openai.ChatCompletionMessageToolCallFunctionParam{
							Name:      b.ToolCall.Name,
							Arguments: string(argsJSON),
						},
					})
				}
			}

			result = append(result, openai.ChatCompletionMessageParamUnion{
				OfAssistant: assistantMsg,
			})
		}
	}

	return result
}

// convertTools translates provider-agnostic tool definitions to OpenAI format.
func (p *OpenAICompatProvider) convertTools(defs []ToolDefinition) []openai.ChatCompletionToolParam {
	tools := make([]openai.ChatCompletionToolParam, 0, len(defs))

	for _, td := range defs {
		tools = append(tools, openai.ChatCompletionToolParam{
			Function: openai.FunctionDefinitionParam{
				Name:        td.Name,
				Description: openai.String(td.Description),
				Parameters:  openai.FunctionParameters(td.Parameters),
			},
		})
	}

	return tools
}
