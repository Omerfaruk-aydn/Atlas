package cmd

import (
	"path/filepath"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/stretchr/testify/require"
)

// fireSessionEndHook is wired into runSessionDelete and runSessionPrune's
// delete loop; both call sites are one line and exercised by this package's
// existing session_delete/session_prune tests (which continue to pass
// unchanged with no SessionEnd hooks configured, proving the wiring is a
// no-op when there is nothing to run). This file tests the hook-firing
// logic itself directly, since session_*.go commands resolve config via
// sessionSetup's config.Init("", dataDir, false) -- deliberately without a
// project working directory, matching every other session_*.go command --
// so a project-level atlas.json is not the right fixture for exercising
// this through the CLI entry points; a global/user-level atlas.json is,
// and that is what fireSessionEndHook itself is agnostic to either way.
func TestFireSessionEndHookRunsTheConfiguredCommand(t *testing.T) {
	workingDir := t.TempDir()
	// A bare relative filename, not an absolute path, deliberately: hooks
	// run with the workspace's working directory as their cwd (see
	// hooks.Runner.cwd), so this both proves that and sidesteps having to
	// shell-quote an absolute temp path for this test.
	writeAtlasConfig(t, workingDir, `{"hooks":{"SessionEnd":[
		{"command":"echo fired > fired.txt"}
	]}}`)

	cfg, err := config.Init(workingDir, t.TempDir(), false)
	require.NoError(t, err)

	fireSessionEndHook(t.Context(), cfg, "some-session-id")

	require.FileExists(t, filepath.Join(workingDir, "fired.txt"))
}

func TestFireSessionEndHookWithNoneConfiguredDoesNothing(t *testing.T) {
	cfg, err := config.Init(t.TempDir(), t.TempDir(), false)
	require.NoError(t, err)

	// Must not panic or block when no SessionEnd hooks are configured.
	fireSessionEndHook(t.Context(), cfg, "some-session-id")
}

func TestFireSessionEndHookLogsAndSwallowsAFailingHook(t *testing.T) {
	workingDir := t.TempDir()
	writeAtlasConfig(t, workingDir, `{"hooks":{"SessionEnd":[
		{"command":"exit 1"}
	]}}`)

	cfg, err := config.Init(workingDir, t.TempDir(), false)
	require.NoError(t, err)

	// A hook that exits non-zero must not surface as an error the caller
	// has to handle: deletion already succeeded by the time this runs.
	fireSessionEndHook(t.Context(), cfg, "some-session-id")
}
