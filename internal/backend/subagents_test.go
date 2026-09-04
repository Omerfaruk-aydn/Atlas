package backend

import (
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/subagents"
	"github.com/stretchr/testify/require"
)

func TestBackendSubagentLifecycle(t *testing.T) {
	b, ws, _ := newPublishingWorkspace(t)

	list, err := b.ListSubagents(ws.ID)
	require.NoError(t, err)
	require.Empty(t, list)

	path, err := b.SaveSubagent(ws.ID, subagents.Subagent{
		Name: "research", Description: "Deep research.", Instructions: "Dig deep.",
	}, false)
	require.NoError(t, err)
	require.NotEmpty(t, path)

	list, err = b.ListSubagents(ws.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "research", list[0].Name)
	require.Equal(t, "Deep research.", list[0].Description)

	// Saving again by the same name updates it in place rather than
	// creating a second entry.
	_, err = b.SaveSubagent(ws.ID, subagents.Subagent{
		Name: "research", Description: "Even deeper research.", Instructions: "Dig deeper.",
	}, false)
	require.NoError(t, err)

	list, err = b.ListSubagents(ws.ID)
	require.NoError(t, err)
	require.Len(t, list, 1, "saving by an existing name must update, not duplicate")
	require.Equal(t, "Even deeper research.", list[0].Description)

	require.NoError(t, b.DeleteSubagent(ws.ID, "research"))

	list, err = b.ListSubagents(ws.ID)
	require.NoError(t, err)
	require.Empty(t, list)
}

func TestBackendDeleteSubagentUnknownNameErrors(t *testing.T) {
	b, ws, _ := newPublishingWorkspace(t)

	err := b.DeleteSubagent(ws.ID, "nope")
	require.Error(t, err)
	require.Contains(t, err.Error(), `no subagent named "nope"`)
}

func TestBackendSaveSubagentRejectsInvalidDefinition(t *testing.T) {
	b, ws, _ := newPublishingWorkspace(t)

	_, err := b.SaveSubagent(ws.ID, subagents.Subagent{Name: "", Description: "d"}, false)
	require.Error(t, err)
}
