package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runCodeMetricsTool(t *testing.T, workingDir string, params CodeMetricsParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewCodeMetricsTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  CodeMetricsToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

const metricsFixture = `package a

func simple() int { return 1 }

func gnarly(n int) int {
	if n > 0 {
		for range 3 {
			if n > 5 && n < 100 {
				n++
			}
		}
	}
	return n
}
`

func TestCodeMetricsToolListsTheMostComplexFirst(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", metricsFixture)

	resp := runCodeMetricsTool(t, dir, CodeMetricsParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "gnarly")
	// gnarly must appear before simple in the listing.
	require.Less(t,
		indexOf(resp.Content, "gnarly"),
		indexOf(resp.Content, "simple"))
}

func TestCodeMetricsToolShowsEveryMeasurement(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", metricsFixture)

	resp := runCodeMetricsTool(t, dir, CodeMetricsParams{})
	require.Contains(t, resp.Content, "cplx")
	require.Contains(t, resp.Content, "lines")
	require.Contains(t, resp.Content, "nest")
	require.Contains(t, resp.Content, "sig")
}

func TestCodeMetricsToolFiltersByMinComplexity(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", metricsFixture)

	resp := runCodeMetricsTool(t, dir, CodeMetricsParams{MinComplexity: 3})
	require.Contains(t, resp.Content, "gnarly")
	require.Contains(t, resp.Content, "complexity >= 3")
	require.NotContains(t, resp.Content, "simple")
}

func TestCodeMetricsToolHonoursTop(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", metricsFixture)

	resp := runCodeMetricsTool(t, dir, CodeMetricsParams{Top: 1})
	require.Contains(t, resp.Content, "gnarly")
	require.Contains(t, resp.Content, "and 1 more above the cut")
}

func TestCodeMetricsToolClampsAnAbsurdTop(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", metricsFixture)

	resp := runCodeMetricsTool(t, dir, CodeMetricsParams{Top: 99999})
	var meta CodeMetricsResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 2, meta.Reported)
}

func TestCodeMetricsToolLabelsMethodsWithTheirReceiver(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", `package a

type T struct{}

func (t T) Method() {}
`)

	resp := runCodeMetricsTool(t, dir, CodeMetricsParams{})
	require.Contains(t, resp.Content, "(T).Method")
}

// A score without the caveat invites mechanical splitting of switches
// that are fine as they are.
func TestCodeMetricsToolWarnsThatAScoreIsNotAVerdict(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", metricsFixture)

	resp := runCodeMetricsTool(t, dir, CodeMetricsParams{})
	require.Contains(t, resp.Content, "not a verdict")
	require.Contains(t, resp.Content, "switch")
}

func TestCodeMetricsToolSummarisesLargestFiles(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", metricsFixture)
	writeDeadCodeFile(t, dir, "b.go", `package a

func other() {}
`)

	resp := runCodeMetricsTool(t, dir, CodeMetricsParams{})
	require.Contains(t, resp.Content, "Largest files:")
	require.Contains(t, resp.Content, "a.go")
}

func TestCodeMetricsToolSaysSoWhenNothingCrossesTheThreshold(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", `package a

func simple() int { return 1 }
`)

	resp := runCodeMetricsTool(t, dir, CodeMetricsParams{MinComplexity: 50})
	require.Contains(t, resp.Content, "0 function(s) at complexity >= 50")
}

func TestCodeMetricsToolReportsNoGoFiles(t *testing.T) {
	resp := runCodeMetricsTool(t, t.TempDir(), CodeMetricsParams{})
	require.Contains(t, resp.Content, "No Go files found")
}

func TestCodeMetricsToolErrorsOnAMissingPath(t *testing.T) {
	resp := runCodeMetricsTool(t, t.TempDir(), CodeMetricsParams{Path: "nope"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "cannot scan")
}

func TestCodeMetricsToolHonoursIncludeTests(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", `package a

func prod() {}
`)
	writeDeadCodeFile(t, dir, "a_test.go", `package a

import "testing"

func TestProd(t *testing.T) {}
`)

	resp := runCodeMetricsTool(t, dir, CodeMetricsParams{})
	require.NotContains(t, resp.Content, "TestProd")

	yes := true
	resp = runCodeMetricsTool(t, dir, CodeMetricsParams{IncludeTests: &yes})
	require.Contains(t, resp.Content, "TestProd")
}

func TestCodeMetricsToolReportsMetadata(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", metricsFixture)

	resp := runCodeMetricsTool(t, dir, CodeMetricsParams{})
	var meta CodeMetricsResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 2, meta.Functions)
	require.Equal(t, 1, meta.FilesScanned)
	require.Positive(t, meta.MaxComplexity)
	require.Positive(t, meta.TotalLines)
}

// indexOf is strings.Index, named for what the assertions above mean.
func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
