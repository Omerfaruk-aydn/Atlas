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

func runDockerBuildExplainTool(t *testing.T, workingDir string, params DockerBuildExplainParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewDockerBuildExplainTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  DockerBuildExplainToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

func TestDockerBuildExplainToolReportsFindings(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM alpine\nRUN echo hi\n"), 0o644))

	resp := runDockerBuildExplainTool(t, dir, DockerBuildExplainParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "1 build stage(s)")
	require.Contains(t, resp.Content, "latest-tag")
	require.Contains(t, resp.Content, "root-user")
}

func TestDockerBuildExplainToolReportsClean(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM alpine:3.19\nUSER app\n"), 0o644))

	resp := runDockerBuildExplainTool(t, dir, DockerBuildExplainParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "No issues found")
}

func TestDockerBuildExplainToolReportsMissingFile(t *testing.T) {
	dir := t.TempDir()
	resp := runDockerBuildExplainTool(t, dir, DockerBuildExplainParams{})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "no Dockerfile found")
}

func TestDockerBuildExplainToolResolvesACustomPath(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile.prod"), []byte("FROM alpine:3.19\nUSER app\n"), 0o644))

	resp := runDockerBuildExplainTool(t, dir, DockerBuildExplainParams{Path: "Dockerfile.prod"})
	require.Contains(t, resp.Content, "No issues found")
}

func TestDockerBuildExplainToolReportsMetadata(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM alpine\nRUN echo hi\n"), 0o644))

	resp := runDockerBuildExplainTool(t, dir, DockerBuildExplainParams{})
	var meta DockerBuildExplainResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.Stages)
	require.Equal(t, 2, meta.Total)
	require.Equal(t, 1, meta.ByKind["latest-tag"])
	require.Equal(t, 1, meta.ByKind["root-user"])
}

func TestDockerBuildExplainToolReportsMultipleStages(t *testing.T) {
	dir := t.TempDir()
	content := "FROM golang:1.22 AS builder\nRUN go build -o app .\n\nFROM alpine:3.19\nCOPY --from=builder /app /app\nUSER nonroot\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(content), 0o644))

	resp := runDockerBuildExplainTool(t, dir, DockerBuildExplainParams{})
	require.Contains(t, resp.Content, "2 build stage(s)")
	require.Contains(t, resp.Content, "builder: FROM golang:1.22")
}
