package dialog

import (
	"context"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	tea "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-ui/v2"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/subagents"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/common"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/styles"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/workspace"
	"github.com/stretchr/testify/require"
)

// modelRolesWorkspace is a minimal workspace.Workspace stub: it serves
// a fixed *config.Config for reads and records SetConfigField/
// RemoveConfigField calls for the write-side assertions.
type modelRolesWorkspace struct {
	workspace.Workspace
	cfg *config.Config

	setKey    string
	setValue  any
	setErr    error
	removeKey string
	removeErr error
}

func (w *modelRolesWorkspace) Config() *config.Config { return w.cfg }

func (w *modelRolesWorkspace) SetConfigField(_ config.Scope, key string, value any) error {
	w.setKey, w.setValue = key, value
	return w.setErr
}

func (w *modelRolesWorkspace) RemoveConfigField(_ config.Scope, key string) error {
	w.removeKey = key
	return w.removeErr
}

func (w *modelRolesWorkspace) ListSubagents(context.Context) ([]subagents.Subagent, error) {
	return nil, nil
}

func newModelRolesDialog(t *testing.T, ws *modelRolesWorkspace) *ModelRoles {
	t.Helper()
	sty := styles.AtlasPantera()
	return NewModelRoles(&common.Common{Workspace: ws, Styles: &sty})
}

// selectRoleByName moves the dialog's selection to the row named name,
// failing the test if no such row exists. Presets always show up in a
// fixed order, so tests select by name rather than relying on position.
func selectRoleByName(t *testing.T, d *ModelRoles, name string) {
	t.Helper()
	for i, item := range d.list.FilteredItems() {
		if e, ok := item.(*modelRoleEntry); ok && e.name == name {
			d.list.SetSelected(i)
			return
		}
	}
	t.Fatalf("no role row named %q", name)
}

func testModelRolesConfig() *config.Config {
	return &config.Config{
		Models: map[config.SelectedModelType]config.SelectedModel{
			config.SelectedModelTypeLarge: {Provider: "anthropic", Model: "claude-sonnet-5"},
			config.SelectedModelTypeSmall: {Provider: "anthropic", Model: "claude-haiku-4-5"},
		},
		Options: &config.Options{
			ModelRoles: map[string]config.SelectedModel{
				"research": {Provider: "openai", Model: "o3", ReasoningEffort: "high"},
			},
		},
	}
}

func TestModelRolesListsBuiltinPresetAndCustomRoles(t *testing.T) {
	d := newModelRolesDialog(t, &modelRolesWorkspace{cfg: testModelRolesConfig()})

	require.Equal(t, ModelRolesID, d.ID())
	// large, small, the 10 presets (one of which -- "research" -- is
	// already assigned in testModelRolesConfig), no leftover custom rows.
	require.Equal(t, 2+len(presetModelRoles()), d.list.Len())
}

func TestModelRolesUnassignedPresetShowsNotSet(t *testing.T) {
	d := newModelRolesDialog(t, &modelRolesWorkspace{cfg: testModelRolesConfig()})
	selectRoleByName(t, d, "compact") // a preset with no model assigned

	entry, ok := d.list.SelectedItem().(*modelRoleEntry)
	require.True(t, ok)
	require.False(t, entry.assigned)
	require.Contains(t, entry.info(), "not set")
}

func TestModelRolesEditOpensFormForUnassignedPreset(t *testing.T) {
	d := newModelRolesDialog(t, &modelRolesWorkspace{cfg: testModelRolesConfig()})
	selectRoleByName(t, d, "compact")

	action := d.HandleMsg(keyMsg('e'))
	form, ok := action.(ActionOpenModelRoleForm)
	require.True(t, ok, "expected ActionOpenModelRoleForm, got %T", action)
	require.Equal(t, "compact", form.ExistingName, "preset name is fixed, not typed")
	require.Empty(t, form.ExistingProvider)
	require.Empty(t, form.ExistingModel)
}

func TestModelRolesDeleteIgnoresUnassignedPreset(t *testing.T) {
	d := newModelRolesDialog(t, &modelRolesWorkspace{cfg: testModelRolesConfig()})
	selectRoleByName(t, d, "compact")

	require.Nil(t, d.HandleMsg(keyMsg('x')), "deleting an unassigned preset must be a no-op")
}

func TestModelRolesAddOpensForm(t *testing.T) {
	d := newModelRolesDialog(t, &modelRolesWorkspace{cfg: testModelRolesConfig()})

	action := d.HandleMsg(keyMsg('a'))
	form, ok := action.(ActionOpenModelRoleForm)
	require.True(t, ok, "expected ActionOpenModelRoleForm, got %T", action)
	require.Empty(t, form.ExistingName)
}

func TestModelRolesEditIgnoresBuiltinRoles(t *testing.T) {
	d := newModelRolesDialog(t, &modelRolesWorkspace{cfg: testModelRolesConfig()})
	d.list.SelectFirst() // "large", a builtin row

	require.Nil(t, d.HandleMsg(keyMsg('e')), "editing a builtin role must be a no-op")
}

func TestModelRolesEditOpensFormForCustomRole(t *testing.T) {
	d := newModelRolesDialog(t, &modelRolesWorkspace{cfg: testModelRolesConfig()})
	selectRoleByName(t, d, "research") // the one assigned preset in this config

	action := d.HandleMsg(keyMsg('e'))
	form, ok := action.(ActionOpenModelRoleForm)
	require.True(t, ok, "expected ActionOpenModelRoleForm, got %T", action)
	require.Equal(t, "research", form.ExistingName)
	require.Equal(t, "openai", form.ExistingProvider)
	require.Equal(t, "o3", form.ExistingModel)
	require.Equal(t, "high", form.ExistingReasoningEffort)
}

func TestModelRolesDeleteIgnoresBuiltinRoles(t *testing.T) {
	d := newModelRolesDialog(t, &modelRolesWorkspace{cfg: testModelRolesConfig()})
	d.list.SelectFirst()

	require.Nil(t, d.HandleMsg(keyMsg('x')), "deleting a builtin role must be a no-op")
}

func TestModelRolesDeleteCustomRoleRemovesConfigFieldAndRefreshes(t *testing.T) {
	ws := &modelRolesWorkspace{cfg: testModelRolesConfig()}
	d := newModelRolesDialog(t, ws)
	selectRoleByName(t, d, "research")

	action := d.HandleMsg(keyMsg('x'))
	cmdAction, ok := action.(ActionCmd)
	require.True(t, ok, "expected ActionCmd, got %T", action)

	msg := cmdAction.Cmd()
	require.Equal(t, "options.model_roles.research", ws.removeKey)

	// Simulate the config actually losing the role, the way a real
	// RemoveConfigField call would, then let the dialog react. The row
	// itself stays (it's a preset) but reverts to unassigned.
	delete(ws.cfg.Options.ModelRoles, "research")
	result := d.HandleMsg(msg)
	require.Nil(t, result)
	require.Equal(t, 2+len(presetModelRoles()), d.list.Len())
	selectRoleByName(t, d, "research")
	entry, ok := d.list.SelectedItem().(*modelRoleEntry)
	require.True(t, ok)
	require.False(t, entry.assigned, "research must be unassigned after delete")
}

func TestModelRolesDeleteErrorIsReported(t *testing.T) {
	ws := &modelRolesWorkspace{cfg: testModelRolesConfig(), removeErr: context.DeadlineExceeded}
	d := newModelRolesDialog(t, ws)
	selectRoleByName(t, d, "research")

	action := d.HandleMsg(keyMsg('x'))
	cmdAction := action.(ActionCmd)
	msg := cmdAction.Cmd()

	result := d.HandleMsg(msg)
	_, ok := result.(ActionCmd)
	require.True(t, ok, "an error must surface as an ActionCmd reporting it")
}

func TestModelRolesCloseKeyClosesDialog(t *testing.T) {
	d := newModelRolesDialog(t, &modelRolesWorkspace{cfg: testModelRolesConfig()})
	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEscape})
	_, ok := action.(ActionClose)
	require.True(t, ok, "expected ActionClose, got %T", action)
}
