package debugger

import (
	"context"
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// locateDlv finds a real `dlv` binary to drive these integration tests
// against. It checks PATH first, then GOPATH/bin (where `go install
// .../dlv@latest` puts it, which is not always on PATH itself). Skips the
// test rather than failing when neither has it -- these are real,
// end-to-end tests against Delve, not something every CI runner is
// expected to have installed.
func locateDlv(t *testing.T) string {
	t.Helper()
	if p, err := exec.LookPath("dlv"); err == nil {
		return p
	}
	candidate := filepath.Join(build.Default.GOPATH, "bin", "dlv"+ExeSuffix())
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	t.Skip("dlv not found on PATH or in GOPATH/bin; skipping Delve integration test")
	return ""
}

// writeTestProgram writes a tiny, deterministic Go program to a temp
// module directory and returns (dir, breakpointLine) -- the line number of
// "sum += i", a stable place to stop mid-loop and inspect state.
func writeTestProgram(t *testing.T) (dir string, breakpointLine int) {
	t.Helper()
	dir = t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module debuggertestprogram\n\ngo 1.21\n"), 0o644))

	src := `package main

import "fmt"

func main() {
	sum := 0
	for i := 0; i < 3; i++ {
		sum += i
		fmt.Println(sum)
	}
	fmt.Println("done")
}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644))

	// "sum += i" is the 8th line of src above -- counted, not guessed, so
	// a future edit to this literal doesn't silently desync the test.
	breakpointLine = 8
	return dir, breakpointLine
}

func startTestSession(t *testing.T, program string) *Session {
	t.Helper()
	dlvPath := locateDlv(t)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	s, err := Start(ctx, Options{DlvPath: dlvPath, ActionTimeout: 15 * time.Second}, program, nil)
	require.NoError(t, err)
	t.Cleanup(s.Close)
	return s
}

func TestSessionStopsAtEntryOnStart(t *testing.T) {
	dir, _ := writeTestProgram(t)
	s := startTestSession(t, dir)

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	stop, exitCode, err := s.WaitStopped(ctx)
	require.NoError(t, err)
	require.Nil(t, exitCode)
	require.NotNil(t, stop)
	require.Equal(t, "entry", stop.Reason)
}

func TestSessionHitsABreakpointAndReadsAVariable(t *testing.T) {
	dir, bpLine := writeTestProgram(t)
	s := startTestSession(t, dir)

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	_, _, err := s.WaitStopped(ctx) // consume the entry stop
	require.NoError(t, err)

	mainGo := filepath.Join(dir, "main.go")
	bps, err := s.SetBreakpoints(ctx, mainGo, []int{bpLine})
	require.NoError(t, err)
	require.Len(t, bps, 1)
	require.True(t, bps[0].Verified, "breakpoint at %s:%d should verify", mainGo, bpLine)

	require.NoError(t, s.Continue(ctx))
	stop, exitCode, err := s.WaitStopped(ctx)
	require.NoError(t, err)
	require.Nil(t, exitCode)
	require.NotNil(t, stop)
	require.Equal(t, "breakpoint", stop.Reason)
	require.Equal(t, bpLine, stop.Line)

	frames, err := s.StackTrace(ctx, 0, 1)
	require.NoError(t, err)
	require.NotEmpty(t, frames)

	scopes, err := s.Scopes(ctx, frames[0].Id)
	require.NoError(t, err)
	require.NotEmpty(t, scopes)

	var found bool
	for _, scope := range scopes {
		vars, err := s.Variables(ctx, scope.VariablesReference)
		require.NoError(t, err)
		for _, v := range vars {
			if v.Name == "sum" {
				found = true
				require.Equal(t, "0", v.Value, "sum should still be 0 the first time this breakpoint is hit")
			}
		}
	}
	require.True(t, found, "expected a local variable named \"sum\" in scope")

	result, err := s.Evaluate(ctx, "sum", frames[0].Id)
	require.NoError(t, err)
	require.Equal(t, "0", result.Result)
}

func TestSessionStepsAndRunsToCompletion(t *testing.T) {
	dir, bpLine := writeTestProgram(t)
	s := startTestSession(t, dir)

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	_, _, err := s.WaitStopped(ctx) // entry
	require.NoError(t, err)

	_, err = s.SetBreakpoints(ctx, filepath.Join(dir, "main.go"), []int{bpLine})
	require.NoError(t, err)

	require.NoError(t, s.Continue(ctx))
	stop, _, err := s.WaitStopped(ctx) // first breakpoint hit, sum == 0
	require.NoError(t, err)
	require.Equal(t, "breakpoint", stop.Reason)

	require.NoError(t, s.Next(ctx))
	stop, _, err = s.WaitStopped(ctx) // stepped to the next line
	require.NoError(t, err)
	require.Equal(t, "step", stop.Reason)

	// Clear the breakpoint and let the rest of the loop run to completion.
	_, err = s.SetBreakpoints(ctx, filepath.Join(dir, "main.go"), nil)
	require.NoError(t, err)
	require.NoError(t, s.Continue(ctx))

	_, exitCode, err := s.WaitStopped(ctx)
	require.NoError(t, err)
	require.NotNil(t, exitCode)
	require.Equal(t, 0, *exitCode)

	// The debuggee's own stdout arrives as "output" events on the same
	// connection as the "exited" event just consumed above, but delve does
	// not guarantee it is flushed through before that event -- so poll
	// (accumulating each partial drain) instead of asserting on a single
	// immediate DrainOutput.
	var output string
	require.Eventually(t, func() bool {
		output += s.DrainOutput()
		return strings.Contains(output, "done")
	}, 2*time.Second, 20*time.Millisecond)
	require.Contains(t, output, "done")
}
