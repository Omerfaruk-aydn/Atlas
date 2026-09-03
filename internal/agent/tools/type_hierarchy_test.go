package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runTypeHierarchyTool(t *testing.T, workingDir string, params TypeHierarchyParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewTypeHierarchyTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  TypeHierarchyToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

const hierarchyFixture = `package a

type Speaker interface {
	Speak() string
}

type Dog struct{}

func (d Dog) Speak() string { return "woof" }

type Robot struct{}

func (r *Robot) Speak() string { return "beep" }
`

func TestTypeHierarchyToolListsInterfacesWithTheirImplementations(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", hierarchyFixture)

	resp := runTypeHierarchyTool(t, dir, TypeHierarchyParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "Speaker")
	require.Contains(t, resp.Content, "Dog")
}

// A pointer-receiver method set belongs to *T. Writing it as T in the
// report would send the caller into a compile error.
func TestTypeHierarchyToolSpellsPointerReceiversWithAStar(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", hierarchyFixture)

	resp := runTypeHierarchyTool(t, dir, TypeHierarchyParams{})
	require.Contains(t, resp.Content, "*Robot")
}

func TestTypeHierarchyToolFocusesOnAnInterface(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", hierarchyFixture)

	resp := runTypeHierarchyTool(t, dir, TypeHierarchyParams{Focus: "Speaker"})
	require.Contains(t, resp.Content, "interface Speaker")
	require.Contains(t, resp.Content, "implemented by:")
	require.Contains(t, resp.Content, "Dog")
	require.Contains(t, resp.Content, "*Robot")
}

// The same parameter has to work from the other side, or the caller has
// to know in advance which kind of name they are holding.
func TestTypeHierarchyToolFocusesOnAConcreteType(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", hierarchyFixture)

	resp := runTypeHierarchyTool(t, dir, TypeHierarchyParams{Focus: "Dog"})
	require.Contains(t, resp.Content, "type Dog")
	require.Contains(t, resp.Content, "satisfies:")
	require.Contains(t, resp.Content, "Speaker")
}

func TestTypeHierarchyToolReportsAnUnknownFocus(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", hierarchyFixture)

	resp := runTypeHierarchyTool(t, dir, TypeHierarchyParams{Focus: "Nonexistent"})
	require.Contains(t, resp.Content, "No interface or type named")
}

// "Nobody implements this" is the result most likely to be acted on
// wrongly, so the caveat has to travel with it.
func TestTypeHierarchyToolQualifiesAnEmptyImplementationList(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", `package a

type Lonely interface {
	Nobody() int
}
`)

	resp := runTypeHierarchyTool(t, dir, TypeHierarchyParams{Focus: "Lonely"})
	require.Contains(t, resp.Content, "nothing in this tree")
	require.Contains(t, resp.Content, "other modules")
}

func TestTypeHierarchyToolShowsMethodSignatures(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", `package a

type Store interface {
	Get(key string) ([]byte, error)
}
`)

	resp := runTypeHierarchyTool(t, dir, TypeHierarchyParams{Focus: "Store"})
	require.Contains(t, resp.Content, "Get(string) ([]byte, error)")
}

func TestTypeHierarchyToolReportsNoGoFiles(t *testing.T) {
	resp := runTypeHierarchyTool(t, t.TempDir(), TypeHierarchyParams{})
	require.Contains(t, resp.Content, "No Go files found")
}

func TestTypeHierarchyToolErrorsOnAMissingPath(t *testing.T) {
	resp := runTypeHierarchyTool(t, t.TempDir(), TypeHierarchyParams{Path: "nope"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "cannot scan")
}

func TestTypeHierarchyToolHonoursIncludeTests(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", `package a

type Speaker interface{ Speak() string }
`)
	writeDeadCodeFile(t, dir, "a_test.go", `package a

type fakeSpeaker struct{}

func (f fakeSpeaker) Speak() string { return "" }
`)

	resp := runTypeHierarchyTool(t, dir, TypeHierarchyParams{Focus: "Speaker"})
	require.NotContains(t, resp.Content, "fakeSpeaker")

	yes := true
	resp = runTypeHierarchyTool(t, dir, TypeHierarchyParams{Focus: "Speaker", IncludeTests: &yes})
	require.Contains(t, resp.Content, "fakeSpeaker")
}

func TestTypeHierarchyToolReportsMetadata(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", hierarchyFixture)

	resp := runTypeHierarchyTool(t, dir, TypeHierarchyParams{})
	var meta TypeHierarchyResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.Interfaces)
	require.Equal(t, 2, meta.Implementations)
	require.Equal(t, 1, meta.FilesScanned)
}
