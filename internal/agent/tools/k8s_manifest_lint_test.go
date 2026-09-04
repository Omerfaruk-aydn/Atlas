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

func runK8sManifestLintTool(t *testing.T, workingDir string, params K8sManifestLintParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewK8sManifestLintTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  K8sManifestLintToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

const k8sFixture = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  template:
    spec:
      containers:
        - name: app
          image: myapp
`

func TestK8sManifestLintToolReportsFindings(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "deploy.yaml"), []byte(k8sFixture), 0o644))

	resp := runK8sManifestLintTool(t, dir, K8sManifestLintParams{Path: "deploy.yaml"})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "Deployment/web")
	require.Contains(t, resp.Content, "missing-resource-limits")
	require.Contains(t, resp.Content, "latest-tag")
	require.Contains(t, resp.Content, "missing-probes")
}

func TestK8sManifestLintToolReportsClean(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pod.yaml"), []byte(`apiVersion: v1
kind: Pod
metadata:
  name: debug
spec:
  containers:
    - name: app
      image: myapp:1.0
      resources:
        limits:
          cpu: "1"
`), 0o644))

	resp := runK8sManifestLintTool(t, dir, K8sManifestLintParams{Path: "pod.yaml"})
	require.Contains(t, resp.Content, "No issues found")
}

func TestK8sManifestLintToolRequiresPath(t *testing.T) {
	dir := t.TempDir()
	resp := runK8sManifestLintTool(t, dir, K8sManifestLintParams{})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "path is required")
}

func TestK8sManifestLintToolReportsMissingFile(t *testing.T) {
	dir := t.TempDir()
	resp := runK8sManifestLintTool(t, dir, K8sManifestLintParams{Path: "nope.yaml"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "no manifest found")
}

func TestK8sManifestLintToolReportsNoWorkloadObjects(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cm.yaml"), []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: config\n"), 0o644))

	resp := runK8sManifestLintTool(t, dir, K8sManifestLintParams{Path: "cm.yaml"})
	require.Contains(t, resp.Content, "nothing to check")
}

func TestK8sManifestLintToolReportsMetadata(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "deploy.yaml"), []byte(k8sFixture), 0o644))

	resp := runK8sManifestLintTool(t, dir, K8sManifestLintParams{Path: "deploy.yaml"})
	var meta K8sManifestLintResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.ObjectsScanned)
	require.Equal(t, 4, meta.Total)
}
