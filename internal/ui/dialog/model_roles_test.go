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

func testModelRolesConfig() *config.Config {
	return &config.Config{
		Models: map[config.SelectedModelType]config.SelectedModel{
			config.SelectedModelTypeLarge: {Provider: "anthropic", Model: "claude-sonnet-5"},
			config.SelectedModelTypeSmall: {Provider: "anthropic", Model: "claude-haiku-4-5"},
		},
		Options: &config.Options{
			ModelRoles: map[string]config.SelectedModel{
				"research": {Provider: "openai", Model: "o3"},
			},
		},
	}
}

func TestModelRolesListsBuiltinAndCustomRoles(t *testing.T) {
	d := newModelRolesDialog(t, &modelRolesWorkspace{cfg: testModelRolesConfig()})

	require.Equal(t, ModelRolesID, d.ID())
	require.Equal(t, 3, d.list.Len(), "large, small, and one custom role")
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
	d.list.SelectLast() // "research", the only custom role

	action := d.HandleMsg(keyMsg('e'))
	form, ok := action.(ActionOpenModelRoleForm)
	require.True(t, ok, "expected ActionOpenModelRoleForm, got %T", action)
	require.Equal(t, "research", form.ExistingName)
	require.Equal(t, "openai", form.ExistingProvider)
	require.Equal(t, "o3", form.ExistingModel)
}

func TestModelRolesDeleteIgnoresBuiltinRoles(t *testing.T) {
	d := newModelRolesDialog(t, &modelRolesWorkspace{cfg: testModelRolesConfig()})
	d.list.SelectFirst()

	require.Nil(t, d.HandleMsg(keyMsg('x')), "deleting a builtin role must be a no-op")
}

func TestModelRolesDeleteCustomRoleRemovesConfigFieldAndRefreshes(t *testing.T) {
	ws := &modelRolesWorkspace{cfg: testModelRolesConfig()}
	d := newModelRolesDialog(t, ws)
	d.list.SelectLast() // "research"

	action := d.HandleMsg(keyMsg('x'))
	cmdAction, ok := action.(ActionCmd)
	require.True(t, ok, "expected ActionCmd, got %T", action)

	msg := cmdAction.Cmd()
	require.Equal(t, "options.model_roles.research", ws.removeKey)

	// Simulate the config actually losing the role, the way a real
	// RemoveConfigField call would, then let the dialog react.
	delete(ws.cfg.Options.ModelRoles, "research")
	result := d.HandleMsg(msg)
	require.Nil(t, result)
	require.Equal(t, 2, d.list.Len(), "the deleted role must be gone after refresh")
}

func TestModelRolesDeleteErrorIsReported(t *testing.T) {
	ws := &modelRolesWorkspace{cfg: testModelRolesConfig(), removeErr: context.DeadlineExceeded}
	d := newModelRolesDialog(t, ws)
	d.list.SelectLast()

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
