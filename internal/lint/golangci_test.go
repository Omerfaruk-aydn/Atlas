package lint

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func requireGolangciLint(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("golangci-lint")
	if err != nil {
		t.Skip("golangci-lint not on PATH")
	}
	return bin
}

// golangci-lint writes its JSON report to stdout and then follows it with
// a human-readable tally ("1 issues:\n* govet: 1"). Unmarshalling the
// whole buffer fails on that trailing text, which silently demotes every
// real run to the much weaker vet fallback -- a regression that looks
// like "the linter found nothing" rather than like a bug.
func TestGolangciLintReportSurvivesTheTrailingSummary(t *testing.T) {
	bin := requireGolangciLint(t)
	dir := lintModule(t, map[string]string{"a.go": vetCatchesThis})

	got, err := runGolangciLint(context.Background(), dir, bin, Options{
		Packages:  "./...",
		Timeout:   2 * time.Minute,
		MaxIssues: 50,
	})
	require.NoError(t, err)
	require.Equal(t, "golangci-lint", got.Tool)
	require.NotEmpty(t, got.Issues, "the printf mismatch must be reported")
	require.Equal(t, "govet", got.Issues[0].Linter)
	require.Contains(t, got.Issues[0].Message, "%d")
	require.Equal(t, 6, got.Issues[0].Line)
}

// The whole point of preferring golangci-lint is that it finds more than
// vet, so Run must actually reach it rather than falling back.
func TestRunPrefersGolangciLintWhenInstalled(t *testing.T) {
	requireGolangciLint(t)
	dir := lintModule(t, map[string]string{"a.go": vetCatchesThis})

	got, err := Run(context.Background(), dir, Options{})
	require.NoError(t, err)
	require.Equal(t, "golangci-lint", got.Tool)
	require.False(t, got.Fallback)
}

func TestGolangciLintFindsWhatVetDoesNot(t *testing.T) {
	requireGolangciLint(t)
	// An unchecked error is errcheck's territory; vet says nothing.
	dir := lintModule(t, map[string]string{
		"a.go": `package a

import "os"

func Bad() {
	os.Remove("/tmp/nothing")
}
`,
	})

	full, err := Run(context.Background(), dir, Options{})
	require.NoError(t, err)

	vetOnly, err := Run(context.Background(), dir, Options{ForceVet: true})
	require.NoError(t, err)

	require.Empty(t, vetOnly.Issues)
	require.NotEmpty(t, full.Issues, "errcheck should flag the unchecked os.Remove")
}

func TestGolangciLintReportsCleanCode(t *testing.T) {
	bin := requireGolangciLint(t)
	dir := lintModule(t, map[string]string{
		"a.go": `package a

// Add returns the sum of x and y.
func Add(x, y int) int { return x + y }
`,
	})

	got, err := runGolangciLint(context.Background(), dir, bin, Options{
		Packages:  "./...",
		Timeout:   2 * time.Minute,
		MaxIssues: 50,
		Linters:   []string{"govet"},
	})
	require.NoError(t, err)
	require.Empty(t, got.Issues)
	require.Equal(t, "golangci-lint", got.Tool)
}

func TestGolangciLintRestrictsToNamedLinters(t *testing.T) {
	bin := requireGolangciLint(t)
	dir := lintModule(t, map[string]string{
		"a.go": `package a

import "os"

func Bad() {
	os.Remove("/tmp/nothing")
}
`,
	})

	// errcheck's finding must disappear when only govet is enabled.
	got, err := runGolangciLint(context.Background(), dir, bin, Options{
		Packages:  "./...",
		Timeout:   2 * time.Minute,
		MaxIssues: 50,
		Linters:   []string{"govet"},
	})
	require.NoError(t, err)
	for _, issue := range got.Issues {
		require.Equal(t, "govet", issue.Linter)
	}
}
