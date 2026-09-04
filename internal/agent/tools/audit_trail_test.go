package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runAuditTrailTool(t *testing.T, workingDir string, params AuditTrailParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewAuditTrailTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  AuditTrailToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

func TestAuditTrailToolReportsHistory(t *testing.T) {
	dir := preCommitRepo(t)
	writeDeadCodeFile(t, dir, "a.go", "package a\n\nfunc Foo() int {\n\treturn 1\n}\n")
	preCommitGit(t, dir, "add", "a.go")
	preCommitGit(t, dir, "commit", "-m", "add Foo")
	writeDeadCodeFile(t, dir, "a.go", "package a\n\nfunc Foo() int {\n\treturn 2\n}\n")
	preCommitGit(t, dir, "add", "a.go")
	preCommitGit(t, dir, "commit", "-m", "change Foo")

	resp := runAuditTrailTool(t, dir, AuditTrailParams{Path: "a.go", Symbol: "Foo"})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "2 commit(s)")
	require.Contains(t, resp.Content, "change Foo")
	require.Contains(t, resp.Content, "add Foo")
}

func TestAuditTrailToolReportsFunctionNotFound(t *testing.T) {
	dir := preCommitRepo(t)
	writeDeadCodeFile(t, dir, "a.go", "package a\n\nfunc Foo() int {\n\treturn 1\n}\n")
	preCommitGit(t, dir, "add", "a.go")
	preCommitGit(t, dir, "commit", "-m", "add Foo")

	resp := runAuditTrailTool(t, dir, AuditTrailParams{Path: "a.go", Symbol: "Missing"})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, `No function named "Missing" found`)
}

func TestAuditTrailToolRequiresPath(t *testing.T) {
	dir := t.TempDir()
	resp := runAuditTrailTool(t, dir, AuditTrailParams{Symbol: "Foo"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "path is required")
}

func TestAuditTrailToolRequiresSymbol(t *testing.T) {
	dir := t.TempDir()
	resp := runAuditTrailTool(t, dir, AuditTrailParams{Path: "a.go"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "symbol is required")
}

func TestAuditTrailToolReportsNotARepository(t *testing.T) {
	dir := t.TempDir()
	resp := runAuditTrailTool(t, dir, AuditTrailParams{Path: "a.go", Symbol: "Foo"})
	require.Contains(t, resp.Content, "Not a git repository")
}

func TestAuditTrailToolReportsMetadata(t *testing.T) {
	dir := preCommitRepo(t)
	writeDeadCodeFile(t, dir, "a.go", "package a\n\nfunc Foo() int {\n\treturn 1\n}\n")
	preCommitGit(t, dir, "add", "a.go")
	preCommitGit(t, dir, "commit", "-m", "add Foo")

	resp := runAuditTrailTool(t, dir, AuditTrailParams{Path: "a.go", Symbol: "Foo"})
	var meta AuditTrailResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.Commits)
}

func TestAuditTrailToolRespectsLimit(t *testing.T) {
	dir := preCommitRepo(t)
	writeDeadCodeFile(t, dir, "a.go", "package a\n\nfunc Foo() int {\n\treturn 1\n}\n")
	preCommitGit(t, dir, "add", "a.go")
	preCommitGit(t, dir, "commit", "-m", "add Foo")
	writeDeadCodeFile(t, dir, "a.go", "package a\n\nfunc Foo() int {\n\treturn 2\n}\n")
	preCommitGit(t, dir, "add", "a.go")
	preCommitGit(t, dir, "commit", "-m", "change Foo")

	resp := runAuditTrailTool(t, dir, AuditTrailParams{Path: "a.go", Symbol: "Foo", Limit: 1})
	require.Contains(t, resp.Content, "1 commit(s)")
}
