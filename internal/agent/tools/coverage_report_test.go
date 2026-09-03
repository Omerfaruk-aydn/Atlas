package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runCoverageTool(t *testing.T, workingDir string, params CoverageReportParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewCoverageReportTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  CoverageReportToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

// partiallyCovered has one function the test exercises and one it never
// touches, so the report has something real to find.
var partiallyCovered = map[string]string{
	"a.go": `package a

func Covered(x int) int {
	if x > 0 {
		return x
	}
	return -x
}

func NeverCalled() string {
	return "nobody runs this"
}
`,
	"a_test.go": `package a

import "testing"

func TestCovered(t *testing.T) {
	if Covered(1) != 1 {
		t.Fatal("wrong")
	}
}
`,
}

func TestCoverageToolReportsThePercentage(t *testing.T) {
	dir := goModule(t, partiallyCovered)

	resp := runCoverageTool(t, dir, CoverageReportParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "coverage:")
	require.Contains(t, resp.Content, "% of statements")

	var meta CoverageReportResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Greater(t, meta.Percent, 0.0)
	require.Less(t, meta.Percent, 100.0)
	require.True(t, meta.TestsOK)
}

// The uncovered ranges are the actual answer; a percentage alone says
// there is a problem but not where.
func TestCoverageToolNamesTheUncoveredLines(t *testing.T) {
	dir := goModule(t, partiallyCovered)

	resp := runCoverageTool(t, dir, CoverageReportParams{})
	require.Contains(t, resp.Content, "uncovered lines:")
}

func TestCoverageToolCanOmitUncoveredRanges(t *testing.T) {
	dir := goModule(t, partiallyCovered)

	no := false
	resp := runCoverageTool(t, dir, CoverageReportParams{ShowUncovered: &no})
	require.NotContains(t, resp.Content, "uncovered lines:")
	require.Contains(t, resp.Content, "coverage:")
}

// Both caveats are routinely dropped, and a percentage quoted without
// them becomes a target rather than a diagnostic.
func TestCoverageToolStatesWhatTheNumberDoesNotMean(t *testing.T) {
	dir := goModule(t, partiallyCovered)

	resp := runCoverageTool(t, dir, CoverageReportParams{})
	require.Contains(t, resp.Content, "counts statements, not lines")
	require.Contains(t, resp.Content, "executed, not verified")
}

func TestCoverageToolChecksAThreshold(t *testing.T) {
	dir := goModule(t, partiallyCovered)

	resp := runCoverageTool(t, dir, CoverageReportParams{MinPercent: 99})
	require.Contains(t, resp.Content, "BELOW the 99.0% threshold")

	var meta CoverageReportResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.NotNil(t, meta.MetMinimum)
	require.False(t, *meta.MetMinimum)

	resp = runCoverageTool(t, dir, CoverageReportParams{MinPercent: 1})
	require.Contains(t, resp.Content, "Meets the 1.0% threshold")
}

func TestCoverageToolRejectsAnImpossibleThreshold(t *testing.T) {
	resp := runCoverageTool(t, t.TempDir(), CoverageReportParams{MinPercent: 150})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "between 0 and 100")
}

func TestCoverageToolRejectsABadTimeout(t *testing.T) {
	resp := runCoverageTool(t, t.TempDir(), CoverageReportParams{Timeout: "whenever"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "not a positive duration")
}

// Coverage measured against a suite that is not passing means less than
// the number suggests, and a reader seeing only the percentage will not
// know that.
func TestCoverageToolWarnsWhenTestsFailed(t *testing.T) {
	dir := goModule(t, map[string]string{
		"a.go": `package a

func Add(x, y int) int { return x + y }
`,
		"a_test.go": `package a

import "testing"

func TestAdd(t *testing.T) {
	_ = Add(1, 2)
	t.Error("deliberate failure")
}
`,
	})

	resp := runCoverageTool(t, dir, CoverageReportParams{})
	require.Contains(t, resp.Content, "WARNING")
	require.Contains(t, resp.Content, "not passing")

	var meta CoverageReportResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.False(t, meta.TestsOK)
}

func TestCoverageToolExplainsWhenThereIsNothingToMeasure(t *testing.T) {
	dir := goModule(t, map[string]string{
		"a.go": `package a

func Nothing() {}
`,
	})

	resp := runCoverageTool(t, dir, CoverageReportParams{})
	require.Contains(t, resp.Content, "No coverage data")
	require.Contains(t, resp.Content, "nothing to measure")
}

func TestCoverageToolReportsFullCoverage(t *testing.T) {
	dir := goModule(t, map[string]string{
		"a.go": `package a

func Add(x, y int) int { return x + y }
`,
		"a_test.go": `package a

import "testing"

func TestAdd(t *testing.T) {
	if Add(1, 2) != 3 {
		t.Fatal("wrong")
	}
}
`,
	})

	resp := runCoverageTool(t, dir, CoverageReportParams{})
	require.Contains(t, resp.Content, "100.0%")

	var meta CoverageReportResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.InDelta(t, 100.0, meta.Percent, 0.01)
}
