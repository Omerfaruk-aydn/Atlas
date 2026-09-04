package codeintel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeMetricFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestIndexMetricsFindsACounter(t *testing.T) {
	dir := t.TempDir()
	writeMetricFile(t, dir, "a.go", `package a

import "github.com/prometheus/client_golang/prometheus"

var requestsTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Namespace: "app",
	Subsystem: "http",
	Name:      "requests_total",
	Help:      "Total HTTP requests.",
})
`)

	got, err := IndexMetrics(dir, false)
	require.NoError(t, err)
	require.Len(t, got.Metrics, 1)
	m := got.Metrics[0]
	require.Equal(t, "counter", m.Type)
	require.Equal(t, "app_http_requests_total", m.Name)
	require.Equal(t, "Total HTTP requests.", m.Help)
}

func TestIndexMetricsFindsAGaugeVecWithLabels(t *testing.T) {
	dir := t.TempDir()
	writeMetricFile(t, dir, "a.go", `package a

import "github.com/prometheus/client_golang/prometheus"

var queueDepth = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Name: "queue_depth",
	Help: "Current queue depth.",
}, []string{"queue", "priority"})
`)

	got, err := IndexMetrics(dir, false)
	require.NoError(t, err)
	require.Len(t, got.Metrics, 1)
	m := got.Metrics[0]
	require.Equal(t, "gauge", m.Type)
	require.Equal(t, "queue_depth", m.Name)
	require.Equal(t, []string{"queue", "priority"}, m.Labels)
}

func TestIndexMetricsHandlesPromauto(t *testing.T) {
	dir := t.TempDir()
	writeMetricFile(t, dir, "a.go", `package a

import "github.com/prometheus/client_golang/prometheus/promauto"

var latency = promauto.NewHistogram(prometheus.HistogramOpts{
	Name: "request_latency_seconds",
	Help: "Request latency.",
})
`)

	got, err := IndexMetrics(dir, false)
	require.NoError(t, err)
	require.Len(t, got.Metrics, 1)
	require.Equal(t, "histogram", got.Metrics[0].Type)
}

func TestIndexMetricsHandlesNonLiteralOpts(t *testing.T) {
	dir := t.TempDir()
	writeMetricFile(t, dir, "a.go", `package a

import "github.com/prometheus/client_golang/prometheus"

func build(opts prometheus.CounterOpts) {
	prometheus.NewCounter(opts)
}
`)

	got, err := IndexMetrics(dir, false)
	require.NoError(t, err)
	require.Len(t, got.Metrics, 1)
	require.Equal(t, "(unknown)", got.Metrics[0].Name)
	require.Empty(t, got.Metrics[0].Help)
}

func TestIndexMetricsFindsMultipleMetrics(t *testing.T) {
	dir := t.TempDir()
	writeMetricFile(t, dir, "a.go", `package a

import "github.com/prometheus/client_golang/prometheus"

var a = prometheus.NewCounter(prometheus.CounterOpts{Name: "a"})
var b = prometheus.NewGauge(prometheus.GaugeOpts{Name: "b"})
`)

	got, err := IndexMetrics(dir, false)
	require.NoError(t, err)
	require.Len(t, got.Metrics, 2)
}

func TestIndexMetricsIgnoresUnrelatedCalls(t *testing.T) {
	dir := t.TempDir()
	writeMetricFile(t, dir, "a.go", `package a

func New() {}

func f() {
	New()
}
`)

	got, err := IndexMetrics(dir, false)
	require.NoError(t, err)
	require.Empty(t, got.Metrics)
}

func TestIndexMetricsSkipsTestFilesByDefault(t *testing.T) {
	dir := t.TempDir()
	writeMetricFile(t, dir, "a_test.go", `package a

import "github.com/prometheus/client_golang/prometheus"

var a = prometheus.NewCounter(prometheus.CounterOpts{Name: "a"})
`)

	got, err := IndexMetrics(dir, false)
	require.NoError(t, err)
	require.Equal(t, 0, got.FilesScanned)
}

func TestIndexMetricsIncludesTestFilesWhenAsked(t *testing.T) {
	dir := t.TempDir()
	writeMetricFile(t, dir, "a_test.go", `package a

import "github.com/prometheus/client_golang/prometheus"

var a = prometheus.NewCounter(prometheus.CounterOpts{Name: "a"})
`)

	got, err := IndexMetrics(dir, true)
	require.NoError(t, err)
	require.Len(t, got.Metrics, 1)
}

func TestIndexMetricsReportsNoMetrics(t *testing.T) {
	dir := t.TempDir()
	writeMetricFile(t, dir, "a.go", "package a\n\nfunc f() {}\n")

	got, err := IndexMetrics(dir, false)
	require.NoError(t, err)
	require.Empty(t, got.Metrics)
	require.Equal(t, 1, got.FilesScanned)
}
