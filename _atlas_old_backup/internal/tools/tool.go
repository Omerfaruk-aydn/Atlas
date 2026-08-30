// Package tools defines Atlas's tool-calling interface and the registry
// that maps tool names to implementations. Native tools (file/shell/web)
// implement Tool directly; MCP-backed tools (Phase 5) will wrap remote
// servers behind the same interface so the agent never needs to know the
// difference.
package tools

import (
	"context"
	"encoding/json"
)

// Result is what a Tool reports back to the model after execution.
type Result struct {
	Content string
	IsError bool
}

// Tool is one capability the agent can invoke mid-conversation.
type Tool interface {
	Name() string
	Description() string
	// InputSchema is a JSON Schema object describing the tool's arguments.
	InputSchema() json.RawMessage
	// RequiresApproval reports whether this tool must be confirmed by the
	// user before running (mutating/executing tools) or may run silently
	// (read-only tools like reading a file or fetching a URL).
	RequiresApproval() bool
	Execute(ctx context.Context, input json.RawMessage) (Result, error)
}

// Preview describes a tool's effect before it runs, as a before/after pair
// the caller can diff. Path is shown as a header; Old is "" for a new file.
type Preview struct {
	Path string
	Old  string
	New  string
}

// Previewable is implemented by tools that can describe their effect ahead
// of time (e.g. a file diff) without actually running. When a tool
// requiring approval also implements this, the agent includes the preview
// in its EventApprovalRequest so the TUI can render a diff instead of raw
// JSON input.
type Previewable interface {
	Preview(input json.RawMessage) (Preview, error)
}
