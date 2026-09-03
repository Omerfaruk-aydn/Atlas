package gotest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// testModule writes a real Go module. Testing a `go test` wrapper against
// anything but a real toolchain run tests the fixture, not the wrapper.
func testModule(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/testmod\n\ngo 1.24\n"), 0o644))
	for name, src := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(src), 0o644))
	}
	return dir
}

func testByName(t *testing.T, r Result, name string) Test {
	t.Helper()
	for _, tc := range r.Tests {
		if tc.Name == name {
			return tc
		}
	}
	t.Fatalf("no result for %q (have %d tests)", name, len(r.Tests))
	return Test{}
}

func TestRunReportsPassesAndFailures(t *testing.T) {
	dir := testModule(t, map[string]string{
		"a_test.go": `package a

import "testing"

func TestPasses(t *testing.T) {}

func TestFails(t *testing.T) { t.Error("deliberate failure") }
`,
	})

	got, err := Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	require.Equal(t, 1, got.Passed)
	require.Equal(t, 1, got.Failed)
	require.False(t, got.OK())
	require.Equal(t, StatusPass, testByName(t, got, "TestPasses").Status)
	require.Equal(t, StatusFail, testByName(t, got, "TestFails").Status)
}

// A failure that does not carry its message forces a second tool call to
// find out what happened.
func TestRunKeepsOutputFromFailingTests(t *testing.T) {
	dir := testModule(t, map[string]string{
		"a_test.go": `package a

import "testing"

func TestFails(t *testing.T) { t.Error("the distinctive message") }
`,
	})

	got, err := Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	require.Contains(t, testByName(t, got, "TestFails").Output, "the distinctive message")
}

// A passing test's log is noise, and keeping all of it is how a large
// run turns into megabytes.
func TestRunDropsOutputFromPassingTestsUnlessVerbose(t *testing.T) {
	dir := testModule(t, map[string]string{
		"a_test.go": `package a

import "testing"

func TestPasses(t *testing.T) { t.Log("chatty passing test") }
`,
	})

	quiet, err := Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	require.Empty(t, testByName(t, quiet, "TestPasses").Output)

	loud, err := Run(context.Background(), dir, Options{Verbose: true, Count: 1})
	require.NoError(t, err)
	require.Contains(t, testByName(t, loud, "TestPasses").Output, "chatty passing test")
}

// A package that does not compile runs no tests. Reporting that as "0
// failures" is the most dangerous wrong answer available here.
func TestRunReportsABuildFailureRatherThanZeroFailures(t *testing.T) {
	dir := testModule(t, map[string]string{
		"a_test.go": `package a

import "testing"

func TestBroken(t *testing.T) {
	this is not valid go
}
`,
	})

	got, err := Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	require.False(t, got.OK())
	require.NotEmpty(t, got.Packages)
	require.NotEmpty(t, got.Packages[0].BuildError)
}

func TestRunSelectsTestsByName(t *testing.T) {
	dir := testModule(t, map[string]string{
		"a_test.go": `package a

import "testing"

func TestAlpha(t *testing.T) {}
func TestBeta(t *testing.T)  {}
`,
	})

	got, err := Run(context.Background(), dir, Options{Run: "TestAlpha"})
	require.NoError(t, err)
	require.Equal(t, 1, got.Passed)
	require.Equal(t, "TestAlpha", got.Tests[0].Name)
}

func TestRunCountsSkips(t *testing.T) {
	dir := testModule(t, map[string]string{
		"a_test.go": `package a

import "testing"

func TestSkipped(t *testing.T) { t.Skip("not today") }
`,
	})

	got, err := Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	require.Equal(t, 1, got.Skipped)
	// A skip is not a failure: the run is still OK.
	require.True(t, got.OK())
}

func TestRunReportsSubtests(t *testing.T) {
	dir := testModule(t, map[string]string{
		"a_test.go": `package a

import "testing"

func TestParent(t *testing.T) {
	t.Run("child_ok", func(t *testing.T) {})
	t.Run("child_bad", func(t *testing.T) { t.Error("nope") })
}
`,
	})

	got, err := Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	require.Equal(t, StatusFail, testByName(t, got, "TestParent/child_bad").Status)
	require.Equal(t, StatusPass, testByName(t, got, "TestParent/child_ok").Status)
}

// Failures have to sort to the top; a reader scrolling for them in a
// thousand-test run is the failure mode this avoids.
func TestRunSortsFailuresFirst(t *testing.T) {
	dir := testModule(t, map[string]string{
		"a_test.go": `package a

import "testing"

func TestAaaPasses(t *testing.T) {}
func TestZzzFails(t *testing.T)  { t.Error("x") }
`,
	})

	got, err := Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	require.Equal(t, StatusFail, got.Tests[0].Status)
}

func TestRunReportsAnEmptyRun(t *testing.T) {
	dir := testModule(t, map[string]string{
		"a.go": `package a

func Nothing() {}
`,
	})

	got, err := Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	require.True(t, got.NoTests)
	require.True(t, got.OK())
}

// A failing suite is a normal result, not a tool error -- the parsed
// output already says what went wrong.
func TestRunDoesNotTreatAFailingSuiteAsAnError(t *testing.T) {
	dir := testModule(t, map[string]string{
		"a_test.go": `package a

import "testing"

func TestFails(t *testing.T) { t.Fatal("x") }
`,
	})

	_, err := Run(context.Background(), dir, Options{})
	require.NoError(t, err)
}

// A pattern that resolves to nothing is reported through the same path
// as a compile failure -- go test emits it as build output on stdout, not
// as a process error. What matters is that it never reads as a clean run.
func TestRunSurfacesAnUnresolvablePattern(t *testing.T) {
	dir := testModule(t, map[string]string{"a.go": "package a\n"})

	got, err := Run(context.Background(), dir, Options{Packages: "./no-such-package-anywhere/..."})
	require.NoError(t, err)
	require.False(t, got.OK())
	require.NotEmpty(t, got.Packages)
	require.NotEmpty(t, got.Packages[0].BuildError)
}

func TestRunWritesACoverageProfileWhenAsked(t *testing.T) {
	dir := testModule(t, map[string]string{
		"a.go": `package a

func Add(x, y int) int { return x + y }
`,
		"a_test.go": `package a

import "testing"

func TestAdd(t *testing.T) {
	if Add(1, 2) != 3 {
		t.Fatal("wrong")
	}
}
`,
	})

	profile := filepath.Join(dir, "cover.out")
	got, err := Run(context.Background(), dir, Options{CoverProfile: profile})
	require.NoError(t, err)
	require.True(t, got.OK())

	data, err := os.ReadFile(profile)
	require.NoError(t, err)
	require.Contains(t, string(data), "mode:")
}

func TestRunRecordsElapsedTime(t *testing.T) {
	dir := testModule(t, map[string]string{
		"a_test.go": `package a

import (
	"testing"
	"time"
)

func TestSlow(t *testing.T) { time.Sleep(20 * time.Millisecond) }
`,
	})

	got, err := Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	require.Positive(t, testByName(t, got, "TestSlow").Elapsed)
}

// A hung test must produce a diagnosable panic from the test binary's own
// deadline, not a silently killed process.
func TestRunTimesOutOnAHangingTest(t *testing.T) {
	dir := testModule(t, map[string]string{
		"a_test.go": `package a

import (
	"testing"
	"time"
)

func TestHangs(t *testing.T) { time.Sleep(30 * time.Second) }
`,
	})

	got, err := Run(context.Background(), dir, Options{Timeout: 2 * time.Second})
	require.NoError(t, err)
	require.False(t, got.OK())
}

func TestOKRequiresEveryPackageToCompile(t *testing.T) {
	r := Result{Packages: []PackageResult{{Package: "x", BuildError: "syntax error"}}}
	require.False(t, r.OK())

	r = Result{Packages: []PackageResult{{Package: "x", Status: StatusPass}}}
	require.True(t, r.OK())
}
