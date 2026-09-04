package dialog

import (
	"context"
	"image"
	"testing"
	"time"

	tea "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-ui/v2"
	uv "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-ultraviolet"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/common"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/styles"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/workspace"
	"github.com/stretchr/testify/require"
)

type agentHubWorkspace struct {
	workspace.Workspace
	entries    []workspace.AgentHubEntry
	cancelled  []string
	cancelledN int
}

func (w *agentHubWorkspace) AgentHubEntries(context.Context, string) []workspace.AgentHubEntry {
	return w.entries
}

func (w *agentHubWorkspace) AgentCancel(sessionID string) {
	w.cancelled = append(w.cancelled, sessionID)
	w.cancelledN++
}

func newAgentHubDialog(t *testing.T, ws *agentHubWorkspace) *AgentHub {
	t.Helper()
	sty := styles.AtlasPantera()
	return NewAgentHub(&common.Common{Workspace: ws, Styles: &sty}, "parent-1")
}

func TestAgentHubListsEveryEntry(t *testing.T) {
	t.Parallel()

	started := time.Now().Add(-5 * time.Minute)
	ws := &agentHubWorkspace{entries: []workspace.AgentHubEntry{
		{SessionID: "s1", Title: "Research", StartedAt: started, Cost: 0.01, Busy: true},
		{SessionID: "s2", Title: "Refactor", StartedAt: started, Cost: 0.02, Busy: false},
	}}
	d := newAgentHubDialog(t, ws)

	require.Equal(t, AgentHubID, d.ID())
	require.Len(t, d.list.FilteredItems(), 2)
}

func TestAgentHubEmptyShowsPlaceholder(t *testing.T) {
	t.Parallel()

	d := newAgentHubDialog(t, &agentHubWorkspace{})
	scr := uv.NewScreenBuffer(80, 24)
	d.Draw(scr, image.Rect(0, 0, 80, 24))
	require.Empty(t, d.list.FilteredItems())
}

func TestAgentHubViewSelectsSession(t *testing.T) {
	t.Parallel()

	ws := &agentHubWorkspace{entries: []workspace.AgentHubEntry{
		{SessionID: "s1", Title: "Research", Busy: true},
	}}
	d := newAgentHubDialog(t, ws)

	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	view, ok := action.(ActionViewSession)
	require.True(t, ok, "expected ActionViewSession, got %T", action)
	require.Equal(t, "s1", view.SessionID)
}

func TestAgentHubKillCancelsBusyEntry(t *testing.T) {
	t.Parallel()

	ws := &agentHubWorkspace{entries: []workspace.AgentHubEntry{
		{SessionID: "s1", Title: "Research", Busy: true},
	}}
	d := newAgentHubDialog(t, ws)

	action := d.HandleMsg(keyMsg('x'))
	cmdAction, ok := action.(ActionCmd)
	require.True(t, ok, "expected ActionCmd, got %T", action)

	msg := cmdAction.Cmd()
	require.Equal(t, []string{"s1"}, ws.cancelled)

	// The dialog reflects the cancellation in place, without re-fetching.
	result := d.HandleMsg(msg)
	require.Nil(t, result)
	entry, ok := d.list.FilteredItems()[0].(*agentHubEntry)
	require.True(t, ok)
	require.False(t, entry.entry.Busy, "cancelled entry must be marked idle")
}

func TestAgentHubKillIgnoresFinishedEntry(t *testing.T) {
	t.Parallel()

	ws := &agentHubWorkspace{entries: []workspace.AgentHubEntry{
		{SessionID: "s1", Title: "Research", Busy: false},
	}}
	d := newAgentHubDialog(t, ws)

	action := d.HandleMsg(keyMsg('x'))
	require.Nil(t, action, "cancelling an already-finished sub-agent must be a no-op")
	require.Empty(t, ws.cancelled)
}

func TestAgentHubCloseKeyClosesDialog(t *testing.T) {
	t.Parallel()

	d := newAgentHubDialog(t, &agentHubWorkspace{})
	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEscape})
	_, ok := action.(ActionClose)
	require.True(t, ok, "expected ActionClose, got %T", action)
}
