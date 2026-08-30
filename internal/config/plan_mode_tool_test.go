package config

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

// The coder agent's tool list is filtered against AllowedTools in
// coordinator.buildTools, so a tool missing from allToolNames() is silently
// dropped before the model ever sees it. exit_plan_mode is the agent's only
// way out of plan mode, and dropping it strands the model: every write/edit
// call is auto-denied and it has no tool to ask the user for approval with.
func TestCoderAgentExposesExitPlanModeTool(t *testing.T) {
	t.Parallel()

	require.Contains(t, allToolNames(), "exit_plan_mode",
		"exit_plan_mode must be in allToolNames or buildTools filters it out")

	c := &Config{Options: &Options{}}
	c.SetupAgents()

	coder, ok := c.Agents[AgentCoder]
	require.True(t, ok, "coder agent must exist")
	require.True(t, slices.Contains(coder.AllowedTools, "exit_plan_mode"),
		"coder agent must be allowed to call exit_plan_mode")
}
