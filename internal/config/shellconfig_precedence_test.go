package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/stretchr/testify/require"
)

// TestShellConfigDotatlasrcTakesPrecedence verifies that a project-local
// .atlasrc overrides atlasrc in the same directory on conflicting settings.
func TestShellConfigDotatlasrcTakesPrecedence(t *testing.T) {
	isolated := t.TempDir()
	t.Setenv("HOME", isolated)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(isolated, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(isolated, ".local", "share"))

	workDir := t.TempDir()
	dataDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(workDir, "atlasrc"),
		[]byte("option notifications bell\n"), 0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(workDir, ".atlasrc"),
		[]byte("option notifications osc\n"), 0o644,
	))

	store, err := config.Load(workDir, dataDir, false)
	require.NoError(t, err)
	require.Equal(t, "osc", store.Config().Options.Notifications,
		".atlasrc should win over atlasrc")
}
