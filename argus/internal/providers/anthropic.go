package providers

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicProvider implements Provider for Claude models.
// Uses the official Anthropic SDK with streaming.
type AnthropicProvider struct {
	client     *anthropic.Client
	modelID    string
	maxCtx     int
	inputCost  float64
	outputCost float64
}

// NewAnthropicProvider creates a provider for Anthropic models.
func NewAnthropicProvider(apiKey, modelID string) *AnthropicProvider {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	model := SupportedModels[modelID]
	return &AnthropicProvider{
		client:     &client,
		modelID:    modelID,
		maxCtx:     model.MaxContext,
		inputCost:  model.InputCostPerMTok,
		outputCost: model.OutputCostPerMTok,
	}
}

func (p *AnthropicProvider) Name() string           { return "anthropic" }
func (p *AnthropicProvider) ModelID() string         { return p.modelID }
func (p *AnthropicProvider) MaxContextTokens() int   { return p.maxCtx }

func (p *AnthropicProvider) Complete(ctx context.Context, req CompletionRequest) (<-chan Event, error) {
	messages := p.convertMessages(req.Messages)
	tools := p.convertTools(req.Tools)

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(p.modelID),
		MaxTokens: int64(req.MaxTokens),
		Messages:  messages,
		Tools:     tools,
	}

	if req.SystemPrompt != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: req.SystemPrompt},
		}
	}

	stream := p.client.Messages.NewStreaming(ctx, params)

	events := make(chan Event, 64)
	go p.processStream(stream, events)
	return events, nil
}

func (p *AnthropicProvider) processStream(stream *anthropic.MessageStream, events chan<- Event) {
	defer close(events)
	defer stream.Close()

	accum := anthropic.Message{}

	// Track current tool call being built from streaming blocks
	var currentToolID string
	var currentToolName string
	var jsonBuf strings.Builder

	for stream.Next() {
		evt := stream.Current()
		_ = accum.Accumulate(evt)

		switch variant := evt.AsAny().(type) {
		case anthropic.ContentBlockStartEvent:
			// Check if this is a tool_use block
			switch cb := variant.ContentBlock.AsAny().(type) {
			case anthropic.ToolUseBlock:
				currentToolID = cb.ID
				currentToolName = cb.Name
				jsonBuf.Reset()
			}

		case anthropic.ContentBlockDeltaEvent:
			switch delta := variant.Delta.AsAny().(type) {
			case anthropic.TextDelta:
				events <- Event{Type: "text_delta", Text: delta.Text}
			case anthropic.InputJSONDelta:
				jsonBuf.WriteString(delta.PartialJSON)
			}

		case anthropic.ContentBlockStopEvent:
			if currentToolID != "" {
				var params map[string]any
				if err := json.Unmarshal([]byte(jsonBuf.String()), &params); err != nil {
					params = map[string]any{"_raw": jsonBuf.String()}
				}
				events <- Event{
					Type: "tool_call",
					ToolCall: &ToolCall{
						ID:     currentToolID,
						Name:   currentToolName,
						Params: params,
					},
				}
				currentToolID = ""
				currentToolName = ""
			}
		}
	}

	if err := stream.Err(); err != nil {
		events <- Event{Type: "error", Error: err.Error()}
		return
	}

	inputTokens := int(accum.Usage.InputTokens)
	outputTokens := int(accum.Usage.OutputTokens)
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

// convertMessages translates provider-agnostic messages to Anthropic format.
func (p *AnthropicProvider) convertMessages(msgs []Message) []anthropic.MessageParam {
	result := make([]anthropic.MessageParam, 0, len(msgs))

	for _, msg := range msgs {
		var blocks []anthropic.ContentBlockParamUnion

		for _, b := range msg.Content {
			switch b.Type {
			case "text":
				blocks = append(blocks, anthropic.NewTextBlock(b.Text))

			case "tool_call":
				if b.ToolCall != nil {
					inputJSON, _ := json.Marshal(b.ToolCall.Params)
					blocks = append(blocks, anthropic.ContentBlockParamUnion{
						OfRequestToolUseBlock: &anthropic.ToolUseBlockParam{
							ID:    b.ToolCall.ID,
							Name:  b.ToolCall.Name,
							Input: json.RawMessage(inputJSON),
						},
					})
				}

			case "tool_result":
				if b.ToolResult != nil {
					blocks = append(blocks, anthropic.NewToolResultBlock(
						b.ToolResult.ToolCallID,
						b.ToolResult.Content,
						b.ToolResult.IsError,
					))
				}
			}
		}

		result = append(result, anthropic.MessageParam{
			Role:    anthropic.MessageParamRole(msg.Role),
			Content: blocks,
		})
	}

	return result
}

// convertTools translates provider-agnostic tool definitions to Anthropic format.
func (p *AnthropicProvider) convertTools(defs []ToolDefinition) []anthropic.ToolUnionParam {
	tools := make([]anthropic.ToolUnionParam, 0, len(defs))

	for _, td := range defs {
		// Extract properties from the JSON Schema
		properties, _ := td.Parameters["properties"]

		tool := anthropic.ToolParam{
			Name:        td.Name,
			Description: anthropic.String(td.Description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: properties,
			},
		}

		tools = append(tools, anthropic.ToolUnionParam{OfTool: &tool})
	}

	return tools
}
