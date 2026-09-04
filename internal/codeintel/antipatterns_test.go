package codeintel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeAntiPatternFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func findKind(t *testing.T, findings []AntiPatternFinding, kind string) *AntiPatternFinding {
	t.Helper()
	for i := range findings {
		if findings[i].Kind == kind {
			return &findings[i]
		}
	}
	return nil
}

func TestScanAntiPatternsFlagsAnEmptyErrCheck(t *testing.T) {
	dir := t.TempDir()
	writeAntiPatternFile(t, dir, "a.go", `package a

func f() error {
	err := g()
	if err != nil {
	}
	return nil
}

func g() error { return nil }
`)

	got, err := ScanAntiPatterns(dir, AntiPatternOptions{})
	require.NoError(t, err)
	f := findKind(t, got.Findings, "swallowed-error")
	require.NotNil(t, f)
	require.Contains(t, f.Message, "err")
}

func TestScanAntiPatternsIgnoresAHandledErrCheck(t *testing.T) {
	dir := t.TempDir()
	writeAntiPatternFile(t, dir, "a.go", `package a

func f() error {
	err := g()
	if err != nil {
		return err
	}
	return nil
}

func g() error { return nil }
`)

	got, err := ScanAntiPatterns(dir, AntiPatternOptions{})
	require.NoError(t, err)
	require.Nil(t, findKind(t, got.Findings, "swallowed-error"))
}

func TestScanAntiPatternsFlagsABlankErrDiscard(t *testing.T) {
	dir := t.TempDir()
	writeAntiPatternFile(t, dir, "a.go", `package a

func f() {
	err := g()
	_ = err
}

func g() error { return nil }
`)

	got, err := ScanAntiPatterns(dir, AntiPatternOptions{})
	require.NoError(t, err)
	f := findKind(t, got.Findings, "swallowed-error")
	require.NotNil(t, f)
	require.Contains(t, f.Snippet, "_ = err")
}

func TestScanAntiPatternsFlagsContextNotFirst(t *testing.T) {
	dir := t.TempDir()
	writeAntiPatternFile(t, dir, "a.go", `package a

import "context"

func f(name string, ctx context.Context) {
	_ = ctx
	_ = name
}
`)

	got, err := ScanAntiPatterns(dir, AntiPatternOptions{})
	require.NoError(t, err)
	f := findKind(t, got.Findings, "context-not-first")
	require.NotNil(t, f)
	require.Equal(t, "f", f.Func)
}

func TestScanAntiPatternsAcceptsContextFirst(t *testing.T) {
	dir := t.TempDir()
	writeAntiPatternFile(t, dir, "a.go", `package a

import "context"

func f(ctx context.Context, name string) {
	_ = ctx
	_ = name
}
`)

	got, err := ScanAntiPatterns(dir, AntiPatternOptions{})
	require.NoError(t, err)
	require.Nil(t, findKind(t, got.Findings, "context-not-first"))
}

func TestScanAntiPatternsFlagsPanicInALibraryFunction(t *testing.T) {
	dir := t.TempDir()
	writeAntiPatternFile(t, dir, "lib/a.go", `package lib

func Parse(s string) int {
	if s == "" {
		panic("empty input")
	}
	return len(s)
}
`)

	got, err := ScanAntiPatterns(dir, AntiPatternOptions{})
	require.NoError(t, err)
	f := findKind(t, got.Findings, "panic-in-library")
	require.NotNil(t, f)
	require.Equal(t, "Parse", f.Func)
}

func TestScanAntiPatternsAllowsPanicInMustFunctions(t *testing.T) {
	dir := t.TempDir()
	writeAntiPatternFile(t, dir, "lib/a.go", `package lib

func MustParse(s string) int {
	if s == "" {
		panic("empty input")
	}
	return len(s)
}
`)

	got, err := ScanAntiPatterns(dir, AntiPatternOptions{})
	require.NoError(t, err)
	require.Nil(t, findKind(t, got.Findings, "panic-in-library"))
}

func TestScanAntiPatternsAllowsPanicInPackageMain(t *testing.T) {
	dir := t.TempDir()
	writeAntiPatternFile(t, dir, "cmd/a.go", `package main

func run() {
	panic("boom")
}

func main() { run() }
`)

	got, err := ScanAntiPatterns(dir, AntiPatternOptions{})
	require.NoError(t, err)
	require.Nil(t, findKind(t, got.Findings, "panic-in-library"))
}

func TestScanAntiPatternsSkipsTestFilesByDefault(t *testing.T) {
	dir := t.TempDir()
	writeAntiPatternFile(t, dir, "a_test.go", `package a

func TestX() {
	err := g()
	_ = err
}

func g() error { return nil }
`)

	got, err := ScanAntiPatterns(dir, AntiPatternOptions{})
	require.NoError(t, err)
	require.Empty(t, got.Findings)
	require.Equal(t, 0, got.FilesScanned)
}

func TestScanAntiPatternsIncludesTestFilesWhenAsked(t *testing.T) {
	dir := t.TempDir()
	writeAntiPatternFile(t, dir, "a_test.go", `package a

func TestX() {
	err := g()
	_ = err
}

func g() error { return nil }
`)

	got, err := ScanAntiPatterns(dir, AntiPatternOptions{IncludeTests: true})
	require.NoError(t, err)
	require.Equal(t, 1, got.FilesScanned)
	require.NotNil(t, findKind(t, got.Findings, "swallowed-error"), "IncludeTests scans the file, and the discard check applies once it is scanned")
}

func TestScanAntiPatternsCountsByKind(t *testing.T) {
	dir := t.TempDir()
	writeAntiPatternFile(t, dir, "a.go", `package a

func f() {
	err := g()
	_ = err
	err2 := g()
	if err2 != nil {
	}
}

func g() error { return nil }
`)

	got, err := ScanAntiPatterns(dir, AntiPatternOptions{})
	require.NoError(t, err)
	require.Equal(t, 2, got.ByKind["swallowed-error"])
}

func TestScanAntiPatternsSkipsUnparsableFiles(t *testing.T) {
	dir := t.TempDir()
	writeAntiPatternFile(t, dir, "broken.go", "package a\nfunc f( {\n")
	writeAntiPatternFile(t, dir, "ok.go", "package a\nfunc g() {}\n")

	got, err := ScanAntiPatterns(dir, AntiPatternOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, got.FilesScanned)
}
