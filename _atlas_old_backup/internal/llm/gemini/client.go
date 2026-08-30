// Package gemini adapts Google's Gemini API to the llm.Provider
// interface. Unlike Anthropic/OpenAI, function calls arrive whole rather
// than as incremental JSON fragments, so EventToolCall is emitted in one
// shot per call instead of being assembled across several stream events.
package gemini

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"github.com/omerfarukaydin/atlas/internal/llm"
	"google.golang.org/genai"
)

type Client struct {
	apiKey string

	mu    sync.RWMutex
	model string
}

func New(apiKey, model string) *Client {
	return &Client{apiKey: apiKey, model: model}
}

func (c *Client) Name() string { return "gemini" }

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

func (c *Client) newGenaiClient(ctx context.Context) (*genai.Client, error) {
	return genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  c.apiKey,
		Backend: genai.BackendGeminiAPI,
	})
}

// ListModels returns model names (e.g. "gemini-2.5-pro") that support
// content generation, available to the configured API key.
func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	client, err := c.newGenaiClient(ctx)
	if err != nil {
		return nil, err
	}

	page, err := client.Models.List(ctx, &genai.ListModelsConfig{PageSize: 200})
	if err != nil {
		return nil, err
	}

	var names []string
	for _, m := range page.Items {
		if !supportsGenerateContent(m.SupportedActions) {
			continue
		}
		names = append(names, strings.TrimPrefix(m.Name, "models/"))
	}
	sort.Strings(names)
	return names, nil
}

func supportsGenerateContent(actions []string) bool {
	if len(actions) == 0 {
		return true // some models omit the field but still support it
	}
	for _, a := range actions {
		if a == "generateContent" {
			return true
		}
	}
	return false
}

func (c *Client) StreamChat(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
	client, err := c.newGenaiClient(ctx)
	if err != nil {
		return nil, err
	}

	model := c.Model()
	if req.Model != "" {
		model = req.Model
	}

	var contents []*genai.Content
	for _, m := range req.Messages {
		role := genai.RoleUser
		if m.Role == llm.RoleAssistant {
			role = genai.RoleModel
		}

		var parts []*genai.Part
		for _, c := range m.Content {
			switch c.Type {
			case "text":
				if c.Text != "" {
					parts = append(parts, genai.NewPartFromText(c.Text))
				}
			case "tool_use":
				var args map[string]any
				_ = json.Unmarshal(c.ToolInput, &args)
				parts = append(parts, genai.NewPartFromFunctionCall(c.ToolName, args))
			case "tool_result":
				parts = append(parts, genai.NewPartFromFunctionResponse(c.ToolName, map[string]any{
					"result": c.ToolResultContent,
				}))
			}
		}
		if len(parts) == 0 {
			continue
		}
		contents = append(contents, &genai.Content{Role: role, Parts: parts})
	}

	config := &genai.GenerateContentConfig{}
	if req.System != "" {
		config.SystemInstruction = genai.NewContentFromText(req.System, genai.Role(genai.RoleUser))
	}
	if req.MaxTokens > 0 {
		config.MaxOutputTokens = int32(req.MaxTokens)
	}
	if len(req.Tools) > 0 {
		var decls []*genai.FunctionDeclaration
		for _, t := range req.Tools {
			decl := &genai.FunctionDeclaration{
				Name:        t.Name,
				Description: t.Description,
			}
			if len(t.InputSchema) > 0 {
				var schema map[string]any
				if json.Unmarshal(t.InputSchema, &schema) == nil {
					decl.ParametersJsonSchema = schema
				}
			}
			decls = append(decls, decl)
		}
		config.Tools = []*genai.Tool{{FunctionDeclarations: decls}}
	}

	events := make(chan llm.StreamEvent)

	go func() {
		defer close(events)

		for resp, err := range client.Models.GenerateContentStream(ctx, model, contents, config) {
			if err != nil {
				select {
				case events <- llm.StreamEvent{Type: llm.EventError, Err: err}:
				case <-ctx.Done():
				}
				return
			}

			if text := resp.Text(); text != "" {
				if !emitGemini(ctx, events, llm.StreamEvent{Type: llm.EventTextDelta, TextDelta: text}) {
					return
				}
			}

			// Gemini delivers each function call whole rather than as
			// incremental JSON fragments, so we can emit EventToolCall
			// directly as soon as we see it.
			for _, cand := range resp.Candidates {
				if cand.Content == nil {
					continue
				}
				for _, part := range cand.Content.Parts {
					if part.FunctionCall == nil {
						continue
					}
					input, _ := json.Marshal(part.FunctionCall.Args)
					id := part.FunctionCall.ID
					if id == "" {
						id = part.FunctionCall.Name
					}
					if !emitGemini(ctx, events, llm.StreamEvent{
						Type:       llm.EventToolCall,
						ToolCallID: id,
						ToolName:   part.FunctionCall.Name,
						ToolInput:  input,
					}) {
						return
					}
				}
			}

			if resp.UsageMetadata != nil {
				if !emitGemini(ctx, events, llm.StreamEvent{
					Type: llm.EventTurnEnd,
					Usage: llm.Usage{
						InputTokens:  int64(resp.UsageMetadata.PromptTokenCount),
						OutputTokens: int64(resp.UsageMetadata.CandidatesTokenCount),
					},
				}) {
					return
				}
			}
		}
	}()

	return events, nil
}

func emitGemini(ctx context.Context, out chan<- llm.StreamEvent, ev llm.StreamEvent) bool {
	select {
	case out <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}
