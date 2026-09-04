package codeintel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeSemanticFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func findMatch(t *testing.T, matches []SymbolMatch, name string) *SymbolMatch {
	t.Helper()
	for i := range matches {
		if matches[i].Name == name {
			return &matches[i]
		}
	}
	return nil
}

func TestSemanticSearchMatchesByCamelCaseName(t *testing.T) {
	dir := t.TempDir()
	writeSemanticFile(t, dir, "a.go", "package a\n\nfunc GenerateDocstring() {}\n\nfunc Unrelated() {}\n")

	got, err := SemanticSearch(dir, "generate docstring", SearchOptions{})
	require.NoError(t, err)
	m := findMatch(t, got.Matches, "GenerateDocstring")
	require.NotNil(t, m)
	require.Nil(t, findMatch(t, got.Matches, "Unrelated"))
}

func TestSemanticSearchMatchesUnexportedSymbols(t *testing.T) {
	dir := t.TempDir()
	writeSemanticFile(t, dir, "a.go", "package a\n\nfunc redactCredential(s string) string { return s }\n")

	got, err := SemanticSearch(dir, "redact credential", SearchOptions{})
	require.NoError(t, err)
	require.NotNil(t, findMatch(t, got.Matches, "redactCredential"))
}

func TestSemanticSearchMatchesDocComment(t *testing.T) {
	dir := t.TempDir()
	writeSemanticFile(t, dir, "a.go", "package a\n\n// Foo scans the tree for orphaned widgets.\nfunc Foo() {}\n")

	got, err := SemanticSearch(dir, "orphaned widgets", SearchOptions{})
	require.NoError(t, err)
	require.NotNil(t, findMatch(t, got.Matches, "Foo"))
}

func TestSemanticSearchRanksNameMatchAboveDocMatch(t *testing.T) {
	dir := t.TempDir()
	writeSemanticFile(t, dir, "a.go", `package a

func ParseWidget() {}

// helper is used while parsing a widget.
func helper() {}
`)

	got, err := SemanticSearch(dir, "widget", SearchOptions{})
	require.NoError(t, err)
	require.Len(t, got.Matches, 2)
	require.Equal(t, "ParseWidget", got.Matches[0].Name)
}

func TestSemanticSearchFindsTypesAndConsts(t *testing.T) {
	dir := t.TempDir()
	writeSemanticFile(t, dir, "a.go", "package a\n\ntype WidgetConfig struct{}\n\nconst maxWidgets = 10\n")

	got, err := SemanticSearch(dir, "widget", SearchOptions{})
	require.NoError(t, err)
	require.NotNil(t, findMatch(t, got.Matches, "WidgetConfig"))
	require.NotNil(t, findMatch(t, got.Matches, "maxWidgets"))
}

func TestSemanticSearchFindsMethods(t *testing.T) {
	dir := t.TempDir()
	writeSemanticFile(t, dir, "a.go", "package a\n\ntype Client struct{}\n\nfunc (c *Client) FetchWidget() error { return nil }\n")

	got, err := SemanticSearch(dir, "fetch widget", SearchOptions{})
	require.NoError(t, err)
	m := findMatch(t, got.Matches, "FetchWidget")
	require.NotNil(t, m)
	require.Equal(t, "method", m.Kind)
	require.Equal(t, "Client", m.Recv)
}

func TestSemanticSearchReturnsNoMatches(t *testing.T) {
	dir := t.TempDir()
	writeSemanticFile(t, dir, "a.go", "package a\n\nfunc Foo() {}\n")

	got, err := SemanticSearch(dir, "completely unrelated term", SearchOptions{})
	require.NoError(t, err)
	require.Empty(t, got.Matches)
}

func TestSemanticSearchRespectsLimit(t *testing.T) {
	dir := t.TempDir()
	writeSemanticFile(t, dir, "a.go", "package a\n\nfunc Widget1() {}\nfunc Widget2() {}\nfunc Widget3() {}\n")

	got, err := SemanticSearch(dir, "widget", SearchOptions{Limit: 2})
	require.NoError(t, err)
	require.Len(t, got.Matches, 2)
}

func TestSemanticSearchSkipsTestFilesByDefault(t *testing.T) {
	dir := t.TempDir()
	writeSemanticFile(t, dir, "a_test.go", "package a\n\nfunc TestWidget(t *testing.T) {}\n")

	got, err := SemanticSearch(dir, "widget", SearchOptions{})
	require.NoError(t, err)
	require.Equal(t, 0, got.FilesScanned)
}

func TestSemanticSearchReportsMatchedTerms(t *testing.T) {
	dir := t.TempDir()
	writeSemanticFile(t, dir, "a.go", "package a\n\nfunc ParseWidget() {}\n")

	got, err := SemanticSearch(dir, "parse widget extra", SearchOptions{})
	require.NoError(t, err)
	m := findMatch(t, got.Matches, "ParseWidget")
	require.NotNil(t, m)
	require.Contains(t, m.MatchedTerms, "parse")
	require.Contains(t, m.MatchedTerms, "widget")
	require.NotContains(t, m.MatchedTerms, "extra")
}
