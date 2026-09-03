package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runImpactTool(t *testing.T, workingDir string, params ImpactAnalysisParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewImpactAnalysisTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  ImpactAnalysisToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

const impactFixture = `package a

func target() int { return 1 }

func mid() int { return target() }

func top() int { return mid() }
`

func TestImpactToolGroupsCallersByDistance(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", impactFixture)

	resp := runImpactTool(t, dir, ImpactAnalysisParams{Symbol: "target"})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "direct callers:")
	require.Contains(t, resp.Content, "mid")
	require.Contains(t, resp.Content, "2 hops away:")
	require.Contains(t, resp.Content, "top")
	// A multi-hop entry has to show the path, not just the name.
	require.Contains(t, resp.Content, "-> mid")
}

func TestImpactToolRequiresASymbol(t *testing.T) {
	resp := runImpactTool(t, t.TempDir(), ImpactAnalysisParams{})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "symbol is required")
}

// Passing a qualified name is the obvious mistake, and silently matching
// nothing would read as "this has no callers".
func TestImpactToolStripsAQualifiedSymbolName(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", impactFixture)

	resp := runImpactTool(t, dir, ImpactAnalysisParams{Symbol: "pkg.target"})
	require.Contains(t, resp.Content, "direct callers:")
	require.Contains(t, resp.Content, "mid")
}

func TestImpactToolReportsAnUnknownSymbol(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", impactFixture)

	resp := runImpactTool(t, dir, ImpactAnalysisParams{Symbol: "nope"})
	require.Contains(t, resp.Content, "No function or method named")
}

// "Nothing calls it" is the result most likely to license a deletion, so
// the blind spots have to travel with it.
func TestImpactToolQualifiesAnEmptyResult(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", `package a

func lonely() int { return 1 }
`)

	resp := runImpactTool(t, dir, ImpactAnalysisParams{Symbol: "lonely"})
	require.Contains(t, resp.Content, "Nothing in this tree calls it")
	require.Contains(t, resp.Content, "interface")
	require.Contains(t, resp.Content, "reflection")
}

func TestImpactToolWarnsAboutAmbiguousNames(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", `package a

type A struct{}
type B struct{}

func (a A) Close() error { return nil }
func (b B) Close() error { return nil }

func shut(a A) error { return a.Close() }
`)

	resp := runImpactTool(t, dir, ImpactAnalysisParams{Symbol: "Close"})
	require.Contains(t, resp.Content, "share this name")
	require.Contains(t, resp.Content, "may belong to any of them")
}

// A method has to be distinguishable from a function of the same name.
func TestImpactToolLabelsMethodsWithTheirReceiver(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", `package a

type T struct{}

func (t T) Run() {}

func start(t T) { t.Run() }
`)

	resp := runImpactTool(t, dir, ImpactAnalysisParams{Symbol: "Run"})
	require.Contains(t, resp.Content, "(T).Run")
}

func TestImpactToolHonoursMaxDepth(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", impactFixture)

	resp := runImpactTool(t, dir, ImpactAnalysisParams{Symbol: "target", MaxDepth: 1})
	require.Contains(t, resp.Content, "mid")
	require.NotContains(t, resp.Content, "2 hops away")
	require.Contains(t, resp.Content, "stopped at the depth limit")

	var meta ImpactAnalysisResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.Callers)
	require.True(t, meta.Truncated)
}

func TestImpactToolClampsAnAbsurdDepth(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", impactFixture)

	resp := runImpactTool(t, dir, ImpactAnalysisParams{Symbol: "target", MaxDepth: 9999})
	var meta ImpactAnalysisResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, maxImpactDepth, meta.MaxDepth)
}

func TestImpactToolHonoursIncludeTests(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", `package a

func target() int { return 1 }
`)
	writeDeadCodeFile(t, dir, "a_test.go", `package a

import "testing"

func TestTarget(t *testing.T) { _ = target() }
`)

	resp := runImpactTool(t, dir, ImpactAnalysisParams{Symbol: "target"})
	require.Contains(t, resp.Content, "Nothing in this tree calls it")

	yes := true
	resp = runImpactTool(t, dir, ImpactAnalysisParams{Symbol: "target", IncludeTests: &yes})
	require.Contains(t, resp.Content, "TestTarget")
}

func TestImpactToolStatesThatItErrsWide(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", impactFixture)

	resp := runImpactTool(t, dir, ImpactAnalysisParams{Symbol: "target"})
	require.Contains(t, resp.Content, "errs wide")
	require.Contains(t, resp.Content, "lsp_references")
}

func TestImpactToolReportsMetadata(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", impactFixture)

	resp := runImpactTool(t, dir, ImpactAnalysisParams{Symbol: "target"})
	var meta ImpactAnalysisResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 2, meta.Callers)
	require.Equal(t, 1, meta.FilesScanned)
	require.Equal(t, 0, meta.Ambiguous)
}

func TestImpactToolReportsNoGoFiles(t *testing.T) {
	resp := runImpactTool(t, t.TempDir(), ImpactAnalysisParams{Symbol: "x"})
	require.Contains(t, resp.Content, "No Go files found")
}
