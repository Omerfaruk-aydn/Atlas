package cicdx

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeWorkflow(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ci.yml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func findCIFinding(t *testing.T, findings []Finding, kind string) *Finding {
	t.Helper()
	for i := range findings {
		if findings[i].Kind == kind {
			return &findings[i]
		}
	}
	return nil
}

func TestParseFlagsUnpinnedBranchRef(t *testing.T) {
	path := writeWorkflow(t, `
jobs:
  build:
    timeout-minutes: 10
    steps:
      - uses: actions/checkout@main
`)

	got, err := Parse(path)
	require.NoError(t, err)
	f := findCIFinding(t, got.Findings, "unpinned-action")
	require.NotNil(t, f)
	require.Equal(t, "build", f.Job)
}

func TestParseAcceptsAVersionTag(t *testing.T) {
	path := writeWorkflow(t, `
jobs:
  build:
    timeout-minutes: 10
    steps:
      - uses: actions/checkout@v4
`)

	got, err := Parse(path)
	require.NoError(t, err)
	require.Nil(t, findCIFinding(t, got.Findings, "unpinned-action"))
}

func TestParseAcceptsACommitSHA(t *testing.T) {
	path := writeWorkflow(t, `
jobs:
  build:
    timeout-minutes: 10
    steps:
      - uses: actions/checkout@8f4b7f84864484a7bf31766abe9204da3cbe65b3
`)

	got, err := Parse(path)
	require.NoError(t, err)
	require.Nil(t, findCIFinding(t, got.Findings, "unpinned-action"))
}

func TestParseIgnoresLocalActions(t *testing.T) {
	path := writeWorkflow(t, `
jobs:
  build:
    timeout-minutes: 10
    steps:
      - uses: ./.github/actions/build
`)

	got, err := Parse(path)
	require.NoError(t, err)
	require.Nil(t, findCIFinding(t, got.Findings, "unpinned-action"))
}

func TestParseFlagsMissingTimeout(t *testing.T) {
	path := writeWorkflow(t, `
jobs:
  build:
    steps:
      - uses: actions/checkout@v4
`)

	got, err := Parse(path)
	require.NoError(t, err)
	f := findCIFinding(t, got.Findings, "missing-timeout")
	require.NotNil(t, f)
	require.Equal(t, "build", f.Job)
}

func TestParseAcceptsATimeout(t *testing.T) {
	path := writeWorkflow(t, `
jobs:
  build:
    timeout-minutes: 15
    steps:
      - uses: actions/checkout@v4
`)

	got, err := Parse(path)
	require.NoError(t, err)
	require.Nil(t, findCIFinding(t, got.Findings, "missing-timeout"))
}

func TestParseFlagsSecretEchoedToLog(t *testing.T) {
	path := writeWorkflow(t, `
jobs:
  build:
    timeout-minutes: 10
    steps:
      - name: debug
        run: echo ${{ secrets.API_TOKEN }}
`)

	got, err := Parse(path)
	require.NoError(t, err)
	f := findCIFinding(t, got.Findings, "secret-in-run")
	require.NotNil(t, f)
	require.Equal(t, "debug", f.Step)
}

func TestParseIgnoresASecretUsedWithoutPrinting(t *testing.T) {
	path := writeWorkflow(t, `
jobs:
  build:
    timeout-minutes: 10
    steps:
      - name: deploy
        run: |
          curl -H "Authorization: Bearer ${{ secrets.API_TOKEN }}" https://example.com
`)

	got, err := Parse(path)
	require.NoError(t, err)
	require.Nil(t, findCIFinding(t, got.Findings, "secret-in-run"))
}

func TestParseReportsJobsFound(t *testing.T) {
	path := writeWorkflow(t, `
jobs:
  build:
    timeout-minutes: 10
    steps: []
  test:
    timeout-minutes: 10
    steps: []
`)

	got, err := Parse(path)
	require.NoError(t, err)
	require.Equal(t, 2, got.JobsFound)
}

func TestParseReportsNoJobs(t *testing.T) {
	path := writeWorkflow(t, "name: empty\n")

	got, err := Parse(path)
	require.NoError(t, err)
	require.Equal(t, 0, got.JobsFound)
	require.Empty(t, got.Findings)
}

func TestParseReportsErrorForMissingFile(t *testing.T) {
	_, err := Parse(filepath.Join(t.TempDir(), "nope.yml"))
	require.Error(t, err)
}

func TestParseUsesStepNameAsLabelFallback(t *testing.T) {
	path := writeWorkflow(t, `
jobs:
  build:
    timeout-minutes: 10
    steps:
      - uses: actions/checkout@main
`)

	got, err := Parse(path)
	require.NoError(t, err)
	f := findCIFinding(t, got.Findings, "unpinned-action")
	require.NotNil(t, f)
	require.Contains(t, f.Step, "actions/checkout@main")
}
