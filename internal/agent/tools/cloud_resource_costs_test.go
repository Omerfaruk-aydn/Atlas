package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runCloudResourceCostsTool(t *testing.T, workingDir string, params CloudResourceCostsParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewCloudResourceCostsTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  CloudResourceCostsToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

const costTerraformFixture = `resource "aws_instance" "web" {
  instance_type = "t3.small"
}
`

func TestCloudResourceCostsToolReportsEstimate(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "main.tf", costTerraformFixture)

	resp := runCloudResourceCostsTool(t, dir, CloudResourceCostsParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "Estimated ~$")
	require.Contains(t, resp.Content, "web")
	require.Contains(t, resp.Content, "t3.small")
}

func TestCloudResourceCostsToolReportsNoFiles(t *testing.T) {
	dir := t.TempDir()
	resp := runCloudResourceCostsTool(t, dir, CloudResourceCostsParams{})
	require.Contains(t, resp.Content, "No .tf or .yaml/.yml files found")
}

func TestCloudResourceCostsToolReportsNoCostableResources(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "cm.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: config\n")

	resp := runCloudResourceCostsTool(t, dir, CloudResourceCostsParams{})
	require.Contains(t, resp.Content, "No costable resources found")
}

func TestCloudResourceCostsToolReportsUnknownInstanceType(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "main.tf", "resource \"aws_instance\" \"web\" {\n  instance_type = \"totally.made.up\"\n}\n")

	resp := runCloudResourceCostsTool(t, dir, CloudResourceCostsParams{})
	require.Contains(t, resp.Content, "Unknown instance types")
	require.Contains(t, resp.Content, "totally.made.up")
}

func TestCloudResourceCostsToolReportsMetadata(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "main.tf", costTerraformFixture)

	resp := runCloudResourceCostsTool(t, dir, CloudResourceCostsParams{})
	var meta CloudResourceCostsResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.Instances)
	require.Equal(t, 1, meta.FilesScanned)
	require.Greater(t, meta.TotalMonthlyUSD, 0.0)
}

func TestCloudResourceCostsToolResolvesARelativePath(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "sub/main.tf", costTerraformFixture)

	resp := runCloudResourceCostsTool(t, dir, CloudResourceCostsParams{Path: "sub"})
	require.Contains(t, resp.Content, "Estimated ~$")
}
