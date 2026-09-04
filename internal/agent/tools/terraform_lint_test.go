package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runTerraformLintTool(t *testing.T, workingDir string, params TerraformLintParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewTerraformLintTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  TerraformLintToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

const terraformFixture = `resource "aws_security_group" "web" {
  ingress {
    cidr_blocks = ["0.0.0.0/0"]
  }
}
`

func TestTerraformLintToolReportsFindings(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "main.tf", terraformFixture)

	resp := runTerraformLintTool(t, dir, TerraformLintParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "1 issue(s)")
	require.Contains(t, resp.Content, "open-ingress")
	require.Contains(t, resp.Content, "aws_security_group.web")
}

func TestTerraformLintToolReportsClean(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "main.tf", "resource \"aws_s3_bucket_acl\" \"data\" {\n  acl = \"private\"\n}\n")

	resp := runTerraformLintTool(t, dir, TerraformLintParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "No issues found")
}

func TestTerraformLintToolReportsNoFiles(t *testing.T) {
	dir := t.TempDir()
	resp := runTerraformLintTool(t, dir, TerraformLintParams{})
	require.Contains(t, resp.Content, "No .tf files found")
}

func TestTerraformLintToolReportsMetadata(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "main.tf", terraformFixture)

	resp := runTerraformLintTool(t, dir, TerraformLintParams{})
	var meta TerraformLintResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.Total)
	require.Equal(t, 1, meta.ByKind["open-ingress"])
	require.Equal(t, 1, meta.FilesScanned)
}

func TestTerraformLintToolResolvesARelativePath(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "sub/main.tf", terraformFixture)

	resp := runTerraformLintTool(t, dir, TerraformLintParams{Path: "sub"})
	require.Contains(t, resp.Content, "1 issue(s)")
}
