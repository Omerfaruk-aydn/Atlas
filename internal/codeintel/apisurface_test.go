package codeintel

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func apiNames(p APIPackage) []string {
	out := make([]string, 0, len(p.Symbols))
	for _, s := range p.Symbols {
		out = append(out, s.Name)
	}
	return out
}

func symbolNamed(t *testing.T, p APIPackage, name string) APISymbol {
	t.Helper()
	for _, s := range p.Symbols {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("no exported symbol %q (have %v)", name, apiNames(p))
	return APISymbol{}
}

func TestAPISurfaceListsOnlyExportedDeclarations(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func Exported() {}

func unexported() {}

type Public struct{}

type private struct{}

const PublicConst = 1

const privateConst = 2
`)

	got, err := APISurface(dir, false)
	require.NoError(t, err)
	require.Len(t, got.Packages, 1)

	names := apiNames(got.Packages[0])
	require.Contains(t, names, "Exported")
	require.Contains(t, names, "Public")
	require.Contains(t, names, "PublicConst")
	require.NotContains(t, names, "unexported")
	require.NotContains(t, names, "private")
	require.NotContains(t, names, "privateConst")
}

func TestAPISurfaceRendersSignatures(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func New(name string, n int) (*Client, error) { return nil, nil }

type Client struct{}
`)

	got, err := APISurface(dir, false)
	require.NoError(t, err)
	require.Equal(t, "func New(string, int) (*Client, error)",
		symbolNamed(t, got.Packages[0], "New").Signature)
}

func TestAPISurfaceRendersMethodsWithTheirReceiver(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

type Client struct{}

func (c *Client) Do(req string) error { return nil }
`)

	got, err := APISurface(dir, false)
	require.NoError(t, err)
	sym := symbolNamed(t, got.Packages[0], "Do")
	require.Equal(t, "method", sym.Kind)
	require.Equal(t, "Client", sym.Recv)
	require.Equal(t, "func (*Client) Do(string) error", sym.Signature)
}

// A capitalised method on an unexported type is not reachable from
// outside, so it is not part of the surface.
func TestAPISurfaceExcludesMethodsOnUnexportedTypes(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

type hidden struct{}

func (h hidden) PubliclyNamed() {}
`)

	got, err := APISurface(dir, false)
	require.NoError(t, err)
	require.NotContains(t, apiNames(got.Packages[0]), "PubliclyNamed")
}

// An exported field is part of the contract just as much as a method:
// changing its type breaks callers the same way.
func TestAPISurfaceIncludesExportedStructFields(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

type Config struct {
	Name    string
	Timeout int
	secret  string
}
`)

	got, err := APISurface(dir, false)
	require.NoError(t, err)
	names := apiNames(got.Packages[0])
	require.Contains(t, names, "Name")
	require.Contains(t, names, "Timeout")
	require.NotContains(t, names, "secret")

	field := symbolNamed(t, got.Packages[0], "Name")
	require.Equal(t, "field", field.Kind)
	require.Equal(t, "Config", field.Recv)
	require.Equal(t, "Name string", field.Signature)
}

// An embedded field promotes its whole method set, so it belongs in the
// surface.
func TestAPISurfaceIncludesEmbeddedFields(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

import "sync"

type Guarded struct {
	sync.Mutex
	Value int
}
`)

	got, err := APISurface(dir, false)
	require.NoError(t, err)
	sym := symbolNamed(t, got.Packages[0], "Mutex")
	require.Equal(t, "field", sym.Kind)
	require.Contains(t, sym.Signature, "embedded")
}

// Reprinting whole doc comments makes the listing longer than the source
// it summarises; Go convention puts the summary on the first line.
func TestAPISurfaceKeepsOnlyTheDocSummary(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

// New creates a client.
//
// It reads the environment, opens a connection, and returns an error if
// the server is unreachable. None of this belongs in a surface listing.
func New() {}
`)

	got, err := APISurface(dir, false)
	require.NoError(t, err)
	sym := symbolNamed(t, got.Packages[0], "New")
	require.Equal(t, "New creates a client.", sym.Doc)
	require.NotContains(t, sym.Doc, "environment")
}

func TestAPISurfaceFlagsDeprecatedSymbols(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

// Old does a thing.
//
// Deprecated: use New instead.
func Old() {}

// New does the thing properly.
func New() {}
`)

	got, err := APISurface(dir, false)
	require.NoError(t, err)
	require.True(t, symbolNamed(t, got.Packages[0], "Old").Deprecated)
	require.False(t, symbolNamed(t, got.Packages[0], "New").Deprecated)
}

// A grouped declaration carries its comment on the group, not on each
// spec, and losing that would report documented symbols as undocumented.
func TestAPISurfaceFallsBackToTheGroupDocComment(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

// Status values reported by the server.
const (
	StatusOK = 1
)

// Registry holds the things.
type Registry struct{}
`)

	got, err := APISurface(dir, false)
	require.NoError(t, err)
	require.Equal(t, "Status values reported by the server.",
		symbolNamed(t, got.Packages[0], "StatusOK").Doc)
	require.Equal(t, "Registry holds the things.",
		symbolNamed(t, got.Packages[0], "Registry").Doc)
}

func TestAPISurfaceCountsUndocumentedSymbols(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

// Documented has a comment.
func Documented() {}

func Bare() {}
`)

	got, err := APISurface(dir, false)
	require.NoError(t, err)
	require.Equal(t, 1, got.Packages[0].Undocumented)
}

// A type read apart from its own methods is much harder to follow than
// one read with them.
func TestAPISurfaceGroupsATypeWithItsMethodsAndFields(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

func ZebraFunc() {}

type Alpha struct {
	Field string
}

func (a Alpha) Method() {}
`)

	got, err := APISurface(dir, false)
	require.NoError(t, err)

	names := apiNames(got.Packages[0])
	alphaIdx, fieldIdx, methodIdx := -1, -1, -1
	for i, n := range names {
		switch n {
		case "Alpha":
			alphaIdx = i
		case "Field":
			fieldIdx = i
		case "Method":
			methodIdx = i
		}
	}
	require.Less(t, alphaIdx, fieldIdx)
	require.Less(t, fieldIdx, methodIdx)
}

// A large struct's whole body in a signature is the entire file.
func TestAPISurfaceNamesTheShapeRatherThanTheBody(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

type Big struct {
	A, B, C, D, E, F string
}

type Doer interface {
	Do() error
}

type Alias = string
`)

	got, err := APISurface(dir, false)
	require.NoError(t, err)
	require.Equal(t, "type Big struct", symbolNamed(t, got.Packages[0], "Big").Signature)
	require.Equal(t, "type Doer interface", symbolNamed(t, got.Packages[0], "Doer").Signature)
	require.Equal(t, "type Alias string", symbolNamed(t, got.Packages[0], "Alias").Signature)
}

func TestAPISurfaceSeparatesPackages(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "one/a.go", `package one

func FromOne() {}
`)
	writeGo(t, dir, "two/b.go", `package two

func FromTwo() {}
`)

	got, err := APISurface(dir, false)
	require.NoError(t, err)
	require.Len(t, got.Packages, 2)
	require.Equal(t, []string{"FromOne"}, apiNames(got.Packages[0]))
	require.Equal(t, []string{"FromTwo"}, apiNames(got.Packages[1]))
}

func TestAPISurfaceRecordsLocations(t *testing.T) {
	dir := t.TempDir()
	path := writeGo(t, dir, "a.go", `package a

func Exported() {}
`)

	got, err := APISurface(dir, false)
	require.NoError(t, err)
	sym := symbolNamed(t, got.Packages[0], "Exported")
	require.Equal(t, path, sym.File)
	require.Equal(t, 3, sym.Line)
}

func TestAPISurfaceSkipsFilesThatDoNotParse(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "good.go", "package a\n\nfunc Good() {}\n")
	writeGo(t, dir, "bad.go", "package a\n\nfunc ((( {\n")

	got, err := APISurface(dir, false)
	require.NoError(t, err)
	require.Equal(t, 1, got.FilesScanned)
	require.Equal(t, []string{"Good"}, apiNames(got.Packages[0]))
}

func TestAPISurfaceFailsClearlyOnAMissingPath(t *testing.T) {
	_, err := APISurface(t.TempDir()+"/nope", false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot scan")
}
