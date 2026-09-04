package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runEnvVarAuditTool(t *testing.T, workingDir string, params EnvVarAuditParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewEnvVarAuditTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  EnvVarAuditToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

const envAuditFixture = `package a

import "os"

func f() {
	os.Getenv("API_KEY")
}
`

func TestEnvVarAuditToolReportsUsages(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", envAuditFixture)

	resp := runEnvVarAuditTool(t, dir, EnvVarAuditParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "1 distinct variable(s), 1 usage(s)")
	require.Contains(t, resp.Content, "API_KEY")
	require.Contains(t, resp.Content, "No .env.example")
}

func TestEnvVarAuditToolReportsUndocumented(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", envAuditFixture)
	writeDeadCodeFile(t, dir, ".env.example", "OTHER_VAR=x\n")

	resp := runEnvVarAuditTool(t, dir, EnvVarAuditParams{})
	require.Contains(t, resp.Content, "Undocumented")
	require.Contains(t, resp.Content, "API_KEY")
}

func TestEnvVarAuditToolReportsFullyDocumented(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", envAuditFixture)
	writeDeadCodeFile(t, dir, ".env.example", "API_KEY=x\n")

	resp := runEnvVarAuditTool(t, dir, EnvVarAuditParams{})
	require.Contains(t, resp.Content, "Every variable read is documented")
}

func TestEnvVarAuditToolReportsNoFiles(t *testing.T) {
	dir := t.TempDir()
	resp := runEnvVarAuditTool(t, dir, EnvVarAuditParams{})
	require.Contains(t, resp.Content, "No Go files found")
}

func TestEnvVarAuditToolReportsNoUsages(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", "package a\n\nfunc f() {}\n")

	resp := runEnvVarAuditTool(t, dir, EnvVarAuditParams{})
	require.Contains(t, resp.Content, "No os.Getenv/LookupEnv/Setenv calls found")
}

func TestEnvVarAuditToolReportsMetadata(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", envAuditFixture)
	writeDeadCodeFile(t, dir, ".env.example", "OTHER=1\n")

	resp := runEnvVarAuditTool(t, dir, EnvVarAuditParams{})
	var meta EnvVarAuditResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.Total)
	require.Equal(t, 1, meta.Undocumented)
	require.True(t, meta.EnvFileFound)
	require.Equal(t, 1, meta.FilesScanned)
}

func TestEnvVarAuditToolResolvesARelativePath(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "sub/a.go", envAuditFixture)

	resp := runEnvVarAuditTool(t, dir, EnvVarAuditParams{Path: "sub"})
	require.Contains(t, resp.Content, "API_KEY")
}
