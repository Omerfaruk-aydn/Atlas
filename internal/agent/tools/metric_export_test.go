package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runMetricExportTool(t *testing.T, workingDir string, params MetricExportParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewMetricExportTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  MetricExportToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

const metricExportFixture = `package a

import "github.com/prometheus/client_golang/prometheus"

var requestsTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "requests_total",
	Help: "Total requests.",
})
`

func TestMetricExportToolReportsMetrics(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", metricExportFixture)

	resp := runMetricExportTool(t, dir, MetricExportParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "1 metric(s)")
	require.Contains(t, resp.Content, "requests_total (counter)")
	require.Contains(t, resp.Content, "Total requests.")
}

func TestMetricExportToolReportsNoFiles(t *testing.T) {
	dir := t.TempDir()
	resp := runMetricExportTool(t, dir, MetricExportParams{})
	require.Contains(t, resp.Content, "No Go files found")
}

func TestMetricExportToolReportsNoMetrics(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", "package a\n\nfunc f() {}\n")

	resp := runMetricExportTool(t, dir, MetricExportParams{})
	require.Contains(t, resp.Content, "No Prometheus metrics found")
}

func TestMetricExportToolSkipsTestFilesByDefault(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a_test.go", metricExportFixture)

	resp := runMetricExportTool(t, dir, MetricExportParams{})
	require.Contains(t, resp.Content, "No Go files found")
}

func TestMetricExportToolIncludesTestFilesWhenAsked(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a_test.go", metricExportFixture)

	include := true
	resp := runMetricExportTool(t, dir, MetricExportParams{IncludeTests: &include})
	require.Contains(t, resp.Content, "1 metric(s)")
}

func TestMetricExportToolReportsMetadata(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", metricExportFixture)

	resp := runMetricExportTool(t, dir, MetricExportParams{})
	var meta MetricExportResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.Total)
	require.Equal(t, 1, meta.ByType["counter"])
	require.Equal(t, 1, meta.FilesScanned)
}

func TestMetricExportToolResolvesARelativePath(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "sub/a.go", metricExportFixture)

	resp := runMetricExportTool(t, dir, MetricExportParams{Path: "sub"})
	require.Contains(t, resp.Content, "1 metric(s)")
}
