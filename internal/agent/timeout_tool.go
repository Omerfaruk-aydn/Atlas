package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
)

// timeoutTool bounds how long one tool call may run. A tool that hangs --
// an LSP server that never answers, a fetch against a host that accepts the
// connection and then goes quiet -- otherwise stalls the turn with nothing
// for the model to react to.
type timeoutTool struct {
	inner   fantasy.AgentTool
	timeout time.Duration
}

// wrapToolsWithTimeout returns a tool slice with each entry bounded by
// timeout. A timeout of zero or less leaves the slice untouched: unbounded
// is the default, since a tool call that legitimately takes ten minutes is
// ordinary work in a large repository.
func wrapToolsWithTimeout(agentTools []fantasy.AgentTool, timeout time.Duration) []fantasy.AgentTool {
	if timeout <= 0 {
		return agentTools
	}
	out := make([]fantasy.AgentTool, len(agentTools))
	for i, tool := range agentTools {
		out[i] = &timeoutTool{inner: tool, timeout: timeout}
	}
	return out
}

func (t *timeoutTool) Info() fantasy.ToolInfo {
	return t.inner.Info()
}

func (t *timeoutTool) ProviderOptions() fantasy.ProviderOptions {
	return t.inner.ProviderOptions()
}

func (t *timeoutTool) SetProviderOptions(opts fantasy.ProviderOptions) {
	t.inner.SetProviderOptions(opts)
}

func (t *timeoutTool) Run(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	runCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	resp, err := t.inner.Run(runCtx, call)
	if !t.deadlineExceeded(ctx, runCtx, err) {
		return resp, err
	}
	// An error response rather than an error: the model can read this,
	// and try something narrower. A returned error would end the turn.
	return fantasy.NewTextErrorResponse(fmt.Sprintf(
		"Tool %q exceeded the configured %s timeout and was stopped.", call.Name, t.timeout)), nil
}

// deadlineExceeded distinguishes this tool's own timeout from a cancellation
// that came from above (the user pressing escape, or shutdown). Only the
// former is reported to the model as a timeout; the latter has to keep
// propagating as a cancellation.
//
// A tool that finished without an error keeps its result even if it ran
// past the deadline: the work is already done, and throwing it away helps
// nobody.
func (t *timeoutTool) deadlineExceeded(parent, runCtx context.Context, err error) bool {
	if err == nil || parent.Err() != nil {
		return false
	}
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(runCtx.Err(), context.DeadlineExceeded)
}
