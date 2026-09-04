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

// toolSettingsWorkspace is a minimal workspace.Workspace stub: a fixed
// *config.Config for reads, and a recorded SetConfigField call for the
// write-side assertions.
type toolSettingsWorkspace struct {
	workspace.Workspace
	cfg      *config.Config
	setKey   string
	setValue any
	setErr   error
}

func (w *toolSettingsWorkspace) Config() *config.Config { return w.cfg }

func (w *toolSettingsWorkspace) SetConfigField(_ config.Scope, key string, value any) error {
	w.setKey, w.setValue = key, value
	return w.setErr
}

func (w *toolSettingsWorkspace) ListSubagents(context.Context) ([]subagents.Subagent, error) {
	return nil, nil
}

func newToolSettingsDialog(t *testing.T, ws *toolSettingsWorkspace) *ToolSettings {
	t.Helper()
	sty := styles.AtlasPantera()
	return NewToolSettings(&common.Common{Workspace: ws, Styles: &sty})
}

func testToolSettingsConfig() *config.Config {
	enabled := true
	return &config.Config{
		Tools: config.Tools{
			Browser: config.ToolBrowser{Enabled: &enabled},
		},
	}
}

func TestToolSettingsListsTheFullCatalog(t *testing.T) {
	d := newToolSettingsDialog(t, &toolSettingsWorkspace{cfg: testToolSettingsConfig()})
	require.Equal(t, ToolSettingsID, d.ID())
	require.Equal(t, len(toolSettingsCatalog), d.list.Len())
}

func TestToolSettingsReadsCurrentEnabledState(t *testing.T) {
	d := newToolSettingsDialog(t, &toolSettingsWorkspace{cfg: testToolSettingsConfig()})

	var browser *toolSettingEntry
	var git *toolSettingEntry
	for _, item := range d.list.FilteredItems() {
		e := item.(*toolSettingEntry)
		switch e.key {
		case "browser":
			browser = e
		case "git":
			git = e
		}
	}
	require.NotNil(t, browser)
	require.True(t, browser.enabled, "browser was enabled in the fixture config")
	require.NotNil(t, git)
	require.False(t, git.enabled, "git defaults to disabled")
}

func TestToolSettingsToggleWritesTheExpectedFieldAndFlips(t *testing.T) {
	ws := &toolSettingsWorkspace{cfg: testToolSettingsConfig()}
	d := newToolSettingsDialog(t, ws)
	d.list.SelectFirst() // "git", currently disabled

	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	cmdAction, ok := action.(ActionCmd)
	require.True(t, ok, "expected ActionCmd, got %T", action)

	msg := cmdAction.Cmd()
	require.Equal(t, "tools.git.enabled", ws.setKey)
	require.Equal(t, true, ws.setValue)

	result := d.HandleMsg(msg)
	require.Nil(t, result)

	entry, ok := d.list.FilteredItems()[0].(*toolSettingEntry)
	require.True(t, ok)
	require.True(t, entry.enabled, "the row must flip in place after a successful toggle")
}

func TestToolSettingsToggleErrorIsReported(t *testing.T) {
	ws := &toolSettingsWorkspace{cfg: testToolSettingsConfig(), setErr: context.DeadlineExceeded}
	d := newToolSettingsDialog(t, ws)
	d.list.SelectFirst()

	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	cmdAction := action.(ActionCmd)
	msg := cmdAction.Cmd()

	result := d.HandleMsg(msg)
	_, ok := result.(ActionCmd)
	require.True(t, ok, "an error must surface as an ActionCmd reporting it")
}

func TestToolSettingsCloseKeyClosesDialog(t *testing.T) {
	d := newToolSettingsDialog(t, &toolSettingsWorkspace{cfg: testToolSettingsConfig()})
	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEscape})
	_, ok := action.(ActionClose)
	require.True(t, ok, "expected ActionClose, got %T", action)
}
