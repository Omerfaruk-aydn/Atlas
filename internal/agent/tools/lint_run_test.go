package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runLintTool(t *testing.T, workingDir string, params LintRunParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewLintRunTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  LintRunToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

// A printf verb mismatch is one of the few things vet reports on every
// toolchain version, so it works as a fixture whichever linter runs.
const lintFixture = `package a

import "fmt"

func Bad() {
	fmt.Printf("%d\n", "not a number")
}
`

func TestLintToolReportsFindingsGroupedByFile(t *testing.T) {
	dir := goModule(t, map[string]string{"a.go": lintFixture})

	yes := true
	resp := runLintTool(t, dir, LintRunParams{ForceVet: &yes})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "issue(s) from go vet")
	require.Contains(t, resp.Content, "a.go")
	require.Contains(t, resp.Content, "%d")
}

func TestLintToolReportsACleanPackage(t *testing.T) {
	dir := goModule(t, map[string]string{
		"a.go": `package a

func Add(x, y int) int { return x + y }
`,
	})

	yes := true
	resp := runLintTool(t, dir, LintRunParams{ForceVet: &yes})
	require.Contains(t, resp.Content, "No issues found")
}

// "No issues from go vet" reads as "this code is clean", which it is not
// evidence for -- so the gap has to be stated where the result is read.
func TestLintToolExplainsWhatTheVetFallbackMisses(t *testing.T) {
	dir := goModule(t, map[string]string{"a.go": lintFixture})

	resp := runLintTool(t, dir, LintRunParams{})

	var meta LintRunResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	if !meta.Fallback {
		t.Skip("golangci-lint is installed, so there is no fallback to describe")
	}
	require.Contains(t, resp.Content, "golangci-lint was not available")
	require.Contains(t, resp.Content, "does not check unchecked errors")
	require.Contains(t, resp.Content, "nothing obviously broken")
}

// An explicit force_vet is a choice, not a fallback, so it must not carry
// the apologetic note.
func TestLintToolDoesNotCallAnExplicitVetRunAFallback(t *testing.T) {
	dir := goModule(t, map[string]string{"a.go": lintFixture})

	yes := true
	resp := runLintTool(t, dir, LintRunParams{ForceVet: &yes})
	require.NotContains(t, resp.Content, "was not available")

	var meta LintRunResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.False(t, meta.Fallback)
	require.Equal(t, "go vet", meta.Tool)
}

func TestLintToolNarrowsToAPackage(t *testing.T) {
	dir := goModule(t, map[string]string{
		"clean/a.go": `package clean

func Fine() {}
`,
		"dirty/b.go": `package dirty

import "fmt"

func Bad() { fmt.Printf("%d\n", "x") }
`,
	})

	yes := true
	resp := runLintTool(t, dir, LintRunParams{ForceVet: &yes, Packages: "./clean/..."})
	require.Contains(t, resp.Content, "No issues found")
	require.Contains(t, resp.Content, "./clean/...")
}

func TestLintToolRejectsABadTimeout(t *testing.T) {
	resp := runLintTool(t, t.TempDir(), LintRunParams{Timeout: "eventually"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "not a positive duration")
}

// A linter's opinions are not always right for a given piece of code, and
// a report without that caveat invites mechanical compliance.
func TestLintToolSaysFindingsNeedJudgement(t *testing.T) {
	dir := goModule(t, map[string]string{"a.go": lintFixture})

	yes := true
	resp := runLintTool(t, dir, LintRunParams{ForceVet: &yes})
	require.Contains(t, resp.Content, "encodes opinions")
}

func TestLintToolReportsMetadata(t *testing.T) {
	dir := goModule(t, map[string]string{"a.go": lintFixture})

	yes := true
	resp := runLintTool(t, dir, LintRunParams{ForceVet: &yes})
	var meta LintRunResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Positive(t, meta.Issues)
	require.Equal(t, "go vet", meta.Tool)
	require.False(t, meta.Truncated)
}

// The primary path is golangci-lint; a silent demotion to vet would look
// like "the linter found less", not like a bug.
func TestLintToolPrefersGolangciLintWhenAvailable(t *testing.T) {
	dir := goModule(t, map[string]string{"a.go": lintFixture})

	resp := runLintTool(t, dir, LintRunParams{})
	var meta LintRunResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	if meta.Fallback {
		t.Skip("golangci-lint not installed in this environment")
	}
	require.Equal(t, "golangci-lint", meta.Tool)
	require.Positive(t, meta.Issues)
}
