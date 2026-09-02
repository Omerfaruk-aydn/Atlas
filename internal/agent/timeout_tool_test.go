package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

type funcTool struct {
	name string
	run  func(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error)
}

func (f *funcTool) Info() fantasy.ToolInfo { return fantasy.ToolInfo{Name: f.name} }

func (f *funcTool) ProviderOptions() fantasy.ProviderOptions { return nil }

func (f *funcTool) SetProviderOptions(fantasy.ProviderOptions) {}

func (f *funcTool) Run(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	return f.run(ctx, call)
}

// A timeout of zero means unbounded, so nothing is wrapped at all.
func TestNoTimeoutLeavesToolsUntouched(t *testing.T) {
	inner := &funcTool{name: "view"}
	for _, d := range []time.Duration{0, -time.Second} {
		got := wrapToolsWithTimeout([]fantasy.AgentTool{inner}, d)
		require.Len(t, got, 1)
		require.Same(t, inner, got[0])
	}
}

func TestTimeoutToolPassesThroughAFastCall(t *testing.T) {
	wrapped := wrapToolsWithTimeout([]fantasy.AgentTool{&funcTool{
		name: "view",
		run: func(context.Context, fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return fantasy.NewTextResponse("the file"), nil
		},
	}}, time.Minute)

	resp, err := wrapped[0].Run(t.Context(), fantasy.ToolCall{Name: "view"})
	require.NoError(t, err)
	require.Equal(t, "the file", resp.Content)
	require.False(t, resp.IsError)
}

// A hung tool comes back as an error response, not an error: the model can
// read this and try something narrower, where a returned error would end
// the turn.
func TestTimeoutToolStopsAHangingCall(t *testing.T) {
	wrapped := wrapToolsWithTimeout([]fantasy.AgentTool{&funcTool{
		name: "fetch",
		run: func(ctx context.Context, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			<-ctx.Done()
			return fantasy.ToolResponse{}, ctx.Err()
		},
	}}, 10*time.Millisecond)

	resp, err := wrapped[0].Run(t.Context(), fantasy.ToolCall{Name: "fetch"})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "timeout")
	require.Contains(t, resp.Content, "fetch")
}

// A cancellation from above -- the user pressing escape -- must keep
// propagating as a cancellation rather than being reported as a timeout.
func TestTimeoutToolPropagatesAnOuterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	wrapped := wrapToolsWithTimeout([]fantasy.AgentTool{&funcTool{
		name: "fetch",
		run: func(ctx context.Context, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			<-ctx.Done()
			return fantasy.ToolResponse{}, ctx.Err()
		},
	}}, time.Minute)

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err := wrapped[0].Run(ctx, fantasy.ToolCall{Name: "fetch"})
	require.ErrorIs(t, err, context.Canceled)
}

// A tool that ignores the deadline but finishes successfully keeps its
// result: the work is already done.
func TestTimeoutToolKeepsALateSuccess(t *testing.T) {
	wrapped := wrapToolsWithTimeout([]fantasy.AgentTool{&funcTool{
		name: "grep",
		run: func(context.Context, fantasy.ToolCall) (fantasy.ToolResponse, error) {
			time.Sleep(20 * time.Millisecond)
			return fantasy.NewTextResponse("found it"), nil
		},
	}}, 5*time.Millisecond)

	resp, err := wrapped[0].Run(t.Context(), fantasy.ToolCall{Name: "grep"})
	require.NoError(t, err)
	require.Equal(t, "found it", resp.Content)
}

// An ordinary tool failure is not a timeout and must not be relabelled.
func TestTimeoutToolPassesThroughAnOrdinaryError(t *testing.T) {
	wantErr := errors.New("file not found")
	wrapped := wrapToolsWithTimeout([]fantasy.AgentTool{&funcTool{
		name: "view",
		run: func(context.Context, fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return fantasy.ToolResponse{}, wantErr
		},
	}}, time.Minute)

	_, err := wrapped[0].Run(t.Context(), fantasy.ToolCall{Name: "view"})
	require.ErrorIs(t, err, wantErr)
}
