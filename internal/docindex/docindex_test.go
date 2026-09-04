package docindex

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeDoc(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func findDoc(t *testing.T, files []DocFile, suffix string) *DocFile {
	t.Helper()
	for i := range files {
		if filepathHasSuffix(files[i].Path, suffix) {
			return &files[i]
		}
	}
	return nil
}

func filepathHasSuffix(path, suffix string) bool {
	return len(path) >= len(suffix) && path[len(path)-len(suffix):] == suffix
}

func TestBuildIndexesHeadings(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, dir, "a.md", "# Title\n\nSome text.\n\n## Section One\n\nMore text.\n\n### Detail\n")

	got, err := Build(dir, Options{})
	require.NoError(t, err)
	doc := findDoc(t, got.Files, "a.md")
	require.NotNil(t, doc)
	require.Equal(t, "Title", doc.Title)
	require.Len(t, doc.Headings, 3)
	require.Equal(t, 1, doc.Headings[0].Level)
	require.Equal(t, 2, doc.Headings[1].Level)
	require.Equal(t, "Section One", doc.Headings[1].Text)
	require.Equal(t, 3, doc.Headings[2].Level)
}

func TestBuildFallsBackToFilenameTitle(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, dir, "getting_started.md", "## Not an H1\n")

	got, err := Build(dir, Options{})
	require.NoError(t, err)
	doc := findDoc(t, got.Files, "getting_started.md")
	require.NotNil(t, doc)
	require.Equal(t, "getting started", doc.Title)
}

func TestBuildIgnoresHashesInFencedCodeBlocks(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, dir, "a.md", "# Title\n\n```\n# not a heading\n```\n\n## Real Section\n")

	got, err := Build(dir, Options{})
	require.NoError(t, err)
	doc := findDoc(t, got.Files, "a.md")
	require.NotNil(t, doc)
	require.Len(t, doc.Headings, 2)
	require.Equal(t, "Real Section", doc.Headings[1].Text)
}

func TestBuildIgnoresARunOfHashesWithNoSpace(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, dir, "a.md", "#!/usr/bin/env not-a-heading\n\n# Real Title\n")

	got, err := Build(dir, Options{})
	require.NoError(t, err)
	doc := findDoc(t, got.Files, "a.md")
	require.NotNil(t, doc)
	require.Len(t, doc.Headings, 1)
	require.Equal(t, "Real Title", doc.Headings[0].Text)
}

func TestBuildStripsTrailingHashes(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, dir, "a.md", "# Title #\n")

	got, err := Build(dir, Options{})
	require.NoError(t, err)
	doc := findDoc(t, got.Files, "a.md")
	require.NotNil(t, doc)
	require.Equal(t, "Title", doc.Headings[0].Text)
}

func TestBuildRespectsMaxDepth(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, dir, "a.md", "# Title\n\n## Section\n\n### Detail\n")

	got, err := Build(dir, Options{MaxDepth: 2})
	require.NoError(t, err)
	doc := findDoc(t, got.Files, "a.md")
	require.NotNil(t, doc)
	require.Len(t, doc.Headings, 2)
}

func TestBuildFiltersByQuery(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, dir, "a.md", "# Installation\n")
	writeDoc(t, dir, "b.md", "# Unrelated\n")

	got, err := Build(dir, Options{Query: "install"})
	require.NoError(t, err)
	require.Len(t, got.Files, 1)
	require.Equal(t, "Installation", got.Files[0].Title)
}

func TestBuildQueryMatchesNestedHeading(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, dir, "a.md", "# Guide\n\n## Authentication Setup\n")

	got, err := Build(dir, Options{Query: "authentication"})
	require.NoError(t, err)
	require.Len(t, got.Files, 1)
}

func TestBuildSkipsNodeModulesAndVendor(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, dir, "node_modules/pkg/README.md", "# Should be skipped\n")
	writeDoc(t, dir, "vendor/lib/README.md", "# Should be skipped\n")
	writeDoc(t, dir, "README.md", "# Kept\n")

	got, err := Build(dir, Options{})
	require.NoError(t, err)
	require.Equal(t, 1, got.FilesScanned)
}

func TestBuildScansASingleFile(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, dir, "sub/a.md", "# Title\n")

	got, err := Build(filepath.Join(dir, "sub", "a.md"), Options{})
	require.NoError(t, err)
	require.Len(t, got.Files, 1)
}

func TestBuildReportsNoFiles(t *testing.T) {
	dir := t.TempDir()
	got, err := Build(dir, Options{})
	require.NoError(t, err)
	require.Equal(t, 0, got.FilesScanned)
	require.Empty(t, got.Files)
}
