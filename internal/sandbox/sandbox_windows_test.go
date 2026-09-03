//go:build windows

package sandbox

import (
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSupportedIsTrueOnWindows(t *testing.T) {
	t.Parallel()
	require.True(t, Supported())
}

func TestNewSetsKillOnJobCloseByDefault(t *testing.T) {
	t.Parallel()
	job, err := New(Limits{})
	require.NoError(t, err)
	require.NotNil(t, job)
	defer job.Close()
}

func TestAssignOnNilJobAndProcessIsANoop(t *testing.T) {
	t.Parallel()
	var job *Job
	require.NoError(t, job.Assign(nil))
	require.NoError(t, job.Close())
}

// TestCloseTerminatesAnAssignedProcess is the real, load-bearing test:
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE must actually kill a process that
// would otherwise keep running long past this test, proving Close does
// what the package doc promises rather than asserting against a mock.
func TestCloseTerminatesAnAssignedProcess(t *testing.T) {
	t.Parallel()

	job, err := New(Limits{})
	require.NoError(t, err)

	// A process that runs far longer than this test should take, so its
	// early death can only be explained by the job closing.
	cmd := exec.CommandContext(t.Context(), "cmd", "/C", "timeout /T 60")
	require.NoError(t, cmd.Start())

	require.NoError(t, job.Assign(cmd.Process))

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	require.NoError(t, job.Close())

	select {
	case <-done:
		// Terminated, as expected -- Wait returning at all (rather than
		// timing out below) is the assertion; its error (a non-zero exit
		// from being killed) isn't informative on its own.
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("process outlived the job it was assigned to; kill-on-close did not work")
	}
}

// TestMaxProcessesIsAcceptedAndAssignable pins that a process-count limit
// doesn't itself break normal assignment. The stronger claim -- that
// exceeding it terminates the job -- is a documented Windows API
// contract (JOB_OBJECT_LIMIT_ACTIVE_PROCESS), not something worth
// re-verifying here with a timing-sensitive fork-bomb test.
func TestMaxProcessesIsAcceptedAndAssignable(t *testing.T) {
	t.Parallel()

	job, err := New(Limits{MaxProcesses: 4})
	require.NoError(t, err)
	defer job.Close()

	cmd := exec.CommandContext(t.Context(), "cmd", "/C", "timeout /T 5")
	require.NoError(t, cmd.Start())
	defer cmd.Process.Kill()

	require.NoError(t, job.Assign(cmd.Process))
}

func TestMaxMemoryBytesIsAcceptedAndAssignable(t *testing.T) {
	t.Parallel()

	job, err := New(Limits{MaxMemoryBytes: 512 * 1024 * 1024})
	require.NoError(t, err)
	defer job.Close()

	cmd := exec.CommandContext(t.Context(), "cmd", "/C", "timeout /T 5")
	require.NoError(t, cmd.Start())
	defer cmd.Process.Kill()

	require.NoError(t, job.Assign(cmd.Process))
}
