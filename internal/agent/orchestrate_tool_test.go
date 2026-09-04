package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/agent/tools"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/credentials"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/csync"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func TestDedupeAgentNames(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"empty", nil, []string{}},
		{"trims and drops blanks", []string{" a ", "", "b"}, []string{"a", "b"}},
		{"dedupes preserving first-seen order", []string{"b", "a", "b", "a"}, []string{"b", "a"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := dedupeAgentNames(c.in)
			require.Equal(t, c.want, got)
		})
	}
}

func TestFormatOrchestrateResults(t *testing.T) {
	out := formatOrchestrateResults([]orchestrateResult{
		{name: "alpha", content: "alpha's answer"},
		{name: "beta", err: context.DeadlineExceeded},
	})
	require.Contains(t, out, "2 agent(s) ran independently")
	require.Contains(t, out, "=== alpha ===")
	require.Contains(t, out, "alpha's answer")
	require.Contains(t, out, "=== beta ===")
	require.Contains(t, out, "failed: context deadline exceeded")
}

func TestRunOrchestratedAgentHappyPath(t *testing.T) {
	const providerID = "test-provider"
	env := testEnv(t)
	coord := newTestCoordinator(t, env, providerID, config.ProviderConfig{ID: providerID})

	parentSession, err := env.sessions.Create(t.Context(), "Parent")
	require.NoError(t, err)

	mock := newMockAgent(providerID, 4096, func(_ context.Context, call SessionAgentCall) (*fantasy.AgentResult, error) {
		require.Equal(t, "do something", call.Prompt)
		return agentResultWithText("alpha did it"), nil
	})
	cache := csync.NewMap[string, SessionAgent]()
	cache.Set("alpha", mock)

	result := coord.runOrchestratedAgent(t.Context(), orchestratedAgentParams{
		agentCfg:          coord.cfg.Config().Agents[config.AgentTask],
		subagentInstances: cache,
		limiter:           newConcurrencyLimiter(0),
		name:              "alpha",
		sessionID:         parentSession.ID,
		agentMessageID:    "msg-1",
		toolCallID:        "call-1-alpha",
		prompt:            "do something",
	})

	require.NoError(t, result.err)
	require.Equal(t, "alpha", result.name)
	require.Equal(t, "alpha did it", result.content)
}

func TestRunOrchestratedAgentUnknownNameFails(t *testing.T) {
	const providerID = "test-provider"
	env := testEnv(t)
	coord := newTestCoordinator(t, env, providerID, config.ProviderConfig{ID: providerID})

	parentSession, err := env.sessions.Create(t.Context(), "Parent")
	require.NoError(t, err)

	result := coord.runOrchestratedAgent(t.Context(), orchestratedAgentParams{
		agentCfg:          coord.cfg.Config().Agents[config.AgentTask],
		subagentInstances: csync.NewMap[string, SessionAgent](),
		limiter:           newConcurrencyLimiter(0),
		name:              "nope",
		sessionID:         parentSession.ID,
		agentMessageID:    "msg-1",
		toolCallID:        "call-1-nope",
		prompt:            "do something",
	})

	require.Error(t, result.err)
	require.Contains(t, result.err.Error(), `no subagent named "nope"`)
}

func runOrchestrateTool(t *testing.T, coord *coordinator, ctx context.Context, params OrchestrateParams) string {
	t.Helper()
	tool, err := coord.orchestrateTool(ctx)
	require.NoError(t, err)

	input, err := json.Marshal(params)
	require.NoError(t, err)

	resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: OrchestrateToolName, Input: string(input)})
	require.NoError(t, err)
	return resp.Content
}

func TestOrchestrateToolRequiresPrompt(t *testing.T) {
	coord := hermeticSubagentCoordinator(t)
	coord.credentials = credentials.New()

	out := runOrchestrateTool(t, coord, t.Context(), OrchestrateParams{AgentNames: []string{"a", "b"}})
	require.Contains(t, out, "prompt is required")
}

func TestOrchestrateToolRequiresTwoDistinctAgents(t *testing.T) {
	coord := hermeticSubagentCoordinator(t)
	coord.credentials = credentials.New()

	cases := [][]string{nil, {"solo"}, {"same", "same"}}
	for _, names := range cases {
		out := runOrchestrateTool(t, coord, t.Context(), OrchestrateParams{Prompt: "do something", AgentNames: names})
		require.Contains(t, out, "at least two distinct agent_names")
	}
}

func TestOrchestrateToolNotConfiguredWithoutTaskAgent(t *testing.T) {
	coord := hermeticSubagentCoordinator(t)
	delete(coord.cfg.Config().Agents, config.AgentTask)

	_, err := coord.orchestrateTool(t.Context())
	require.Error(t, err)
	require.Contains(t, err.Error(), "task agent not configured")
}

func TestOrchestrateToolRunsNamedAgentsInParallel(t *testing.T) {
	coord := hermeticSubagentCoordinator(t)
	coord.credentials = credentials.New()

	ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, "session-1")
	ctx = context.WithValue(ctx, tools.MessageIDContextKey, "message-1")

	parentSession, err := coord.sessions.Create(ctx, "Parent")
	require.NoError(t, err)
	ctx = context.WithValue(ctx, tools.SessionIDContextKey, parentSession.ID)

	// Neither "alpha" nor "beta" resolves to a real, reachable provider,
	// so each run fails fast rather than blocking on the network -- this
	// still exercises the full fan-out/aggregation path: dedupe, the
	// parallel wait group, the per-agent limiter, and formatting every
	// result (success or failure) side by side.
	out := runOrchestrateTool(t, coord, ctx, OrchestrateParams{
		Prompt:     "do something",
		AgentNames: []string{"alpha", "beta"},
	})

	require.Contains(t, out, "2 agent(s) ran independently")
	require.Contains(t, out, "=== alpha ===")
	require.Contains(t, out, "=== beta ===")
}
