package subagents

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderRoundTrips(t *testing.T) {
	s := &Subagent{Name: "research", Description: "Deep research.", Model: "@research", Instructions: "Dig deep."}

	content, err := Render(s)
	require.NoError(t, err)

	got, err := ParseContent(content)
	require.NoError(t, err)
	require.Equal(t, s.Name, got.Name)
	require.Equal(t, s.Description, got.Description)
	require.Equal(t, s.Model, got.Model)
	require.Equal(t, s.Instructions, got.Instructions)
}

func TestRenderOmitsAnEmptyModel(t *testing.T) {
	content, err := Render(&Subagent{Name: "generic", Description: "d", Instructions: "i"})
	require.NoError(t, err)
	require.NotContains(t, string(content), "model:")
}

func TestSaveWritesAParsableFile(t *testing.T) {
	dir := t.TempDir()
	path, err := Save(dir, &Subagent{Name: "frontend", Description: "UI work.", Model: "@frontend", Instructions: "Body."})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "frontend.md"), path)

	got, err := Parse(path)
	require.NoError(t, err)
	require.Equal(t, "frontend", got.Name)
	require.Equal(t, "@frontend", got.Model)
}

func TestSaveRejectsAnInvalidSubagent(t *testing.T) {
	_, err := Save(t.TempDir(), &Subagent{Name: "", Description: "d"})
	require.Error(t, err)
}

func TestSaveCreatesTheDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "agents")
	_, err := Save(dir, &Subagent{Name: "x", Description: "d"})
	require.NoError(t, err)
	require.DirExists(t, dir)
}

func TestDeleteRemovesTheFile(t *testing.T) {
	dir := t.TempDir()
	path, err := Save(dir, &Subagent{Name: "old", Description: "d"})
	require.NoError(t, err)

	require.NoError(t, Delete(dir, "old"))
	_, err = os.Stat(path)
	require.True(t, os.IsNotExist(err))
}

func TestDeleteFailsWhenNothingIsThere(t *testing.T) {
	require.Error(t, Delete(t.TempDir(), "nope"))
}
