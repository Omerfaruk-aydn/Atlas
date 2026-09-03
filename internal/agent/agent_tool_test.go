package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/credentials"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/csync"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/subagents"
	"github.com/stretchr/testify/require"
)

// hermeticSubagentCoordinator builds a coordinator against two fully local,
// never-dialed openai-typed providers ("mock" -- the session's primary --
// and "role-provider" -- what a subagent's model role points at), mirroring
// the hermetic setup in coordinator_readiness_test.go. No network call is
// ever made: buildProvider only constructs a client, and the tests below
// never call Run.
func hermeticSubagentCoordinator(t *testing.T) *coordinator {
	t.Helper()
	env := testEnv(t)

	atlasJSON := `{
  "options": {"disable_default_providers": true, "disable_provider_auto_update": true},
  "providers": {
    "mock": {"id": "mock", "name": "Mock", "type": "openai",
      "base_url": "http://127.0.0.1:9/v1", "api_key": "test-key",
      "models": [{"id": "mock-model", "name": "Mock", "context_window": 8192, "default_max_tokens": 128}]},
    "role-provider": {"id": "role-provider", "name": "Role", "type": "openai",
      "base_url": "http://127.0.0.1:9/v1", "api_key": "test-key",
      "models": [{"id": "role-model", "name": "Role", "context_window": 8192, "default_max_tokens": 128}]}
  },
  "models": {"large": {"provider": "mock", "model": "mock-model"},
             "small": {"provider": "mock", "model": "mock-model"}}
}`
	require.NoError(t, os.WriteFile(filepath.Join(env.workingDir, "atlas.json"), []byte(atlasJSON), 0o644))

	cfg, err := config.Init(env.workingDir, "", false)
	require.NoError(t, err)
	cfg.SetupAgents()

	return &coordinator{
		cfg:         cfg,
		sessions:    env.sessions,
		messages:    env.messages,
		permissions: env.permissions,
		history:     env.history,
		filetracker: *env.filetracker,
		credentials: credentials.New(),
	}
}

func TestBuildAdvisorDisabledByDefault(t *testing.T) {
	coord := hermeticSubagentCoordinator(t)
	model, tools := coord.buildAdvisor(t.Context())
	require.Nil(t, model)
	require.Nil(t, tools)
}

func TestBuildAdvisorWithNoModelRoleConfigured(t *testing.T) {
	coord := hermeticSubagentCoordinator(t)
	coord.cfg.Config().Options.Advisor = &config.Advisor{Enabled: true}

	model, tools := coord.buildAdvisor(t.Context())
	require.Nil(t, model)
	require.Nil(t, tools)
}

func TestBuildAdvisorResolvesItsModelRole(t *testing.T) {
	coord := hermeticSubagentCoordinator(t)
	coord.cfg.Config().Options.Advisor = &config.Advisor{Enabled: true}
	coord.cfg.Config().Options.ModelRoles = map[string]config.SelectedModel{
		"advisor": {Provider: "role-provider", Model: "role-model"},
	}

	model, tools := coord.buildAdvisor(t.Context())
	require.NotNil(t, model)
	require.Equal(t, "role-provider", model.ModelCfg.Provider)
	require.Equal(t, "role-model", model.ModelCfg.Model)
	require.NotEmpty(t, tools)

	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Info().Name
	}
	for _, want := range []string{"glob", "grep", "ls", "view"} {
		require.Contains(t, names, want)
	}
	require.NotContains(t, names, "bash", "the advisor must not get write/execute tools")
	require.NotContains(t, names, "edit", "the advisor must not get write/execute tools")
}

func TestResolveModelBuildsAReadyModel(t *testing.T) {
	coord := hermeticSubagentCoordinator(t)

	model, err := coord.resolveModel(t.Context(), config.SelectedModel{Provider: "role-provider", Model: "role-model"}, true)
	require.NoError(t, err)
	require.Equal(t, "role-provider", model.ModelCfg.Provider)
	require.Equal(t, "role-model", model.ModelCfg.Model)
}

func TestResolveModelUnknownProvider(t *testing.T) {
	coord := hermeticSubagentCoordinator(t)

	_, err := coord.resolveModel(t.Context(), config.SelectedModel{Provider: "no-such-provider", Model: "x"}, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not configured")
}

func TestResolveModelUnknownModelInCatalog(t *testing.T) {
	coord := hermeticSubagentCoordinator(t)

	_, err := coord.resolveModel(t.Context(), config.SelectedModel{Provider: "role-provider", Model: "no-such-model"}, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "catalog")
}

func TestBuildSubagentSessionAgentUsesDefaultModelWhenNoRoleSet(t *testing.T) {
	coord := hermeticSubagentCoordinator(t)
	taskCfg := coord.cfg.Config().Agents[config.AgentTask]

	agent, err := coord.buildSubagentSessionAgent(t.Context(), taskCfg, &subagents.Subagent{Name: "generic", Description: "d"})
	require.NoError(t, err)
	require.Equal(t, "mock-model", agent.Model().ModelCfg.Model)
}

func TestBuildSubagentSessionAgentUsesRoleOverride(t *testing.T) {
	coord := hermeticSubagentCoordinator(t)
	coord.cfg.Config().Options.ModelRoles = map[string]config.SelectedModel{
		"research": {Provider: "role-provider", Model: "role-model"},
	}
	taskCfg := coord.cfg.Config().Agents[config.AgentTask]

	agent, err := coord.buildSubagentSessionAgent(t.Context(), taskCfg,
		&subagents.Subagent{Name: "research", Description: "d", Model: "@research", Instructions: "Dig deep."})
	require.NoError(t, err)
	require.Equal(t, "role-model", agent.Model().ModelCfg.Model)
	require.Equal(t, "role-provider", agent.Model().ModelCfg.Provider)

	// Built synchronously (unlike the generic task agent's readyWg-based
	// build), so the system prompt is set immediately -- no need to wait
	// for a background goroutine to observe the instructions appended.
	sa, ok := agent.(*sessionAgent)
	require.True(t, ok)
	require.Contains(t, sa.systemPrompt.Get(), "Dig deep.")
	require.Contains(t, sa.systemPrompt.Get(), `name="research"`)
}

func TestBuildSubagentSessionAgentUnknownRoleErrors(t *testing.T) {
	coord := hermeticSubagentCoordinator(t)
	taskCfg := coord.cfg.Config().Agents[config.AgentTask]

	_, err := coord.buildSubagentSessionAgent(t.Context(), taskCfg,
		&subagents.Subagent{Name: "frontend", Description: "d", Model: "@frontend"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown model role")
}

func TestResolveSubagentCachesTheBuiltAgent(t *testing.T) {
	coord := hermeticSubagentCoordinator(t)
	taskCfg := coord.cfg.Config().Agents[config.AgentTask]
	discovered := []*subagents.Subagent{{Name: "generic", Description: "d"}}
	cache := csync.NewMap[string, SessionAgent]()

	first, err := coord.resolveSubagent(t.Context(), taskCfg, discovered, cache, "generic")
	require.NoError(t, err)
	second, err := coord.resolveSubagent(t.Context(), taskCfg, discovered, cache, "generic")
	require.NoError(t, err)

	require.Same(t, first, second, "the second call must reuse the cached instance, not rebuild")
}

func TestResolveSubagentUnknownNameErrors(t *testing.T) {
	coord := hermeticSubagentCoordinator(t)
	taskCfg := coord.cfg.Config().Agents[config.AgentTask]
	cache := csync.NewMap[string, SessionAgent]()

	_, err := coord.resolveSubagent(t.Context(), taskCfg, nil, cache, "nope")
	require.Error(t, err)
	require.Contains(t, err.Error(), `no subagent named "nope"`)
}
