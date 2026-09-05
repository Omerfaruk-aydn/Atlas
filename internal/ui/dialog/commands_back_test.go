package dialog

import (
	"testing"

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
