package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProjectSubagentsDirIncludesDotAtlasAgents(t *testing.T) {
	workingDir := t.TempDir()
	dirs := ProjectSubagentsDir(workingDir)

	require.Contains(t, dirs, filepath.Join(workingDir, ".atlas", "agents"))
}

func TestGlobalSubagentsDirsIsNonEmpty(t *testing.T) {
	require.NotEmpty(t, GlobalSubagentsDirs())
}

func TestGlobalSubagentsDirsRespectsEnvOverride(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "custom-agents")
	t.Setenv("ATLAS_AGENT_SUBAGENTS_DIR", custom)

	require.Equal(t, []string{custom}, GlobalSubagentsDirs())
}
