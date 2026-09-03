package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runAPISurfaceTool(t *testing.T, workingDir string, params APISurfaceParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewAPISurfaceTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  APISurfaceToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

const apiFixture = `package client

// Client talks to the server.
type Client struct {
	// Timeout bounds a request.
	Timeout int
	secret  string
}

// Do sends a request.
func (c *Client) Do(path string) error { return nil }

// New creates a Client.
func New() *Client { return nil }

// Old is the previous constructor.
//
// Deprecated: use New instead.
func Old() *Client { return nil }

func Bare() {}

func internalHelper() {}
`

func TestAPISurfaceToolListsTheExportedSurface(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "client/a.go", apiFixture)

	resp := runAPISurfaceTool(t, dir, APISurfaceParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "package client")
	require.Contains(t, resp.Content, "func New() *Client")
	require.Contains(t, resp.Content, "func (*Client) Do(string) error")
	require.Contains(t, resp.Content, "Timeout int")
	require.NotContains(t, resp.Content, "internalHelper")
	require.NotContains(t, resp.Content, "secret")
}

func TestAPISurfaceToolMarksDeprecatedSymbols(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "client/a.go", apiFixture)

	resp := runAPISurfaceTool(t, dir, APISurfaceParams{})
	require.Contains(t, resp.Content, "[DEPRECATED]")

	var meta APISurfaceResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.Deprecated)
}

func TestAPISurfaceToolShowsDocSummaries(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "client/a.go", apiFixture)

	resp := runAPISurfaceTool(t, dir, APISurfaceParams{})
	require.Contains(t, resp.Content, "// New creates a Client.")
}

func TestAPISurfaceToolCanListOnlyUndocumented(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "client/a.go", apiFixture)

	yes := true
	resp := runAPISurfaceTool(t, dir, APISurfaceParams{UndocumentedOnly: &yes})
	require.Contains(t, resp.Content, "Bare")
	require.NotContains(t, resp.Content, "func New()")
}

func TestAPISurfaceToolSaysWhenEverythingIsDocumented(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", `package a

// Documented is documented.
func Documented() {}
`)

	yes := true
	resp := runAPISurfaceTool(t, dir, APISurfaceParams{UndocumentedOnly: &yes})
	require.Contains(t, resp.Content, "is documented")
}

// The caller may know the package name or its directory; both must work.
func TestAPISurfaceToolFiltersByPackageNameOrDirectory(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "one/a.go", "package one\n\nfunc FromOne() {}\n")
	writeDeadCodeFile(t, dir, "two/b.go", "package two\n\nfunc FromTwo() {}\n")

	byName := runAPISurfaceTool(t, dir, APISurfaceParams{Package: "one"})
	require.Contains(t, byName.Content, "FromOne")
	require.NotContains(t, byName.Content, "FromTwo")

	byDir := runAPISurfaceTool(t, dir, APISurfaceParams{Package: "two"})
	require.Contains(t, byDir.Content, "FromTwo")
	require.NotContains(t, byDir.Content, "FromOne")
}

func TestAPISurfaceToolReportsAnUnknownPackage(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", "package a\n\nfunc Exported() {}\n")

	resp := runAPISurfaceTool(t, dir, APISurfaceParams{Package: "nope"})
	require.Contains(t, resp.Content, "No package matching")
}

// A type read apart from its members is much harder to follow.
func TestAPISurfaceToolIndentsMembersUnderTheirType(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "client/a.go", apiFixture)

	resp := runAPISurfaceTool(t, dir, APISurfaceParams{})
	require.Contains(t, resp.Content, "      Timeout int")
	require.Contains(t, resp.Content, "      func (*Client) Do")
}

func TestAPISurfaceToolReportsNoGoFiles(t *testing.T) {
	resp := runAPISurfaceTool(t, t.TempDir(), APISurfaceParams{})
	require.Contains(t, resp.Content, "No Go files found")
}

func TestAPISurfaceToolErrorsOnAMissingPath(t *testing.T) {
	resp := runAPISurfaceTool(t, t.TempDir(), APISurfaceParams{Path: "nope"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "cannot scan")
}

func TestAPISurfaceToolReportsMetadata(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "client/a.go", apiFixture)

	resp := runAPISurfaceTool(t, dir, APISurfaceParams{})
	var meta APISurfaceResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.Packages)
	require.Positive(t, meta.Symbols)
	require.Equal(t, 1, meta.Undocumented, "only Bare has no doc comment")
	require.Equal(t, 1, meta.FilesScanned)
}
