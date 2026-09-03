package codeintel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// writeGo drops a .go file into dir, creating parents as needed.
func writeGo(t *testing.T, dir, name, src string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(src), 0o644))
	return path
}

// names returns just the symbol names, for compact assertions.
func names(syms []DeadSymbol) []string {
	out := make([]string, 0, len(syms))
	for _, s := range syms {
		out = append(out, s.Name)
	}
	return out
}

func TestFindDeadCodeReportsAnUncalledFunction(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func Used() int { return 1 }

func Unused() int { return 2 }

func caller() int { return Used() }
`)

	got, err := FindDeadCode(dir, false, true)
	require.NoError(t, err)
	require.Contains(t, names(got.Symbols), "Unused")
	require.NotContains(t, names(got.Symbols), "Used")
}

func TestFindDeadCodeSeesUsesFromAnotherFile(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func Helper() int { return 1 }
`)
	writeGo(t, dir, "b.go", `package a

func Run() int { return Helper() }
`)

	got, err := FindDeadCode(dir, false, true)
	require.NoError(t, err)
	require.NotContains(t, names(got.Symbols), "Helper")
}

// A symbol used only by a test is dead as far as production code is
// concerned, and that is the whole point of excluding tests -- but it must
// come back to life the moment tests are included.
func TestFindDeadCodeHonoursTheIncludeTestsFlag(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func OnlyTestsUseMe() int { return 1 }
`)
	writeGo(t, dir, "a_test.go", `package a

import "testing"

func TestIt(t *testing.T) { _ = OnlyTestsUseMe() }
`)

	withoutTests, err := FindDeadCode(dir, false, true)
	require.NoError(t, err)
	require.Contains(t, names(withoutTests.Symbols), "OnlyTestsUseMe")
	require.True(t, withoutTests.SkippedTests)

	withTests, err := FindDeadCode(dir, true, true)
	require.NoError(t, err)
	require.NotContains(t, names(withTests.Symbols), "OnlyTestsUseMe")
	require.False(t, withTests.SkippedTests)
}

// main, init and the testing-framework entry points are called by the
// toolchain, never by name, so reporting them would be pure noise.
func TestFindDeadCodeIgnoresEntryPoints(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "main.go", `package main

func init() {}

func main() {}
`)
	writeGo(t, dir, "x_test.go", `package main

import "testing"

func TestSomething(t *testing.T)      {}
func BenchmarkSomething(b *testing.B) {}
`)

	got, err := FindDeadCode(dir, true, true)
	require.NoError(t, err)
	require.NotContains(t, names(got.Symbols), "main")
	require.NotContains(t, names(got.Symbols), "init")
	require.NotContains(t, names(got.Symbols), "TestSomething")
	require.NotContains(t, names(got.Symbols), "BenchmarkSomething")
}

func TestFindDeadCodeCanRestrictItselfToExportedSymbols(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func Exported() {}

func unexported() {}
`)

	exportedOnly, err := FindDeadCode(dir, false, false)
	require.NoError(t, err)
	require.Equal(t, []string{"Exported"}, names(exportedOnly.Symbols))

	all, err := FindDeadCode(dir, false, true)
	require.NoError(t, err)
	require.Contains(t, names(all.Symbols), "unexported")
}

func TestFindDeadCodeCoversTypesConstsAndVars(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

type Orphan struct{}

const OrphanConst = 1

var OrphanVar = 2
`)

	got, err := FindDeadCode(dir, false, true)
	require.NoError(t, err)

	byName := map[string]string{}
	for _, s := range got.Symbols {
		byName[s.Name] = s.Kind
	}
	require.Equal(t, "type", byName["Orphan"])
	require.Equal(t, "const", byName["OrphanConst"])
	require.Equal(t, "var", byName["OrphanVar"])
}

func TestFindDeadCodeReportsAccurateFileAndLine(t *testing.T) {
	dir := t.TempDir()
	path := writeGo(t, dir, "a.go", `package a

func Orphan() {}
`)

	got, err := FindDeadCode(dir, false, true)
	require.NoError(t, err)
	require.Len(t, got.Symbols, 1)
	require.Equal(t, path, got.Symbols[0].File)
	require.Equal(t, 3, got.Symbols[0].Line)
	require.True(t, got.Symbols[0].Exported)
}

// A tree mid-edit is the normal case for an agent, so one unparsable file
// must not take the whole scan down with it.
func TestFindDeadCodeSkipsFilesThatDoNotParse(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "good.go", `package a

func Orphan() {}
`)
	writeGo(t, dir, "broken.go", `package a

func ( this is not go {{{
`)

	got, err := FindDeadCode(dir, false, true)
	require.NoError(t, err)
	require.Equal(t, 1, got.FilesScanned)
	require.Contains(t, names(got.Symbols), "Orphan")
}

func TestFindDeadCodeSkipsVendorAndTestdata(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func Orphan() {}
`)
	writeGo(t, dir, "vendor/dep/d.go", `package dep

func VendorOrphan() {}
`)
	writeGo(t, dir, "testdata/t.go", `package testdata

func TestdataOrphan() {}
`)

	got, err := FindDeadCode(dir, false, true)
	require.NoError(t, err)
	require.Equal(t, 1, got.FilesScanned)
	require.NotContains(t, names(got.Symbols), "VendorOrphan")
	require.NotContains(t, names(got.Symbols), "TestdataOrphan")
}

func TestFindDeadCodeAcceptsASingleFile(t *testing.T) {
	dir := t.TempDir()
	path := writeGo(t, dir, "a.go", `package a

func Orphan() {}
`)

	got, err := FindDeadCode(path, false, true)
	require.NoError(t, err)
	require.Equal(t, 1, got.FilesScanned)
	require.Contains(t, names(got.Symbols), "Orphan")
}

func TestFindDeadCodeFailsClearlyOnAMissingPath(t *testing.T) {
	_, err := FindDeadCode(filepath.Join(t.TempDir(), "nope"), false, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot scan")
}

// The blank identifier is a declaration name that can never be referenced,
// so reporting it as dead would be noise on every file with a var _ = ...
// interface assertion.
func TestFindDeadCodeIgnoresTheBlankIdentifier(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

var _ = 1
`)

	got, err := FindDeadCode(dir, false, true)
	require.NoError(t, err)
	require.Empty(t, got.Symbols)
}
