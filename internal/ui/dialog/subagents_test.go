package dialog

import (
	"context"
	"testing"

	tea "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-ui/v2"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/subagents"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/common"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/styles"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/workspace"
	"github.com/stretchr/testify/require"
)

// subagentsWorkspace is a minimal workspace.Workspace stub for the
// Subagents dialog: a fixed list for reads, and a recorded
// DeleteSubagent call for the write-side assertions.
type subagentsWorkspace struct {
	workspace.Workspace
	list       []subagents.Subagent
	listErr    error
	deletedFor string
	deleteErr  error
}

func (w *subagentsWorkspace) ListSubagents(context.Context) ([]subagents.Subagent, error) {
	return w.list, w.listErr
}

func (w *subagentsWorkspace) DeleteSubagent(_ context.Context, name string) error {
	w.deletedFor = name
	return w.deleteErr
}

func newSubagentsDialog(t *testing.T, ws *subagentsWorkspace) *Subagents {
	t.Helper()
	sty := styles.AtlasPantera()
	return NewSubagents(&common.Common{Workspace: ws, Styles: &sty})
}

func TestSubagentsListsDiscoveredEntriesSortedByName(t *testing.T) {
	ws := &subagentsWorkspace{list: []subagents.Subagent{
		{Name: "frontend", Description: "UI work.", Path: "/agents/frontend.md"},
		{Name: "backend", Description: "API work.", Path: "/agents/backend.md"},
	}}
	d := newSubagentsDialog(t, ws)

	require.Equal(t, SubagentsID, d.ID())
	require.Equal(t, 2, d.list.Len())
	first, ok := d.list.FilteredItems()[0].(*subagentEntry)
	require.True(t, ok)
	require.Equal(t, "backend", first.sub.Name, "entries must be sorted by name")
}

func TestSubagentsAddOpensEmptyForm(t *testing.T) {
	d := newSubagentsDialog(t, &subagentsWorkspace{})

	action := d.HandleMsg(keyMsg('a'))
	form, ok := action.(ActionOpenSubagentForm)
	require.True(t, ok, "expected ActionOpenSubagentForm, got %T", action)
	require.Empty(t, form.ExistingName)
}

func TestSubagentsEditOpensFormPrefilled(t *testing.T) {
	ws := &subagentsWorkspace{list: []subagents.Subagent{
		{Name: "research", Description: "Deep research.", Model: "@research", Path: "/agents/research.md"},
	}}
	d := newSubagentsDialog(t, ws)
	d.list.SelectFirst()

	action := d.HandleMsg(keyMsg('e'))
	form, ok := action.(ActionOpenSubagentForm)
	require.True(t, ok, "expected ActionOpenSubagentForm, got %T", action)
	require.Equal(t, "research", form.ExistingName)
	require.Equal(t, "Deep research.", form.ExistingDescription)
	require.Equal(t, "@research", form.ExistingModel)
}

func TestSubagentsEditFileOpensThatSubagentsPath(t *testing.T) {
	ws := &subagentsWorkspace{list: []subagents.Subagent{
		{Name: "research", Description: "d", Path: "/agents/research.md"},
	}}
	d := newSubagentsDialog(t, ws)
	d.list.SelectFirst()

	action := d.HandleMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	edit, ok := action.(ActionEditSubagentFile)
	require.True(t, ok, "expected ActionEditSubagentFile, got %T", action)
	require.Equal(t, "research", edit.Name)
	require.Equal(t, "/agents/research.md", edit.Path)
}

func TestSubagentsDeletePersistsAndRefreshes(t *testing.T) {
	ws := &subagentsWorkspace{list: []subagents.Subagent{
		{Name: "research", Description: "d", Path: "/agents/research.md"},
	}}
	d := newSubagentsDialog(t, ws)
	d.list.SelectFirst()

	action := d.HandleMsg(keyMsg('x'))
	cmdAction, ok := action.(ActionCmd)
	require.True(t, ok, "expected ActionCmd, got %T", action)

	msg := cmdAction.Cmd()
	require.Equal(t, "research", ws.deletedFor)

	// Simulate the file actually being gone, then let the dialog react.
	ws.list = nil
	result := d.HandleMsg(msg)
	require.Nil(t, result)
	require.Equal(t, 0, d.list.Len())
}

func TestSubagentsDeleteErrorIsReported(t *testing.T) {
	ws := &subagentsWorkspace{
		list:      []subagents.Subagent{{Name: "research", Description: "d", Path: "/agents/research.md"}},
		deleteErr: context.DeadlineExceeded,
	}
	d := newSubagentsDialog(t, ws)
	d.list.SelectFirst()

	action := d.HandleMsg(keyMsg('x'))
	cmdAction := action.(ActionCmd)
	msg := cmdAction.Cmd()

	result := d.HandleMsg(msg)
	_, ok := result.(ActionCmd)
	require.True(t, ok, "an error must surface as an ActionCmd reporting it")
}
