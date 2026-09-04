package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runCICDPipelineDebuggerTool(t *testing.T, workingDir string, params CICDPipelineDebuggerParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewCICDPipelineDebuggerTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  CICDPipelineDebuggerToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

const workflowFixture = `jobs:
  build:
    steps:
      - uses: actions/checkout@main
`

func TestCICDPipelineDebuggerToolReportsFindings(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ci.yml"), []byte(workflowFixture), 0o644))

	resp := runCICDPipelineDebuggerTool(t, dir, CICDPipelineDebuggerParams{Path: "ci.yml"})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "build")
	require.Contains(t, resp.Content, "unpinned-action")
	require.Contains(t, resp.Content, "missing-timeout")
}

func TestCICDPipelineDebuggerToolReportsClean(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ci.yml"), []byte(`jobs:
  build:
    timeout-minutes: 10
    steps:
      - uses: actions/checkout@v4
`), 0o644))

	resp := runCICDPipelineDebuggerTool(t, dir, CICDPipelineDebuggerParams{Path: "ci.yml"})
	require.Contains(t, resp.Content, "No issues found")
}

func TestCICDPipelineDebuggerToolRequiresPath(t *testing.T) {
	dir := t.TempDir()
	resp := runCICDPipelineDebuggerTool(t, dir, CICDPipelineDebuggerParams{})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "path is required")
}

func TestCICDPipelineDebuggerToolReportsMissingFile(t *testing.T) {
	dir := t.TempDir()
	resp := runCICDPipelineDebuggerTool(t, dir, CICDPipelineDebuggerParams{Path: "nope.yml"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "no workflow file found")
}

func TestCICDPipelineDebuggerToolReportsNoJobs(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ci.yml"), []byte("name: empty\n"), 0o644))

	resp := runCICDPipelineDebuggerTool(t, dir, CICDPipelineDebuggerParams{Path: "ci.yml"})
	require.Contains(t, resp.Content, "nothing to check")
}

func TestCICDPipelineDebuggerToolReportsMetadata(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ci.yml"), []byte(workflowFixture), 0o644))

	resp := runCICDPipelineDebuggerTool(t, dir, CICDPipelineDebuggerParams{Path: "ci.yml"})
	var meta CICDPipelineDebuggerResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.JobsFound)
	require.Equal(t, 2, meta.Total)
}
