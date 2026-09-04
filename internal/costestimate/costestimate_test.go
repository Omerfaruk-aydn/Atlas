package costestimate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeCostFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestEstimateTerraformKnownInstanceType(t *testing.T) {
	dir := t.TempDir()
	writeCostFile(t, dir, "main.tf", `resource "aws_instance" "web" {
  instance_type = "t3.small"
}
`)

	got, err := Estimate(dir)
	require.NoError(t, err)
	require.Len(t, got.Instances, 1)
	inst := got.Instances[0]
	require.True(t, inst.Known)
	require.Equal(t, "web", inst.Name)
	require.Equal(t, 1, inst.Count)
	require.InDelta(t, 0.0208*730, inst.MonthlyUSD, 0.01)
	require.InDelta(t, inst.MonthlyUSD, got.TotalMonthlyUSD, 0.01)
}

func TestEstimateTerraformRespectsCount(t *testing.T) {
	dir := t.TempDir()
	writeCostFile(t, dir, "main.tf", `resource "aws_instance" "web" {
  instance_type = "t3.small"
  count         = 3
}
`)

	got, err := Estimate(dir)
	require.NoError(t, err)
	require.Equal(t, 3, got.Instances[0].Count)
	require.InDelta(t, 0.0208*730*3, got.Instances[0].MonthlyUSD, 0.01)
}

func TestEstimateTerraformUnknownInstanceType(t *testing.T) {
	dir := t.TempDir()
	writeCostFile(t, dir, "main.tf", `resource "aws_instance" "web" {
  instance_type = "totally.made.up"
}
`)

	got, err := Estimate(dir)
	require.NoError(t, err)
	require.False(t, got.Instances[0].Known)
	require.Zero(t, got.Instances[0].MonthlyUSD)
	require.Equal(t, 0.0, got.TotalMonthlyUSD)
	require.Contains(t, got.UnknownInstanceTypes, "totally.made.up")
}

func TestEstimateTerraformMultipleResources(t *testing.T) {
	dir := t.TempDir()
	writeCostFile(t, dir, "main.tf", `resource "aws_instance" "web" {
  instance_type = "t3.small"
}

resource "aws_instance" "db" {
  instance_type = "m5.large"
}
`)

	got, err := Estimate(dir)
	require.NoError(t, err)
	require.Len(t, got.Instances, 2)
}

func TestEstimateK8sDeploymentWithReplicas(t *testing.T) {
	dir := t.TempDir()
	writeCostFile(t, dir, "deploy.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  replicas: 3
  template:
    spec:
      containers:
        - name: app
          resources:
            requests:
              cpu: "500m"
              memory: "256Mi"
`)

	got, err := Estimate(dir)
	require.NoError(t, err)
	require.Len(t, got.K8sWorkloads, 1)
	w := got.K8sWorkloads[0]
	require.Equal(t, 3, w.Replicas)
	require.InDelta(t, 1.5, w.CPUCores, 0.001)  // 0.5 * 3
	require.InDelta(t, 0.75, w.MemoryGiB, 0.01) // 0.25 GiB * 3
	require.Greater(t, w.MonthlyUSD, 0.0)
}

func TestEstimateK8sSkipsWorkloadsWithNoRequests(t *testing.T) {
	dir := t.TempDir()
	writeCostFile(t, dir, "deploy.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  replicas: 2
  template:
    spec:
      containers:
        - name: app
`)

	got, err := Estimate(dir)
	require.NoError(t, err)
	require.Empty(t, got.K8sWorkloads)
}

func TestEstimateK8sDefaultsToOneReplica(t *testing.T) {
	dir := t.TempDir()
	writeCostFile(t, dir, "deploy.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  template:
    spec:
      containers:
        - name: app
          resources:
            requests:
              cpu: "1"
              memory: "1Gi"
`)

	got, err := Estimate(dir)
	require.NoError(t, err)
	require.Equal(t, 1, got.K8sWorkloads[0].Replicas)
	require.InDelta(t, 1.0, got.K8sWorkloads[0].CPUCores, 0.001)
	require.InDelta(t, 1.0, got.K8sWorkloads[0].MemoryGiB, 0.001)
}

func TestEstimateIgnoresNonWorkloadKinds(t *testing.T) {
	dir := t.TempDir()
	writeCostFile(t, dir, "cm.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: config\n")

	got, err := Estimate(dir)
	require.NoError(t, err)
	require.Empty(t, got.K8sWorkloads)
}

func TestEstimateCombinesBothSources(t *testing.T) {
	dir := t.TempDir()
	writeCostFile(t, dir, "main.tf", `resource "aws_instance" "web" {
  instance_type = "t3.small"
}
`)
	writeCostFile(t, dir, "deploy.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  replicas: 1
  template:
    spec:
      containers:
        - name: app
          resources:
            requests:
              cpu: "1"
              memory: "1Gi"
`)

	got, err := Estimate(dir)
	require.NoError(t, err)
	require.Len(t, got.Instances, 1)
	require.Len(t, got.K8sWorkloads, 1)
	require.InDelta(t, got.Instances[0].MonthlyUSD+got.K8sWorkloads[0].MonthlyUSD, got.TotalMonthlyUSD, 0.01)
}

func TestParseCPUHandlesMillicoresAndWholeCores(t *testing.T) {
	require.InDelta(t, 0.5, parseCPU("500m"), 0.001)
	require.InDelta(t, 2.0, parseCPU("2"), 0.001)
}

func TestParseMemoryGiBHandlesUnits(t *testing.T) {
	require.InDelta(t, 1.0, parseMemoryGiB("1Gi"), 0.001)
	require.InDelta(t, 0.25, parseMemoryGiB("256Mi"), 0.001)
}
