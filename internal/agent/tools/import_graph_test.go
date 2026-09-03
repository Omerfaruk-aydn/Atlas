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

func runImportGraphTool(t *testing.T, workingDir string, params ImportGraphParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewImportGraphTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  ImportGraphToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

// graphModule writes a go.mod plus files so import paths resolve.
func graphModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/m\n\ngo 1.26\n"), 0o644))
	for name, src := range files {
		writeDeadCodeFile(t, dir, name, src)
	}
	return dir
}

var acyclicFixture = map[string]string{
	"a/a.go": `package a

import "example.com/m/b"

var _ = b.X
`,
	"b/b.go": `package b

var X = 1
`,
	"c/c.go": `package c

import "example.com/m/b"

var _ = b.X
`,
}

func TestImportGraphToolSummarisesTheModule(t *testing.T) {
	dir := graphModule(t, acyclicFixture)

	resp := runImportGraphTool(t, dir, ImportGraphParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "module example.com/m")
	require.Contains(t, resp.Content, "3 package(s)")
	require.Contains(t, resp.Content, "No import cycles")
	require.Contains(t, resp.Content, "Most depended on")
	require.Contains(t, resp.Content, "example.com/m/b")
}

// A cycle is a build error, so it must be at the top of the report and
// spelled out as a loop rather than a set.
func TestImportGraphToolReportsCyclesAsALoop(t *testing.T) {
	dir := graphModule(t, map[string]string{
		"a/a.go": `package a
import "example.com/m/b"
var _ = b.X
`,
		"b/b.go": `package b
import "example.com/m/a"
var X = a.Y
`,
	})

	resp := runImportGraphTool(t, dir, ImportGraphParams{})
	require.Contains(t, resp.Content, "1 import cycle(s)")
	require.Contains(t, resp.Content, "->")
	// The loop must close: the last element points back at the first.
	require.Regexp(t, `example\.com/m/[ab] -> example\.com/m/[ab] -> example\.com/m/[ab]`, resp.Content)
}

func TestImportGraphToolDetailsAFocusedPackage(t *testing.T) {
	dir := graphModule(t, acyclicFixture)

	resp := runImportGraphTool(t, dir, ImportGraphParams{Focus: "example.com/m/b"})
	require.Contains(t, resp.Content, "package example.com/m/b")
	require.Contains(t, resp.Content, "imported by")
	require.Contains(t, resp.Content, "<- example.com/m/a")
	require.Contains(t, resp.Content, "<- example.com/m/c")
	require.Contains(t, resp.Content, "this is a leaf")
}

// The caller usually knows the repository-relative directory, not the
// module prefix, so a bare suffix has to resolve.
func TestImportGraphToolAcceptsADirectoryAsFocus(t *testing.T) {
	dir := graphModule(t, acyclicFixture)

	resp := runImportGraphTool(t, dir, ImportGraphParams{Focus: "b"})
	require.Contains(t, resp.Content, "package example.com/m/b")
}

func TestImportGraphToolReportsAnUnknownFocus(t *testing.T) {
	dir := graphModule(t, acyclicFixture)

	resp := runImportGraphTool(t, dir, ImportGraphParams{Focus: "nope"})
	require.Contains(t, resp.Content, "No package matching")
}

func TestImportGraphToolShowsExternalImportsSeparately(t *testing.T) {
	dir := graphModule(t, map[string]string{
		"a/a.go": `package a

import (
	"fmt"
	"github.com/other/dep"
)

var _ = fmt.Sprint(dep.X)
`,
	})

	resp := runImportGraphTool(t, dir, ImportGraphParams{Focus: "a"})
	require.Contains(t, resp.Content, "external imports")
	require.Contains(t, resp.Content, "github.com/other/dep")
	require.Contains(t, resp.Content, "this is a leaf")
}

func TestImportGraphToolFlagsAFocusedPackageInACycle(t *testing.T) {
	dir := graphModule(t, map[string]string{
		"a/a.go": `package a
import "example.com/m/b"
var _ = b.X
`,
		"b/b.go": `package b
import "example.com/m/a"
var X = a.Y
`,
	})

	resp := runImportGraphTool(t, dir, ImportGraphParams{Focus: "a"})
	require.Contains(t, resp.Content, "In an import cycle")
}

func TestImportGraphToolReportsNoPackages(t *testing.T) {
	resp := runImportGraphTool(t, t.TempDir(), ImportGraphParams{})
	require.Contains(t, resp.Content, "No Go packages found")
}

func TestImportGraphToolErrorsOnAMissingPath(t *testing.T) {
	resp := runImportGraphTool(t, t.TempDir(), ImportGraphParams{Path: "nope"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "cannot scan")
}

// The blind spot has to be stated where the answer is read, not only in
// the tool description.
func TestImportGraphToolStatesItsBlindSpot(t *testing.T) {
	dir := graphModule(t, acyclicFixture)

	resp := runImportGraphTool(t, dir, ImportGraphParams{})
	require.Contains(t, resp.Content, "Only static imports are read")
}

func TestImportGraphToolReportsMetadata(t *testing.T) {
	dir := graphModule(t, acyclicFixture)

	resp := runImportGraphTool(t, dir, ImportGraphParams{})
	var meta ImportGraphResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 3, meta.Packages)
	require.Equal(t, 0, meta.Cycles)
	require.Equal(t, "example.com/m", meta.Module)
}
