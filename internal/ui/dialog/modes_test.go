package dialog

import (
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	tea "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-ui/v2"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/subagents"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/common"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/styles"
	"github.com/stretchr/testify/require"
)

func newModesDialog(t *testing.T, cfg *config.Config) *Modes {
	t.Helper()
	sty := styles.AtlasPantera()
	return NewModes(&common.Common{Workspace: &modelRolesWorkspace{cfg: cfg}, Styles: &sty})
}

func testModesConfig(active string) *config.Config {
	return &config.Config{
		Models: map[config.SelectedModelType]config.SelectedModel{
			config.SelectedModelTypeLarge: {Provider: "anthropic", Model: "claude-sonnet-5"},
		},
		Options: &config.Options{
			SessionMode: active,
			ModelRoles: map[string]config.SelectedModel{
				"review": {Provider: "openai", Model: "o3"},
			},
		},
	}
}

func selectModeByName(t *testing.T, d *Modes, name string) {
	t.Helper()
	for i, item := range d.list.FilteredItems() {
		if e, ok := item.(*modeEntry); ok && e.name == name {
			d.list.SetSelected(i)
			return
		}
	}
	t.Fatalf("no mode row named %q", name)
}

func TestModesListsEveryBuiltinPlusAnOffRow(t *testing.T) {
	d := newModesDialog(t, testModesConfig(""))

	require.Equal(t, ModesID, d.ID())
	require.Equal(t, len(subagents.Builtin())+1, d.list.Len(), "every shipped mode, plus the row that turns modes off")
}

func TestModesShowsTheAssignedModelForAMode(t *testing.T) {
	d := newModesDialog(t, testModesConfig(""))
	selectModeByName(t, d, "review")

	entry := d.list.SelectedItem().(*modeEntry)
	require.Equal(t, "openai/o3", entry.roleModel)
	require.Contains(t, entry.info(), "runs on openai/o3")
}

func TestModesSaysWhenAModeHasNoModelAssigned(t *testing.T) {
	d := newModesDialog(t, testModesConfig(""))
	selectModeByName(t, d, "security")

	entry := d.list.SelectedItem().(*modeEntry)
	require.Empty(t, entry.roleModel)
	require.Contains(t, entry.info(), "no model assigned")
}

func TestModesOpensOnTheActiveMode(t *testing.T) {
	d := newModesDialog(t, testModesConfig("security"))

	entry, ok := d.list.SelectedItem().(*modeEntry)
	require.True(t, ok)
	require.Equal(t, "security", entry.name, "the dialog must open on the mode already in effect")
	require.True(t, entry.active)
}

func TestModesSelectingAModeEmitsIt(t *testing.T) {
	d := newModesDialog(t, testModesConfig(""))
	selectModeByName(t, d, "planner")

	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	selected, ok := action.(ActionSelectSessionMode)
	require.True(t, ok, "expected ActionSelectSessionMode, got %T", action)
	require.Equal(t, "planner", selected.Mode)
}

func TestModesSelectingNoModeClearsIt(t *testing.T) {
	d := newModesDialog(t, testModesConfig("planner"))
	selectModeByName(t, d, noModeKey)

	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	selected, ok := action.(ActionSelectSessionMode)
	require.True(t, ok, "expected ActionSelectSessionMode, got %T", action)
	require.Empty(t, selected.Mode, "the off row must clear the mode, not set one")
}

func TestModesCloseKeyClosesDialog(t *testing.T) {
	d := newModesDialog(t, testModesConfig(""))
	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEscape})
	_, ok := action.(ActionClose)
	require.True(t, ok, "expected ActionClose, got %T", action)
}

func TestSessionModeCommandLabelNamesTheActiveMode(t *testing.T) {
	require.Equal(t, "Session Mode", sessionModeCommandLabel(testModesConfig("")))
	require.Equal(t, "Session Mode (Security)", sessionModeCommandLabel(testModesConfig("security")))
	require.Equal(t, "Session Mode", sessionModeCommandLabel(nil))
}

// Every shipped mode must have a matching role row, since assigning it a
// model is how a user gives the mode its own model. Deriving one from the
// other in presetModelRoles is what keeps this true; the test pins it.
func TestEveryBuiltinModeHasAPresetRole(t *testing.T) {
	roles := make(map[string]bool)
	for _, r := range presetModelRoles() {
		roles[r.name] = true
	}
	for _, mode := range subagents.Builtin() {
		require.True(t, roles[mode.Name], "mode %q has no model role to assign a model to", mode.Name)
	}
}
