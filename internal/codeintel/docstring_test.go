package codeintel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeDocstringFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func findSuggestion(t *testing.T, suggestions []DocstringSuggestion, name string) *DocstringSuggestion {
	t.Helper()
	for i := range suggestions {
		if suggestions[i].Name == name {
			return &suggestions[i]
		}
	}
	return nil
}

func TestGenerateDocstringsFlagsAnUndocumentedExportedFunc(t *testing.T) {
	dir := t.TempDir()
	writeDocstringFile(t, dir, "a.go", `package a

func Foo(name string, count int) error {
	return nil
}
`)

	got, err := GenerateDocstrings(dir, DocstringOptions{})
	require.NoError(t, err)
	s := findSuggestion(t, got.Suggestions, "Foo")
	require.NotNil(t, s)
	require.Equal(t, "func", s.Kind)
	require.Contains(t, s.Stub, "// Foo TODO: describe what Foo does.")
	require.Contains(t, s.Stub, "- name: TODO (string).")
	require.Contains(t, s.Stub, "- count: TODO (int).")
	require.Contains(t, s.Stub, "Returns TODO, or an error if TODO.")
}

func TestGenerateDocstringsSkipsADocumentedFunc(t *testing.T) {
	dir := t.TempDir()
	writeDocstringFile(t, dir, "a.go", `package a

// Foo already has a comment.
func Foo() {}
`)

	got, err := GenerateDocstrings(dir, DocstringOptions{})
	require.NoError(t, err)
	require.Nil(t, findSuggestion(t, got.Suggestions, "Foo"))
}

func TestGenerateDocstringsSkipsAnUnexportedFunc(t *testing.T) {
	dir := t.TempDir()
	writeDocstringFile(t, dir, "a.go", `package a

func foo() {}
`)

	got, err := GenerateDocstrings(dir, DocstringOptions{})
	require.NoError(t, err)
	require.Empty(t, got.Suggestions)
}

func TestGenerateDocstringsOmitsALoneContextParam(t *testing.T) {
	dir := t.TempDir()
	writeDocstringFile(t, dir, "a.go", `package a

import "context"

func Foo(ctx context.Context, id string) {}
`)

	got, err := GenerateDocstrings(dir, DocstringOptions{})
	require.NoError(t, err)
	s := findSuggestion(t, got.Suggestions, "Foo")
	require.NotNil(t, s)
	require.NotContains(t, s.Stub, "ctx")
	require.Contains(t, s.Stub, "- id: TODO (string).")
}

func TestGenerateDocstringsHandlesAMethodWithExportedReceiver(t *testing.T) {
	dir := t.TempDir()
	writeDocstringFile(t, dir, "a.go", `package a

type Client struct{}

func (c *Client) Do() error { return nil }
`)

	got, err := GenerateDocstrings(dir, DocstringOptions{})
	require.NoError(t, err)
	s := findSuggestion(t, got.Suggestions, "Do")
	require.NotNil(t, s)
	require.Equal(t, "method", s.Kind)
	require.Equal(t, "Client", s.Recv)
}

func TestGenerateDocstringsSkipsAMethodWithUnexportedReceiver(t *testing.T) {
	dir := t.TempDir()
	writeDocstringFile(t, dir, "a.go", `package a

type client struct{}

func (c *client) Do() error { return nil }
`)

	got, err := GenerateDocstrings(dir, DocstringOptions{})
	require.NoError(t, err)
	require.Empty(t, got.Suggestions)
}

func TestGenerateDocstringsFlagsAnUndocumentedExportedType(t *testing.T) {
	dir := t.TempDir()
	writeDocstringFile(t, dir, "a.go", `package a

type Config struct {
	Name string
}
`)

	got, err := GenerateDocstrings(dir, DocstringOptions{})
	require.NoError(t, err)
	s := findSuggestion(t, got.Suggestions, "Config")
	require.NotNil(t, s)
	require.Equal(t, "type", s.Kind)
	require.Contains(t, s.Stub, "// Config TODO: describe what Config represents.")
}

func TestGenerateDocstringsFiltersBySymbol(t *testing.T) {
	dir := t.TempDir()
	writeDocstringFile(t, dir, "a.go", `package a

func Foo() {}

func Bar() {}
`)

	got, err := GenerateDocstrings(dir, DocstringOptions{Symbol: "Bar"})
	require.NoError(t, err)
	require.Len(t, got.Suggestions, 1)
	require.Equal(t, "Bar", got.Suggestions[0].Name)
}

func TestGenerateDocstringsSkipsTestFilesByDefault(t *testing.T) {
	dir := t.TempDir()
	writeDocstringFile(t, dir, "a_test.go", `package a

func Foo() {}
`)

	got, err := GenerateDocstrings(dir, DocstringOptions{})
	require.NoError(t, err)
	require.Equal(t, 0, got.FilesScanned)
}

func TestGenerateDocstringsIncludesTestFilesWhenAsked(t *testing.T) {
	dir := t.TempDir()
	writeDocstringFile(t, dir, "a_test.go", `package a

func Foo() {}
`)

	got, err := GenerateDocstrings(dir, DocstringOptions{IncludeTests: true})
	require.NoError(t, err)
	require.NotNil(t, findSuggestion(t, got.Suggestions, "Foo"))
}

func TestGenerateDocstringsNoResultReturnsNoNote(t *testing.T) {
	dir := t.TempDir()
	writeDocstringFile(t, dir, "a.go", `package a

func Foo() {}
`)

	got, err := GenerateDocstrings(dir, DocstringOptions{})
	require.NoError(t, err)
	s := findSuggestion(t, got.Suggestions, "Foo")
	require.NotNil(t, s)
	require.NotContains(t, s.Stub, "Returns")
	require.NotContains(t, s.Stub, "Parameters")
}

func TestGenerateDocstringsSkipsUnparsableFiles(t *testing.T) {
	dir := t.TempDir()
	writeDocstringFile(t, dir, "broken.go", "not valid go (((")

	got, err := GenerateDocstrings(dir, DocstringOptions{})
	require.NoError(t, err)
	require.Equal(t, 0, got.FilesScanned)
	require.Empty(t, got.Suggestions)
}
