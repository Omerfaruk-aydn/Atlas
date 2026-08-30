package agent

import "encoding/json"

// Event is emitted by the agent loop as a turn progresses. The TUI drains
// these off a channel and turns each one into a tea.Msg.
type Event struct {
	Type       EventType
	TextDelta  string
	InputTok   int64
	OutputTok  int64
	StopReason string
	Err        error

	// Tool-call fields, populated for EventToolStart, EventApprovalRequest,
	// and EventToolResult.
	ToolCallID  string
	ToolName    string
	ToolInput   json.RawMessage
	ToolOutput  string
	ToolIsError bool
	ElapsedMS   int64 // populated for EventAmbient

	// PreviewPath/PreviewOld/PreviewNew are populated for EventApprovalRequest
	// when the tool implements tools.Previewable, letting the TUI render a
	// diff instead of raw JSON input.
	PreviewPath string
	PreviewOld  string
	PreviewNew  string
}

type EventType string

const (
	EventTextDelta EventType = "text_delta"
	EventUsage     EventType = "usage"
	EventTurnDone  EventType = "turn_done"
	EventError     EventType = "error"

	// EventApprovalRequest is emitted before running a tool that requires
	// confirmation; the agent loop then blocks until Agent.Approve is
	// called with the user's decision.
	EventApprovalRequest EventType = "approval_request"
	// EventToolStart is emitted right before a tool actually executes
	// (after any approval has been granted).
	EventToolStart EventType = "tool_start"
	// EventToolResult is emitted once a tool call has finished.
	EventToolResult EventType = "tool_result"
	// EventAmbient is a one-off, non-blocking status line injected while a
	// tool call runs unusually long, so a slow tool doesn't read as a hang.
	EventAmbient EventType = "ambient"
)
