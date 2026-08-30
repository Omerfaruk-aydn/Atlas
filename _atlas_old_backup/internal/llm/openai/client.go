// Package openai adapts the OpenAI Chat Completions API to the
// llm.Provider interface.
package openai

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"github.com/omerfarukaydin/atlas/internal/llm"
	sdk "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
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

func (c *Client) Name() string { return "openai" }

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

// nonChatModelHints filters out models that can't serve chat completions
// (audio/image/embedding/moderation endpoints) from ListModels' output.
var nonChatModelHints = []string{"whisper", "tts", "dall-e", "embedding", "moderation", "davinci", "babbage"}

// ListModels returns chat-capable model IDs available to the configured API key.
func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	var ids []string
	page := c.client.Models.ListAutoPaging(ctx)
	for page.Next() {
		id := page.Current().ID
		if isChatModel(id) {
			ids = append(ids, id)
		}
	}
	if err := page.Err(); err != nil {
		return nil, err
	}
	sort.Strings(ids)
	return ids, nil
}

func isChatModel(id string) bool {
	lower := strings.ToLower(id)
	for _, hint := range nonChatModelHints {
		if strings.Contains(lower, hint) {
			return false
		}
	}
	return true
}

func (c *Client) StreamChat(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
	params := toParams(req, c.Model())
	events := make(chan llm.StreamEvent)

	go func() {
		defer close(events)

		// OpenAI streams each tool call's id/name once and its arguments as
		// repeated string fragments, all keyed by the call's index within
		// the response; there's no explicit "block closed" marker, so we
		// accumulate across the whole stream and emit once it ends.
		toolCalls := map[int64]*pendingToolCall{}
		var toolOrder []int64

		stream := c.client.Chat.Completions.NewStreaming(ctx, params)
		for stream.Next() {
			chunk := stream.Current()

			// The IncludeUsage stream option delivers one extra chunk at the
			// end with populated Usage and empty Choices.
			if chunk.Usage.TotalTokens > 0 {
				if !emitOpenAI(ctx, events, llm.StreamEvent{
					Type: llm.EventTurnEnd,
					Usage: llm.Usage{
						InputTokens:  chunk.Usage.PromptTokens,
						OutputTokens: chunk.Usage.CompletionTokens,
					},
				}) {
					return
				}
			}

			for _, choice := range chunk.Choices {
				if choice.Delta.Content != "" {
					if !emitOpenAI(ctx, events, llm.StreamEvent{Type: llm.EventTextDelta, TextDelta: choice.Delta.Content}) {
						return
					}
				}
				for _, tc := range choice.Delta.ToolCalls {
					p, ok := toolCalls[tc.Index]
					if !ok {
						p = &pendingToolCall{}
						toolCalls[tc.Index] = p
						toolOrder = append(toolOrder, tc.Index)
					}
					if tc.ID != "" {
						p.id = tc.ID
					}
					if tc.Function.Name != "" {
						p.name = tc.Function.Name
					}
					p.argsBuf.WriteString(tc.Function.Arguments)
				}
			}
		}

		for _, idx := range toolOrder {
			p := toolCalls[idx]
			args := p.argsBuf.String()
			if args == "" {
				args = "{}"
			}
			if !emitOpenAI(ctx, events, llm.StreamEvent{
				Type:       llm.EventToolCall,
				ToolCallID: p.id,
				ToolName:   p.name,
				ToolInput:  json.RawMessage(args),
			}) {
				return
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

func emitOpenAI(ctx context.Context, out chan<- llm.StreamEvent, ev llm.StreamEvent) bool {
	select {
	case out <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}

// pendingToolCall accumulates one streamed tool call's id/name/arguments
// across however many chunks OpenAI splits them into.
type pendingToolCall struct {
	id, name string
	argsBuf  strings.Builder
}

func toParams(req llm.Request, model string) sdk.ChatCompletionNewParams {
	if req.Model != "" {
		model = req.Model
	}

	params := sdk.ChatCompletionNewParams{
		Model:         model,
		StreamOptions: sdk.ChatCompletionStreamOptionsParam{IncludeUsage: sdk.Bool(true)},
	}
	if req.MaxTokens > 0 {
		params.MaxCompletionTokens = sdk.Int(req.MaxTokens)
	}
	if req.System != "" {
		params.Messages = append(params.Messages, sdk.SystemMessage(req.System))
	}
	for _, t := range req.Tools {
		fn := sdk.FunctionDefinitionParam{
			Name:        t.Name,
			Description: sdk.String(t.Description),
			Parameters:  toFunctionParameters(t.InputSchema),
		}
		params.Tools = append(params.Tools, sdk.ChatCompletionToolUnionParam{
			OfFunction: &sdk.ChatCompletionFunctionToolParam{Function: fn},
		})
	}

	for _, m := range req.Messages {
		var text string
		var toolCalls []sdk.ChatCompletionMessageToolCallUnionParam
		var toolResults []llm.ContentBlock

		for _, c := range m.Content {
			switch c.Type {
			case "text":
				text += c.Text
			case "tool_use":
				toolCalls = append(toolCalls, sdk.ChatCompletionMessageToolCallUnionParam{
					OfFunction: &sdk.ChatCompletionMessageFunctionToolCallParam{
						ID: c.ToolUseID,
						Function: sdk.ChatCompletionMessageFunctionToolCallFunctionParam{
							Name:      c.ToolName,
							Arguments: string(c.ToolInput),
						},
					},
				})
			case "tool_result":
				toolResults = append(toolResults, c)
			}
		}

		switch {
		case len(toolResults) > 0:
			for _, tr := range toolResults {
				params.Messages = append(params.Messages, sdk.ToolMessage(tr.ToolResultContent, tr.ToolResultID))
			}
		case len(toolCalls) > 0:
			am := sdk.ChatCompletionAssistantMessageParam{ToolCalls: toolCalls}
			if text != "" {
				am.Content = sdk.ChatCompletionAssistantMessageParamContentUnion{OfString: sdk.String(text)}
			}
			params.Messages = append(params.Messages, sdk.ChatCompletionMessageParamUnion{OfAssistant: &am})
		case m.Role == llm.RoleUser:
			params.Messages = append(params.Messages, sdk.UserMessage(text))
		default:
			params.Messages = append(params.Messages, sdk.AssistantMessage(text))
		}
	}

	return params
}

func toFunctionParameters(schema json.RawMessage) sdk.FunctionParameters {
	if len(schema) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(schema, &m); err != nil {
		return nil
	}
	return sdk.FunctionParameters(m)
}
