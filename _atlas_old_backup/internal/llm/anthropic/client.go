// Package anthropic adapts the Anthropic Claude API to the llm.Provider
// interface.
package anthropic

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/omerfarukaydin/atlas/internal/llm"
)

type Client struct {
	client sdk.Client

	mu    sync.RWMutex
	model string
}

func New(apiKey, model string) *Client {
	return &Client{
		client: sdk.NewClient(option.WithAPIKey(apiKey)),
		model:  model,
	}
}

func (c *Client) Name() string { return "anthropic" }

func (c *Client) Model() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.model
}

func (c *Client) SetModel(model string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.model = model
}

// ListModels returns the model IDs available to the configured API key.
func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	var ids []string
	iter := c.client.Models.ListAutoPaging(ctx, sdk.ModelListParams{})
	for iter.Next() {
		ids = append(ids, iter.Current().ID)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	sort.Strings(ids)
	return ids, nil
}

func (c *Client) StreamChat(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
	params := toParams(req, c.Model())
	events := make(chan llm.StreamEvent)

	go func() {
		defer close(events)

		// Anthropic streams tool_use input as incremental JSON fragments
		// (input_json_delta) keyed by content-block index; accumulate them
		// here and emit a single llm.EventToolCall once the block closes.
		toolByIndex := map[int64]*pendingToolCall{}

		stream := c.client.Messages.NewStreaming(ctx, params)
		for stream.Next() {
			ev := stream.Current()
			switch variant := ev.AsAny().(type) {
			case sdk.ContentBlockStartEvent:
				if variant.ContentBlock.Type == "tool_use" {
					toolByIndex[variant.Index] = &pendingToolCall{
						id:   variant.ContentBlock.ID,
						name: variant.ContentBlock.Name,
					}
				}
			case sdk.ContentBlockDeltaEvent:
				switch variant.Delta.Type {
				case "text_delta":
					events <- llm.StreamEvent{Type: llm.EventTextDelta, TextDelta: variant.Delta.Text}
				case "input_json_delta":
					if p, ok := toolByIndex[variant.Index]; ok {
						p.jsonBuf.WriteString(variant.Delta.PartialJSON)
					}
				}
			case sdk.ContentBlockStopEvent:
				if p, ok := toolByIndex[variant.Index]; ok {
					input := p.jsonBuf.String()
					if input == "" {
						input = "{}"
					}
					events <- llm.StreamEvent{
						Type:       llm.EventToolCall,
						ToolCallID: p.id,
						ToolName:   p.name,
						ToolInput:  json.RawMessage(input),
					}
					delete(toolByIndex, variant.Index)
				}
			case sdk.MessageStartEvent:
				events <- llm.StreamEvent{
					Type:  llm.EventUsage,
					Usage: llm.Usage{InputTokens: variant.Message.Usage.InputTokens},
				}
			case sdk.MessageDeltaEvent:
				events <- llm.StreamEvent{
					Type:       llm.EventTurnEnd,
					StopReason: string(variant.Delta.StopReason),
					Usage:      llm.Usage{OutputTokens: variant.Usage.OutputTokens},
				}
			}
		}
		if err := stream.Err(); err != nil {
			select {
			case events <- llm.StreamEvent{Type: llm.EventError, Err: err}:
			case <-ctx.Done():
			}
		}
	}()

	return events, nil
}

// pendingToolCall accumulates one tool_use content block's streamed input
// JSON fragments until its content_block_stop event arrives.
type pendingToolCall struct {
	id, name string
	jsonBuf  strings.Builder
}

func toParams(req llm.Request, model string) sdk.MessageNewParams {
	if req.Model != "" {
		model = req.Model
	}

	params := sdk.MessageNewParams{
		MaxTokens: req.MaxTokens,
		Model:     model,
	}
	if req.System != "" {
		params.System = []sdk.TextBlockParam{{Text: req.System}}
	}
	for _, t := range req.Tools {
		tool := sdk.ToolParam{
			Name:        t.Name,
			Description: sdk.String(t.Description),
			InputSchema: toInputSchema(t.InputSchema),
		}
		params.Tools = append(params.Tools, sdk.ToolUnionParam{OfTool: &tool})
	}

	for _, m := range req.Messages {
		var blocks []sdk.ContentBlockParamUnion
		for _, c := range m.Content {
			switch c.Type {
			case "text":
				blocks = append(blocks, sdk.NewTextBlock(c.Text))
			case "tool_use":
				blocks = append(blocks, sdk.NewToolUseBlock(c.ToolUseID, json.RawMessage(c.ToolInput), c.ToolName))
			case "tool_result":
				blocks = append(blocks, sdk.NewToolResultBlock(c.ToolResultID, c.ToolResultContent, c.ToolResultIsError))
			}
		}
		if m.Role == llm.RoleUser {
			params.Messages = append(params.Messages, sdk.NewUserMessage(blocks...))
		} else {
			params.Messages = append(params.Messages, sdk.NewAssistantMessage(blocks...))
		}
	}

	return params
}

func toInputSchema(schema json.RawMessage) sdk.ToolInputSchemaParam {
	var parsed struct {
		Properties any      `json:"properties"`
		Required   []string `json:"required"`
	}
	if len(schema) > 0 {
		_ = json.Unmarshal(schema, &parsed)
	}
	return sdk.ToolInputSchemaParam{Properties: parsed.Properties, Required: parsed.Required}
}
