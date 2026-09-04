package dialog

import (
	"context"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/common"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/styles"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/workspace"
	"github.com/stretchr/testify/require"
)

// fallbacksWorkspace is a minimal workspace.Workspace stub for the
// Fallbacks dialog: a fixed *config.Config for reads, and a recorded
// SetConfigField call for the write-side assertions.
type fallbacksWorkspace struct {
	workspace.Workspace
	cfg      *config.Config
	setKey   string
	setValue any
	setErr   error
}

func (w *fallbacksWorkspace) Config() *config.Config { return w.cfg }

func (w *fallbacksWorkspace) SetConfigField(_ config.Scope, key string, value any) error {
	w.setKey, w.setValue = key, value
	return w.setErr
}

func newFallbacksDialog(t *testing.T, ws *fallbacksWorkspace) *Fallbacks {
	t.Helper()
	sty := styles.AtlasPantera()
	return NewFallbacks(&common.Common{Workspace: ws, Styles: &sty})
}

func testFallbacksConfig() *config.Config {
	return &config.Config{
		Options: &config.Options{
			FallbackCooldown: 300,
			ModelFallbacks: map[config.SelectedModelType][]config.SelectedModel{
				config.SelectedModelTypeLarge: {
					{Provider: "openai", Model: "gpt-4o-mini"},
					{Provider: "anthropic", Model: "claude-haiku-4-5"},
				},
			},
		},
	}
}

func TestFallbacksListsCooldownHeadersAndEntries(t *testing.T) {
	d := newFallbacksDialog(t, &fallbacksWorkspace{cfg: testFallbacksConfig()})

	require.Equal(t, FallbacksID, d.ID())
	// large-cooldown, large#1, large#2, small-cooldown (small has no entries)
	require.Equal(t, 4, d.list.Len())
}

func TestFallbacksAddUsesSelectedRowsModelType(t *testing.T) {
	d := newFallbacksDialog(t, &fallbacksWorkspace{cfg: testFallbacksConfig()})
	d.list.SelectFirst() // large's cooldown header

	action := d.HandleMsg(keyMsg('a'))
	form, ok := action.(ActionOpenFallbackEntryForm)
	require.True(t, ok, "expected ActionOpenFallbackEntryForm, got %T", action)
	require.Equal(t, config.SelectedModelTypeLarge, form.ModelType)
}

func TestFallbacksCooldownKeyIgnoresEntryRows(t *testing.T) {
	d := newFallbacksDialog(t, &fallbacksWorkspace{cfg: testFallbacksConfig()})
	d.list.SelectNext() // large's cooldown header -> large#1 (an entry row)

	require.Nil(t, d.HandleMsg(keyMsg('c')), "editing cooldown on an entry row must be a no-op")
}

func TestFallbacksCooldownKeyOpensFormOnCooldownRow(t *testing.T) {
	d := newFallbacksDialog(t, &fallbacksWorkspace{cfg: testFallbacksConfig()})
	d.list.SelectFirst()

	action := d.HandleMsg(keyMsg('c'))
	form, ok := action.(ActionOpenFallbackCooldownForm)
	require.True(t, ok, "expected ActionOpenFallbackCooldownForm, got %T", action)
	require.Equal(t, 300, form.Current)
}

func TestFallbacksDeleteIgnoresCooldownRows(t *testing.T) {
	d := newFallbacksDialog(t, &fallbacksWorkspace{cfg: testFallbacksConfig()})
	d.list.SelectFirst()

	require.Nil(t, d.HandleMsg(keyMsg('x')), "deleting a cooldown header row must be a no-op")
}

func TestFallbacksDeleteEntryPersistsTheRemainingChain(t *testing.T) {
	ws := &fallbacksWorkspace{cfg: testFallbacksConfig()}
	d := newFallbacksDialog(t, ws)
	d.list.SelectNext() // large#1 (gpt-4o-mini)

	action := d.HandleMsg(keyMsg('x'))
	cmdAction, ok := action.(ActionCmd)
	require.True(t, ok, "expected ActionCmd, got %T", action)

	msg := cmdAction.Cmd()
	require.Equal(t, "options.model_fallbacks.large", ws.setKey)
	require.Equal(t, []config.SelectedModel{{Provider: "anthropic", Model: "claude-haiku-4-5"}}, ws.setValue)

	// Simulate the config reflecting the write, then let the dialog
	// react and refresh.
	ws.cfg.Options.ModelFallbacks[config.SelectedModelTypeLarge] = ws.setValue.([]config.SelectedModel)
	result := d.HandleMsg(msg)
	require.Nil(t, result)
	require.Equal(t, 3, d.list.Len(), "large-cooldown, large#1 (the survivor), small-cooldown")
}

func TestFallbacksDeleteErrorIsReported(t *testing.T) {
	ws := &fallbacksWorkspace{cfg: testFallbacksConfig(), setErr: context.DeadlineExceeded}
	d := newFallbacksDialog(t, ws)
	d.list.SelectNext()

	action := d.HandleMsg(keyMsg('x'))
	cmdAction := action.(ActionCmd)
	msg := cmdAction.Cmd()

	result := d.HandleMsg(msg)
	_, ok := result.(ActionCmd)
	require.True(t, ok, "an error must surface as an ActionCmd reporting it")
}
