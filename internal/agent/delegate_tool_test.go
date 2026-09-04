package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/agent/tools"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/credentials"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func TestValidDelegateTasksDropsMalformedEntries(t *testing.T) {
	got := validDelegateTasks([]DelegateTask{
		{AgentName: "frontend", Prompt: "do the ui"},
		{AgentName: "", Prompt: "no agent name"},
		{AgentName: "backend", Prompt: "  "},
		{AgentName: "  research  ", Prompt: "  dig in  "},
	})
	require.Equal(t, []DelegateTask{
		{AgentName: "frontend", Prompt: "do the ui"},
		{AgentName: "research", Prompt: "dig in"},
	}, got)
}

func TestFormatDelegateResults(t *testing.T) {
	out := formatDelegateResults([]delegateResult{
		{agentName: "frontend", prompt: "build the form", content: "form built"},
		{agentName: "backend", prompt: "validate input", err: context.DeadlineExceeded},
	})
	require.Contains(t, out, "2 subtask(s) ran in parallel")
	require.Contains(t, out, "=== Task 1: frontend ===")
	require.Contains(t, out, "build the form")
	require.Contains(t, out, "form built")
	require.Contains(t, out, "=== Task 2: backend ===")
	require.Contains(t, out, "validate input")
	require.Contains(t, out, "failed: context deadline exceeded")
}

func runDelegateTool(t *testing.T, coord *coordinator, ctx context.Context, params DelegateParams) string {
	t.Helper()
	tool, err := coord.delegateTool(ctx)
	require.NoError(t, err)

	input, err := json.Marshal(params)
	require.NoError(t, err)

	resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: DelegateToolName, Input: string(input)})
	require.NoError(t, err)
	return resp.Content
}

func TestDelegateToolRequiresTwoValidTasks(t *testing.T) {
	coord := hermeticSubagentCoordinator(t)
	coord.credentials = credentials.New()

	cases := [][]DelegateTask{
		nil,
		{{AgentName: "frontend", Prompt: "do it"}},
		{{AgentName: "frontend", Prompt: "do it"}, {AgentName: "", Prompt: ""}},
	}
	for _, tasks := range cases {
		out := runDelegateTool(t, coord, t.Context(), DelegateParams{Tasks: tasks})
		require.Contains(t, out, "delegate needs at least two tasks")
	}
}

func TestDelegateToolNotConfiguredWithoutTaskAgent(t *testing.T) {
	coord := hermeticSubagentCoordinator(t)
	delete(coord.cfg.Config().Agents, config.AgentTask)

	_, err := coord.delegateTool(t.Context())
	require.Error(t, err)
	require.Contains(t, err.Error(), "task agent not configured")
}

func TestDelegateToolRunsEachTaskWithItsOwnPrompt(t *testing.T) {
	coord := hermeticSubagentCoordinator(t)
	coord.credentials = credentials.New()

	ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "session-1")
	ctx = context.WithValue(ctx, tools.MessageIDContextKey, "message-1")
	parentSession, err := coord.sessions.Create(ctx, "Parent")
	require.NoError(t, err)
	ctx = context.WithValue(ctx, tools.SessionIDContextKey, parentSession.ID)

	// Neither "frontend" nor "backend" resolves to a real, reachable
	// provider, so each run fails fast rather than blocking on the
	// network -- this still exercises the full fan-out/aggregation path
	// for distinct per-task prompts, same as
	// TestOrchestrateToolRunsNamedAgentsInParallel does for a shared one.
	out := runDelegateTool(t, coord, ctx, DelegateParams{
		Tasks: []DelegateTask{
			{AgentName: "frontend", Prompt: "build the form"},
			{AgentName: "backend", Prompt: "validate the input"},
		},
	})

	require.Contains(t, out, "2 subtask(s) ran in parallel")
	require.Contains(t, out, "=== Task 1: frontend ===")
	require.Contains(t, out, "build the form")
	require.Contains(t, out, "=== Task 2: backend ===")
	require.Contains(t, out, "validate the input")
}

func TestDelegateToolAllowsRepeatingAgentNames(t *testing.T) {
	coord := hermeticSubagentCoordinator(t)
	coord.credentials = credentials.New()

	ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "session-1")
	ctx = context.WithValue(ctx, tools.MessageIDContextKey, "message-1")
	parentSession, err := coord.sessions.Create(ctx, "Parent")
	require.NoError(t, err)
	ctx = context.WithValue(ctx, tools.SessionIDContextKey, parentSession.ID)

	out := runDelegateTool(t, coord, ctx, DelegateParams{
		Tasks: []DelegateTask{
			{AgentName: "frontend", Prompt: "build page A"},
			{AgentName: "frontend", Prompt: "build page B"},
		},
	})

	require.Contains(t, out, "build page A")
	require.Contains(t, out, "build page B")
}
