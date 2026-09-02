package agent

import (
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/hooks"
	"github.com/stretchr/testify/require"
)

func newPostRunner(t *testing.T, cmd string) *hooks.Runner {
	t.Helper()
	cfg := &config.Config{
		Hooks: map[string][]config.HookConfig{
			hooks.EventPostToolUse: {{Command: cmd}},
		},
	}
	require.NoError(t, cfg.ValidateHooks())
	return hooks.NewRunner(cfg.Hooks[hooks.EventPostToolUse], t.TempDir(), t.TempDir())
}

func TestPostHookContextIsAppendedToTheResult(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "view", resp: fantasy.NewTextResponse("file contents")}
	tool := newHookedTool(inner, nil, newPostRunner(t, `echo '{"context":"reviewed by policy"}'`))

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "c1", Name: "view"})
	require.NoError(t, err)
	require.True(t, inner.called)
	require.Contains(t, resp.Content, "file contents")
	require.Contains(t, resp.Content, "reviewed by policy")
}

// A post hook cannot undo the call -- it already happened -- but it can end
// the turn.
func TestPostHookCanHaltTheTurn(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "bash", resp: fantasy.NewTextResponse("done")}
	tool := newHookedTool(inner, nil, newPostRunner(t, `echo '{"halt":true,"reason":"budget spent"}'`))

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "c1", Name: "bash"})
	require.NoError(t, err)
	require.True(t, resp.StopTurn)
	require.Contains(t, resp.Content, "done", "the tool's own result must survive")
	require.Contains(t, resp.Content, "budget spent")
}

// A hook that says nothing must leave the result exactly as the tool
// returned it.
func TestSilentPostHookLeavesTheResultAlone(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "view", resp: fantasy.NewTextResponse("untouched")}
	tool := newHookedTool(inner, nil, newPostRunner(t, `exit 0`))

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "c1", Name: "view"})
	require.NoError(t, err)
	require.Equal(t, "untouched", resp.Content)
	require.False(t, resp.StopTurn)
}

// A blocked call never ran, so there is nothing for a post hook to see.
func TestPostHookDoesNotRunWhenPreHookBlocks(t *testing.T) {
	t.Parallel()

	inner := &fakeTool{name: "bash", resp: fantasy.NewTextResponse("should not happen")}
	pre := newRunner(t, `echo '{"decision":"deny","reason":"nope"}'`)
	tool := newHookedTool(inner, pre, newPostRunner(t, `echo '{"context":"post ran"}'`))

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{ID: "c1", Name: "bash"})
	require.NoError(t, err)
	require.False(t, inner.called)
	require.NotContains(t, resp.Content, "post ran")
}

func TestToolsAreWrappedWhenOnlyPostHooksExist(t *testing.T) {
	t.Parallel()

	inputs := []fantasy.AgentTool{&fakeTool{name: "view"}}
	out := wrapToolsWithHooks(inputs, nil, newPostRunner(t, "exit 0"), false)
	require.IsType(t, &hookedTool{}, out[0])

	// Sub-agents still fire nothing.
	require.Equal(t, inputs, wrapToolsWithHooks(inputs, nil, newPostRunner(t, "exit 0"), true))
}
