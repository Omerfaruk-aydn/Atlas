package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runTestRunTool(t *testing.T, workingDir string, params TestRunParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewTestRunTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  TestRunToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

func goModule(t *testing.T, files map[string]string) string {
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

func TestTestRunToolReportsAGreenRun(t *testing.T) {
	dir := goModule(t, map[string]string{
		"a_test.go": `package a

import "testing"

func TestPasses(t *testing.T) {}
`,
	})

	resp := runTestRunTool(t, dir, TestRunParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "PASS: 1 passed")
}

// The failure's own message has to travel with it, or finding out what
// broke costs a second tool call.
func TestTestRunToolShowsWhyATestFailed(t *testing.T) {
	dir := goModule(t, map[string]string{
		"a_test.go": `package a

import "testing"

func TestFails(t *testing.T) { t.Error("the distinctive message") }
`,
	})

	resp := runTestRunTool(t, dir, TestRunParams{})
	require.Contains(t, resp.Content, "FAIL: 1 failed")
	require.Contains(t, resp.Content, "--- FAIL TestFails")
	require.Contains(t, resp.Content, "the distinctive message")
}

// A headline of "0 failed" above a silent compile error is the single
// most misleading thing this tool could print.
func TestTestRunToolLeadsWithBuildFailures(t *testing.T) {
	dir := goModule(t, map[string]string{
		"a_test.go": `package a

import "testing"

func TestBroken(t *testing.T) {
	this is not valid go
}
`,
	})

	resp := runTestRunTool(t, dir, TestRunParams{})
	require.Contains(t, resp.Content, "DID NOT COMPILE")
	require.Contains(t, resp.Content, "their tests never ran")
	require.NotContains(t, resp.Content, "PASS:")

	var meta TestRunResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.False(t, meta.OK)
	require.Equal(t, 1, meta.BuildFailures)
}

func TestTestRunToolSelectsTestsByName(t *testing.T) {
	dir := goModule(t, map[string]string{
		"a_test.go": `package a

import "testing"

func TestAlpha(t *testing.T) {}
func TestBeta(t *testing.T)  { t.Error("would fail") }
`,
	})

	resp := runTestRunTool(t, dir, TestRunParams{Run: "TestAlpha"})
	require.Contains(t, resp.Content, "PASS: 1 passed")
	require.Contains(t, resp.Content, "matching TestAlpha")
}

// "No tests ran" gets read as "everything is fine" unless it says why it
// might have happened.
func TestTestRunToolExplainsAnEmptyRun(t *testing.T) {
	dir := goModule(t, map[string]string{
		"a.go": `package a

func Nothing() {}
`,
	})

	resp := runTestRunTool(t, dir, TestRunParams{})
	require.Contains(t, resp.Content, "No tests ran")
	require.Contains(t, resp.Content, "no test files")
}

func TestTestRunToolCountsSkips(t *testing.T) {
	dir := goModule(t, map[string]string{
		"a_test.go": `package a

import "testing"

func TestSkipped(t *testing.T) { t.Skip("not today") }
func TestPasses(t *testing.T)  {}
`,
	})

	resp := runTestRunTool(t, dir, TestRunParams{})
	require.Contains(t, resp.Content, "1 skipped")

	var meta TestRunResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.True(t, meta.OK)
	require.Equal(t, 1, meta.Skipped)
}

func TestTestRunToolRejectsABadTimeout(t *testing.T) {
	resp := runTestRunTool(t, t.TempDir(), TestRunParams{Timeout: "soon"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "is not a duration")
}

func TestTestRunToolRejectsANonPositiveTimeout(t *testing.T) {
	resp := runTestRunTool(t, t.TempDir(), TestRunParams{Timeout: "0s"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "must be positive")
}

func TestTestRunToolReportsSubtestFailures(t *testing.T) {
	dir := goModule(t, map[string]string{
		"a_test.go": `package a

import "testing"

func TestParent(t *testing.T) {
	t.Run("child_bad", func(t *testing.T) { t.Error("nope") })
}
`,
	})

	resp := runTestRunTool(t, dir, TestRunParams{})
	require.Contains(t, resp.Content, "TestParent/child_bad")
}

func TestTestRunToolNarrowsToAPackage(t *testing.T) {
	dir := goModule(t, map[string]string{
		"good/a_test.go": `package good

import "testing"

func TestGood(t *testing.T) {}
`,
		"bad/b_test.go": `package bad

import "testing"

func TestBad(t *testing.T) { t.Error("x") }
`,
	})

	resp := runTestRunTool(t, dir, TestRunParams{Packages: "./good/..."})
	require.Contains(t, resp.Content, "PASS: 1 passed")
	require.NotContains(t, resp.Content, "TestBad")
}

func TestTestRunToolReportsMetadata(t *testing.T) {
	dir := goModule(t, map[string]string{
		"a_test.go": `package a

import "testing"

func TestPasses(t *testing.T) {}
func TestFails(t *testing.T)  { t.Error("x") }
`,
	})

	resp := runTestRunTool(t, dir, TestRunParams{})
	var meta TestRunResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.Passed)
	require.Equal(t, 1, meta.Failed)
	require.False(t, meta.OK)
	require.False(t, meta.TimedOut)
}

func TestShortPackageKeepsTheDistinguishingPart(t *testing.T) {
	require.Equal(t, "internal/agent/tools",
		shortPackage("github.com/example/repo/internal/agent/tools"))
	require.Equal(t, "example.com/m", shortPackage("example.com/m"))
}

func TestLimitLinesTruncatesWithACount(t *testing.T) {
	got := limitLines("a\nb\nc\nd", 2)
	require.Len(t, got, 3)
	require.Contains(t, got[2], "2 more lines")

	require.Nil(t, limitLines("   ", 5))
}
