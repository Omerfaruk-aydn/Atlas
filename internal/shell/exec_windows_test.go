//go:build windows

package shell

import (
	"context"
	"testing"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/sandbox"
	"github.com/stretchr/testify/require"
)

// These tests toggle SetSandboxLimits, which is process-global state (see
// sandbox.go). None of them call t.Parallel(): Go only actually runs
// t.Parallel() tests concurrently with each other, deferred until every
// non-parallel test in the package has finished, so staying serial here
// is what keeps this safe alongside the rest of the package's (parallel)
// tests rather than racing the same toggle.

// "ping" is used here (rather than "echo" or "sleep") specifically
// because it is a real external process -- Go coreutils intercepts
// several common builtin-like names before they ever reach
// sandboxedExec, which would make a test built on one of those pass
// without actually exercising the sandboxing code path at all.

func TestSandboxedExecRunsNormalCommandsCorrectly(t *testing.T) {
	SetSandboxLimits(true, sandbox.Limits{})
	t.Cleanup(func() { SetSandboxLimits(false, sandbox.Limits{}) })

	bgManager := GetBackgroundShellManager()
	bgShell, err := bgManager.Start(t.Context(), t.TempDir(), nil, "ping -n 2 127.0.0.1", "")
	require.NoError(t, err)
	bgShell.Wait()

	stdout, _, done, execErr := bgShell.GetOutput()
	require.NoError(t, execErr)
	require.True(t, done)
	require.Contains(t, stdout, "Pinging 127.0.0.1")

	bgManager.Kill(bgShell.ID)
}

func TestSandboxedExecStillHonorsContextCancellation(t *testing.T) {
	SetSandboxLimits(true, sandbox.Limits{})
	t.Cleanup(func() { SetSandboxLimits(false, sandbox.Limits{}) })

	// A background shell manages its own detached context internally, so
	// exercise cancellation through Kill (which cancels that context)
	// rather than a context passed in from the test -- this is the same
	// mechanism the bash tool and job_kill use in production. -n 30 would
	// otherwise run for roughly 29 seconds.
	bgManager := GetBackgroundShellManager()
	bgShell, err := bgManager.Start(context.Background(), t.TempDir(), nil, "ping -n 30 127.0.0.1", "")
	require.NoError(t, err)

	start := time.Now()
	require.NoError(t, bgManager.Kill(bgShell.ID))

	select {
	case <-bgShell.done:
	case <-time.After(10 * time.Second):
		t.Fatal("sandboxed process outlived Kill")
	}
	require.Less(t, time.Since(start), 10*time.Second, "sandboxed exec should still be killed promptly")
}

func TestSandboxDisabledLeavesDefaultBehaviorInPlace(t *testing.T) {
	SetSandboxLimits(false, sandbox.Limits{})

	bgManager := GetBackgroundShellManager()
	bgShell, err := bgManager.Start(t.Context(), t.TempDir(), nil, "echo hello", "")
	require.NoError(t, err)
	bgShell.Wait()

	stdout, _, done, execErr := bgShell.GetOutput()
	require.NoError(t, execErr)
	require.True(t, done)
	require.Contains(t, stdout, "hello")

	bgManager.Kill(bgShell.ID)
}

func TestSetSandboxLimitsRefusesUnsupportedPlatformSilentlyDegrading(t *testing.T) {
	// Windows always supports it, so this just pins that enabling stays
	// enabled here -- the "unsupported, falling back" branch is exercised
	// by internal/sandbox's own non-Windows test instead, since Supported()
	// is hardcoded per platform.
	SetSandboxLimits(true, sandbox.Limits{})
	t.Cleanup(func() { SetSandboxLimits(false, sandbox.Limits{}) })

	limits, enabled := currentSandboxLimits()
	require.True(t, enabled)
	require.Equal(t, sandbox.Limits{}, limits)
}
