package dialog

import (
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/commands"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/common"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/styles"
	"github.com/stretchr/testify/require"
)

func newCommandsDialog(t *testing.T, hasPreviousSession bool) *Commands {
	t.Helper()
	sty := styles.AtlasPantera()
	com := &common.Common{Workspace: &modelRolesWorkspace{cfg: &config.Config{Options: &config.Options{}}}, Styles: &sty}
	c, err := NewCommands(com, "s1", true, hasPreviousSession, false, false, nil, nil)
	require.NoError(t, err)
	return c
}

func commandIDs(c *Commands) map[string]bool {
	ids := make(map[string]bool)
	for _, item := range c.defaultCommands() {
		ids[item.ID()] = true
	}
	return ids
}

// Stepping into a sub-agent's session leaves f3 as the only way back,
// and f3 only shows in the expanded help -- so the palette must offer it
// too, but only when there is actually somewhere to go back to.
func TestCommandsOffersBackToSessionOnlyAfterSteppingIntoOne(t *testing.T) {
	require.True(t, commandIDs(newCommandsDialog(t, true))["back_to_session"])
	require.False(t, commandIDs(newCommandsDialog(t, false))["back_to_session"])
}

func TestCommandsAlwaysOffersSessionMode(t *testing.T) {
	require.True(t, commandIDs(newCommandsDialog(t, false))["session-mode"])
}

// AllItems feeds the "/" inline completion popup: it must offer every
// command the full palette does, across all three of its tabs at once,
// not just the system tab someone would land on by default.
func TestAllItemsCombinesSystemCustomAndMCPCommands(t *testing.T) {
	sty := styles.AtlasPantera()
	com := &common.Common{Workspace: &modelRolesWorkspace{cfg: &config.Config{Options: &config.Options{}}}, Styles: &sty}
	custom := []commands.CustomCommand{{ID: "c1", Name: "My Custom Command"}}
	mcp := []commands.MCPPrompt{{ID: "m1", PromptID: "my_mcp_prompt"}}

	c, err := NewCommands(com, "s1", true, false, false, false, custom, mcp)
	require.NoError(t, err)

	var titles []string
	for _, item := range c.AllItems() {
		titles = append(titles, item.Title())
	}
	require.Contains(t, titles, "New Session", "a system command must be included")
	require.Contains(t, titles, "My Custom Command")
	require.Contains(t, titles, "my_mcp_prompt")
}
