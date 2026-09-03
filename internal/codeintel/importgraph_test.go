package codeintel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// modTree writes a go.mod plus the given files, so import paths resolve
// the way they do in a real module.
func modTree(t *testing.T, modulePath string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module "+modulePath+"\n\ngo 1.26\n"), 0o644))
	for name, src := range files {
		writeGo(t, dir, name, src)
	}
	return dir
}

func pkgByPath(t *testing.T, r ImportGraphResult, path string) PackageNode {
	t.Helper()
	for _, p := range r.Packages {
		if p.ImportPath == path {
			return p
		}
	}
	t.Fatalf("package %q not in graph", path)
	return PackageNode{}
}

func TestImportGraphResolvesInternalEdgesThroughTheModulePath(t *testing.T) {
	dir := modTree(t, "example.com/m", map[string]string{
		"a/a.go": `package a

import "example.com/m/b"

var _ = b.X
`,
		"b/b.go": `package b

var X = 1
`,
	})

	got, err := ImportGraph(dir, false)
	require.NoError(t, err)
	require.Equal(t, "example.com/m", got.Module)
	require.Equal(t, []string{"example.com/m/b"}, pkgByPath(t, got, "example.com/m/a").Internal)
}

func TestImportGraphSeparatesExternalImports(t *testing.T) {
	dir := modTree(t, "example.com/m", map[string]string{
		"a/a.go": `package a

import (
	"fmt"
	"example.com/m/b"
	"github.com/other/dep"
)

var _ = fmt.Sprint(b.X, dep.Y)
`,
		"b/b.go": `package b

var X = 1
`,
	})

	got, err := ImportGraph(dir, false)
	require.NoError(t, err)
	a := pkgByPath(t, got, "example.com/m/a")
	require.Equal(t, []string{"example.com/m/b"}, a.Internal)
	require.Equal(t, []string{"fmt", "github.com/other/dep"}, a.External)
}

func TestImportGraphFindsATwoPackageCycle(t *testing.T) {
	dir := modTree(t, "example.com/m", map[string]string{
		"a/a.go": `package a

import "example.com/m/b"

var _ = b.X
`,
		"b/b.go": `package b

import "example.com/m/a"

var X = 1
var _ = a.Y
`,
	})

	got, err := ImportGraph(dir, false)
	require.NoError(t, err)
	require.Len(t, got.Cycles, 1)
	require.ElementsMatch(t,
		[]string{"example.com/m/a", "example.com/m/b"},
		got.Cycles[0])
}

func TestImportGraphFindsALongerCycle(t *testing.T) {
	dir := modTree(t, "example.com/m", map[string]string{
		"a/a.go": `package a
import "example.com/m/b"
var _ = b.X
`,
		"b/b.go": `package b
import "example.com/m/c"
var X = c.Y
`,
		"c/c.go": `package c
import "example.com/m/a"
var Y = a.Z
`,
	})

	got, err := ImportGraph(dir, false)
	require.NoError(t, err)
	require.Len(t, got.Cycles, 1)
	require.Len(t, got.Cycles[0], 3)
}

// The same loop reached from three different entry points is one cycle,
// not three.
func TestImportGraphReportsEachCycleOnce(t *testing.T) {
	dir := modTree(t, "example.com/m", map[string]string{
		"a/a.go": `package a
import "example.com/m/b"
var _ = b.X
`,
		"b/b.go": `package b
import "example.com/m/a"
var X = a.Z
`,
		"c/c.go": `package c
import "example.com/m/a"
var _ = a.Z
`,
		"d/d.go": `package d
import "example.com/m/b"
var _ = b.X
`,
	})

	got, err := ImportGraph(dir, false)
	require.NoError(t, err)
	require.Len(t, got.Cycles, 1)
}

func TestImportGraphReportsNoCyclesForAnAcyclicTree(t *testing.T) {
	dir := modTree(t, "example.com/m", map[string]string{
		"a/a.go": `package a
import "example.com/m/b"
var _ = b.X
`,
		"b/b.go": `package b
import "example.com/m/c"
var X = c.Y
`,
		"c/c.go": `package c
var Y = 1
`,
	})

	got, err := ImportGraph(dir, false)
	require.NoError(t, err)
	require.Empty(t, got.Cycles)
}

// A package is a directory, so several files in one directory are one
// node with the union of their imports.
func TestImportGraphMergesFilesInOneDirectory(t *testing.T) {
	dir := modTree(t, "example.com/m", map[string]string{
		"a/one.go": `package a
import "example.com/m/b"
var _ = b.X
`,
		"a/two.go": `package a
import "example.com/m/c"
var _ = c.Y
`,
		"b/b.go": `package b
var X = 1
`,
		"c/c.go": `package c
var Y = 1
`,
	})

	got, err := ImportGraph(dir, false)
	require.NoError(t, err)
	a := pkgByPath(t, got, "example.com/m/a")
	require.Equal(t, 2, a.Files)
	require.Equal(t, []string{"example.com/m/b", "example.com/m/c"}, a.Internal)
}

// Without a go.mod there is no module path to anchor to, but same-tree
// edges must still be usable.
func TestImportGraphWorksWithoutAGoMod(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a/a.go", `package a
var X = 1
`)
	writeGo(t, dir, "b/b.go", `package b
var Y = 1
`)

	got, err := ImportGraph(dir, false)
	require.NoError(t, err)
	require.Empty(t, got.Module)
	require.Len(t, got.Packages, 2)
	require.Equal(t, "a", got.Packages[0].ImportPath)
}

func TestImportGraphDedupesRepeatedImports(t *testing.T) {
	dir := modTree(t, "example.com/m", map[string]string{
		"a/one.go": `package a
import "example.com/m/b"
var _ = b.X
`,
		"a/two.go": `package a
import "example.com/m/b"
var _ = b.X
`,
		"b/b.go": `package b
var X = 1
`,
	})

	got, err := ImportGraph(dir, false)
	require.NoError(t, err)
	require.Equal(t, []string{"example.com/m/b"}, pkgByPath(t, got, "example.com/m/a").Internal)
}

// A package importing itself is not a cycle worth reporting; it is not
// even legal Go, and treating it as one would produce a length-1 loop.
func TestImportGraphIgnoresASelfImport(t *testing.T) {
	dir := modTree(t, "example.com/m", map[string]string{
		"a/a.go": `package a
import "example.com/m/a"
var _ = a.X
`,
	})

	got, err := ImportGraph(dir, false)
	require.NoError(t, err)
	require.Empty(t, got.Cycles)
}

func TestImportGraphSkipsFilesThatDoNotParse(t *testing.T) {
	dir := modTree(t, "example.com/m", map[string]string{
		"a/a.go":      `package a` + "\n",
		"a/broken.go": "package a\n\nimport ( \"unterminated\n",
	})

	got, err := ImportGraph(dir, false)
	require.NoError(t, err)
	require.Len(t, got.Packages, 1)
	require.Equal(t, 1, pkgByPath(t, got, "example.com/m/a").Files)
}

func TestImportGraphRecordsTheRootPackage(t *testing.T) {
	dir := modTree(t, "example.com/m", map[string]string{
		"main.go": `package main

func main() {}
`,
	})

	got, err := ImportGraph(dir, false)
	require.NoError(t, err)
	require.Equal(t, "example.com/m", got.Packages[0].ImportPath)
	require.Equal(t, "main", got.Packages[0].Name)
}

func TestImportGraphFailsClearlyOnAMissingPath(t *testing.T) {
	_, err := ImportGraph(filepath.Join(t.TempDir(), "nope"), false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot scan")
}
