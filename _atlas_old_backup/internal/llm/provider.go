// Package llm defines a provider-agnostic abstraction over chat-completion
// LLM APIs (Anthropic, OpenAI, Gemini, ...). The agent core only ever talks
// to the Provider interface below.
package llm

import (
	"context"
	"encoding/json"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// ContentBlock is one piece of a message: plain text, a tool invocation
// requested by the model ("tool_use"), or the result an executed tool
// reports back ("tool_result").
type ContentBlock struct {
	Type string // "text" | "tool_use" | "tool_result"
	Text string

	// tool_use fields (model → us)
	ToolUseID string
	ToolName  string
	ToolInput json.RawMessage

	// tool_result fields (us → model)
	ToolResultID      string
	ToolResultContent string
	ToolResultIsError bool
}

type Message struct {
	Role    Role
	Content []ContentBlock
}

func TextMessage(role Role, text string) Message {
	return Message{Role: role, Content: []ContentBlock{{Type: "text", Text: text}}}
}

// ToolDef describes one tool the model may call, in JSON-Schema terms
// shared across providers.
type ToolDef struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

type Request struct {
	System    string
	Messages  []Message
	Tools     []ToolDef
	MaxTokens int64
	Model     string
}

type StreamEventType string

const (
	EventTextDelta StreamEventType = "text_delta"
	EventToolCall  StreamEventType = "tool_call"
	EventUsage     StreamEventType = "usage"
	EventTurnEnd   StreamEventType = "turn_end"
	EventError     StreamEventType = "error"
)

type Usage struct {
	InputTokens  int64
	OutputTokens int64
}

type StreamEvent struct {
	Type       StreamEventType
	TextDelta  string
	ToolCallID string
	ToolName   string
	ToolInput  json.RawMessage
	Usage      Usage
	StopReason string
	Err        error
}

type ModelInfo struct {
	ID            string
	ContextWindow int
}

// Provider is the single interface the agent core depends on. Concrete
// providers (anthropic, openai, gemini, ...) live in their own subpackages
// and translate to/from this normalized shape.
type Provider interface {
	Name() string
	StreamChat(ctx context.Context, req Request) (<-chan StreamEvent, error)

	// Model returns the model currently used for StreamChat calls.
	Model() string
	// SetModel switches the model used by subsequent StreamChat calls.
	SetModel(model string)
	// ListModels queries the provider's API for models available to the
	// configured API key.
	ListModels(ctx context.Context) ([]string, error)
}
