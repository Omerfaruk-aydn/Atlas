package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runSecurityScanTool(t *testing.T, workingDir string, params SecurityScanParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewSecurityScanTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  SecurityScanToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

const securityFixture = `package a

func connect() {
	password := "hunter2-real-secret"
	_ = password
}
`

func TestSecurityScanToolReportsFindings(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", securityFixture)

	resp := runSecurityScanTool(t, dir, SecurityScanParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "1 finding(s)")
	require.Contains(t, resp.Content, "hardcoded-credential")
	require.Contains(t, resp.Content, "not type-checked")
}

func TestSecurityScanToolReportsCleanCode(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", "package a\n\nfunc f() {}\n")

	resp := runSecurityScanTool(t, dir, SecurityScanParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "No security smells found")
}

func TestSecurityScanToolReportsNoFiles(t *testing.T) {
	dir := t.TempDir()
	resp := runSecurityScanTool(t, dir, SecurityScanParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "No Go files found")
}

func TestSecurityScanToolSkipsTestFilesByDefault(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a_test.go", securityFixture)

	resp := runSecurityScanTool(t, dir, SecurityScanParams{})
	require.Contains(t, resp.Content, "No Go files found")
}

func TestSecurityScanToolIncludesTestFilesWhenAsked(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a_test.go", securityFixture)

	include := true
	resp := runSecurityScanTool(t, dir, SecurityScanParams{IncludeTests: &include})
	require.Contains(t, resp.Content, "1 finding(s)")
}

func TestSecurityScanToolReportsMetadata(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", securityFixture)

	resp := runSecurityScanTool(t, dir, SecurityScanParams{})
	var meta SecurityScanResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.Total)
	require.Equal(t, 1, meta.ByKind["hardcoded-credential"])
	require.Equal(t, 1, meta.FilesScanned)
}

func TestSecurityScanToolResolvesARelativePath(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "sub/a.go", securityFixture)

	resp := runSecurityScanTool(t, dir, SecurityScanParams{Path: "sub"})
	require.Contains(t, resp.Content, "1 finding(s)")
}
