package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryShowBothScopesEmpty(t *testing.T) {
	workingDir := t.TempDir()
	c := newSkillTestCmd(t, runMemoryShow, workingDir, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	require.Contains(t, out.String(), "# project")
	require.Contains(t, out.String(), "# user")
	require.Contains(t, out.String(), "(empty)")
}

func TestMemoryShowOneScope(t *testing.T) {
	workingDir := t.TempDir()
	dataDir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "memory"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "memory", "MEMORY.md"), []byte("- go 1.26 is pinned\n"), 0o644))

	c := newSkillTestCmd(t, runMemoryShow, workingDir, dataDir)
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, []string{"project"}))
	require.Contains(t, out.String(), "go 1.26 is pinned")
	require.NotContains(t, out.String(), "# user")
}

func TestMemoryShowRejectsAnUnknownScope(t *testing.T) {
	workingDir := t.TempDir()
	c := newSkillTestCmd(t, runMemoryShow, workingDir, t.TempDir())

	err := c.RunE(c, []string{"everywhere"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown scope")
}

func TestMemoryClear(t *testing.T) {
	workingDir := t.TempDir()
	dataDir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "memory"), 0o755))
	memPath := filepath.Join(dataDir, "memory", "MEMORY.md")
	require.NoError(t, os.WriteFile(memPath, []byte("- something learned\n"), 0o644))

	c := newSkillTestCmd(t, runMemoryClear, workingDir, dataDir)
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, []string{"project"}))
	require.Contains(t, out.String(), "Cleared project memory")

	// An emptied store is no file at all -- see memory.Store.write.
	_, err := os.Stat(memPath)
	require.True(t, os.IsNotExist(err))
}
