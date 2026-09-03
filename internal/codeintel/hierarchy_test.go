package codeintel

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// implPairs flattens the result into "Type->Interface" strings.
func implPairs(r HierarchyResult) []string {
	out := make([]string, 0, len(r.Implementations))
	for _, impl := range r.Implementations {
		out = append(out, impl.Type.Name+"->"+impl.Interface.Name)
	}
	return out
}

func TestTypeHierarchyMatchesASimpleImplementation(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

type Speaker interface {
	Speak(loud bool) string
}

type Dog struct{}

func (d Dog) Speak(loud bool) string { return "woof" }

type Rock struct{}
`)

	got, err := TypeHierarchy(dir, false)
	require.NoError(t, err)
	require.Contains(t, implPairs(got), "Dog->Speaker")
	require.NotContains(t, implPairs(got), "Rock->Speaker")
}

// A parameter's name has no bearing on assignability, so it must not stop
// two identical signatures from matching.
func TestTypeHierarchyIgnoresParameterNames(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

type Writer interface {
	Write(p []byte) (int, error)
}

type Sink struct{}

func (s Sink) Write(buffer []byte) (int, error) { return 0, nil }
`)

	got, err := TypeHierarchy(dir, false)
	require.NoError(t, err)
	require.Contains(t, implPairs(got), "Sink->Writer")
}

func TestTypeHierarchyRejectsAMismatchedSignature(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

type Speaker interface {
	Speak() string
}

type Mute struct{}

func (m Mute) Speak() error { return nil }
`)

	got, err := TypeHierarchy(dir, false)
	require.NoError(t, err)
	require.Empty(t, got.Implementations)
}

func TestTypeHierarchyRequiresEveryMethod(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

type ReadWriter interface {
	Read() string
	Write(s string)
}

type Half struct{}

func (h Half) Read() string { return "" }

type Full struct{}

func (f Full) Read() string   { return "" }
func (f Full) Write(s string) {}
`)

	got, err := TypeHierarchy(dir, false)
	require.NoError(t, err)
	require.Contains(t, implPairs(got), "Full->ReadWriter")
	require.NotContains(t, implPairs(got), "Half->ReadWriter")
}

// Only *T has a pointer-receiver method in its method set, and the report
// has to say so or the caller will assign the wrong thing.
func TestTypeHierarchyFlagsPointerReceivers(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

type Closer interface {
	Close() error
}

type Handle struct{}

func (h *Handle) Close() error { return nil }
`)

	got, err := TypeHierarchy(dir, false)
	require.NoError(t, err)
	require.Len(t, got.Implementations, 1)
	require.Equal(t, "Handle", got.Implementations[0].Type.Name)
	require.True(t, got.Implementations[0].ViaPointer)
}

func TestTypeHierarchyFlattensEmbeddedInterfaces(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

type Reader interface {
	Read() string
}

type Writer interface {
	Write(s string)
}

type ReadWriter interface {
	Reader
	Writer
}

type Both struct{}

func (b Both) Read() string   { return "" }
func (b Both) Write(s string) {}

type OnlyRead struct{}

func (o OnlyRead) Read() string { return "" }
`)

	got, err := TypeHierarchy(dir, false)
	require.NoError(t, err)
	require.Contains(t, implPairs(got), "Both->ReadWriter")
	require.NotContains(t, implPairs(got), "OnlyRead->ReadWriter")

	// The flattened set is what the report shows, so it has to be right.
	for _, iface := range got.Interfaces {
		if iface.Name == "ReadWriter" {
			require.Len(t, iface.Methods, 2)
		}
	}
}

// An embedding chain that cannot be resolved (io.Reader lives outside the
// tree) must not hang or crash; it just under-reports.
func TestTypeHierarchyToleratesUnresolvableEmbeds(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

import "io"

type Mine interface {
	io.Reader
	Extra() int
}

type T struct{}

func (t T) Extra() int { return 0 }
`)

	got, err := TypeHierarchy(dir, false)
	require.NoError(t, err)
	// T covers every method the tree can see, so it matches. That is the
	// documented under-approximation, not a bug.
	require.Contains(t, implPairs(got), "T->Mine")
}

// An empty interface is satisfied by everything, which is true and
// useless; emitting one row per type would drown the real results.
func TestTypeHierarchySkipsTheEmptyInterface(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

type Any interface{}

type T struct{}

func (t T) M() {}
`)

	got, err := TypeHierarchy(dir, false)
	require.NoError(t, err)
	require.Empty(t, got.Implementations)
}

func TestTypeHierarchyCollectsMethodsDeclaredAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "iface.go", `package a

type Pair interface {
	One() int
	Two() int
}
`)
	writeGo(t, dir, "one.go", `package a

type T struct{}

func (t T) One() int { return 1 }
`)
	writeGo(t, dir, "two.go", `package a

func (t T) Two() int { return 2 }
`)

	got, err := TypeHierarchy(dir, false)
	require.NoError(t, err)
	require.Contains(t, implPairs(got), "T->Pair")
}

func TestTypeHierarchyHandlesGenericReceivers(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

type Lener interface {
	Len() int
}

type Stack[T any] struct{ items []T }

func (s Stack[T]) Len() int { return len(s.items) }
`)

	got, err := TypeHierarchy(dir, false)
	require.NoError(t, err)
	require.Contains(t, implPairs(got), "Stack->Lener")
}

func TestTypeHierarchyRecordsInterfaceLocations(t *testing.T) {
	dir := t.TempDir()
	path := writeGo(t, dir, "a.go", `package a

type Speaker interface {
	Speak() string
}
`)

	got, err := TypeHierarchy(dir, false)
	require.NoError(t, err)
	require.Len(t, got.Interfaces, 1)
	require.Equal(t, path, got.Interfaces[0].File)
	require.Equal(t, 3, got.Interfaces[0].Line)
	require.Equal(t, "a", got.Interfaces[0].Package)
}

func TestTypeHierarchySkipsFilesThatDoNotParse(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "good.go", `package a

type Speaker interface{ Speak() string }
`)
	writeGo(t, dir, "broken.go", `package a

type {{{ nope
`)

	got, err := TypeHierarchy(dir, false)
	require.NoError(t, err)
	require.Equal(t, 1, got.FilesScanned)
	require.Len(t, got.Interfaces, 1)
}

// A type must never be reported as implementing an interface of its own
// name, which would happen if an interface were also collected as a
// concrete type.
func TestTypeHierarchyDoesNotMatchATypeAgainstItself(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

type Speaker interface {
	Speak() string
}
`)

	got, err := TypeHierarchy(dir, false)
	require.NoError(t, err)
	require.Empty(t, got.Implementations)
}
