package codeintel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func todoKinds(r TodoResult) []string {
	out := make([]string, 0, len(r.Todos))
	for _, todo := range r.Todos {
		out = append(out, todo.Kind)
	}
	return out
}

func TestFindTodosRecognisesEveryMarker(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", `package a

// TODO: one
// FIXME: two
// HACK: three
// XXX: four
// BUG: five
// OPTIMIZE: six
// DEPRECATED: seven
`)

	got, err := FindTodos(dir, TodoOptions{})
	require.NoError(t, err)
	require.ElementsMatch(t,
		[]string{"TODO", "FIXME", "HACK", "XXX", "BUG", "OPTIMIZE", "DEPRECATED"},
		todoKinds(got))
}

// Without anchoring to a comment opener, every identifier containing
// "todo" matches and the result is unusable on any project that manages
// a todo list.
func TestFindTodosDoesNotMatchProseOrIdentifiers(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", `package a

// This function renders the user's todos on the dashboard.
func renderTodos(todoList []string) {}

var todoCount = len(todoList)
`)

	got, err := FindTodos(dir, TodoOptions{})
	require.NoError(t, err)
	require.Empty(t, got.Todos)
}

// An owned marker is one somebody can be asked about; an unowned one
// usually belongs to nobody.
// Both of these were found by running the scanner over a real
// repository, and both were reported as markers before the pattern was
// tightened. On this codebase they and their like were most of the
// findings, which is what makes a scan not worth reading.
func TestFindTodosIgnoresTheFalsePositivesARealScanFound(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", `package a

// Todo is one marker found in a comment.
type Todo struct{}

func build() {
	todo := Todo{}
	_ = todo
}

// Bug reports an issue.
func Bug() {}
`)

	got, err := FindTodos(dir, TodoOptions{})
	require.NoError(t, err)
	require.Empty(t, got.Todos,
		"a doc comment on a type named Todo, and an identifier at the start of a line, are not markers")
}

// The flip side of that tightening: a marker without a colon must still
// be found when it is written the conventional way, in capitals.
func TestFindTodosStillMatchesAnAllCapsMarkerWithoutAColon(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", `package a

// TODO handle the empty case
// FIXME this breaks on retry
`)

	got, err := FindTodos(dir, TodoOptions{})
	require.NoError(t, err)
	require.Len(t, got.Todos, 2)
	require.Equal(t, "handle the empty case", got.Todos[0].Text)
}

// A lower-case marker is real when it is punctuated like one.
func TestFindTodosMatchesALowerCaseMarkerWithAColon(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", `package a

// todo: still a marker
// Fixme(bob): so is this
`)

	got, err := FindTodos(dir, TodoOptions{})
	require.NoError(t, err)
	require.Len(t, got.Todos, 2)
	require.Equal(t, "TODO", got.Todos[0].Kind)
	require.Equal(t, "FIXME", got.Todos[1].Kind)
	require.Equal(t, "bob", got.Todos[1].Owner)
}

func TestFindTodosCapturesTheOwner(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", `package a

// TODO(alice): ask her about this
// TODO: nobody owns this
`)

	got, err := FindTodos(dir, TodoOptions{})
	require.NoError(t, err)
	require.Len(t, got.Todos, 2)
	require.Equal(t, "alice", got.Todos[0].Owner)
	require.Empty(t, got.Todos[1].Owner)
}

func TestFindTodosCapturesATicketReference(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", `package a

// TODO: fix this properly, see #1234
// FIXME: blocked on PROJ-42
// TODO: no ticket here
`)

	got, err := FindTodos(dir, TodoOptions{})
	require.NoError(t, err)
	require.Equal(t, "#1234", got.Todos[0].Ticket)
	require.Equal(t, "PROJ-42", got.Todos[1].Ticket)
	require.Empty(t, got.Todos[2].Ticket)
}

func TestFindTodosCapturesTheText(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", `package a

// TODO: handle the empty case
`)

	got, err := FindTodos(dir, TodoOptions{})
	require.NoError(t, err)
	require.Equal(t, "handle the empty case", got.Todos[0].Text)
	require.Equal(t, 3, got.Todos[0].Line)
}

// Different languages open comments differently, and a scanner that only
// knows // misses most of a polyglot repository.
func TestFindTodosHandlesEveryCommentStyle(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.py", "# TODO: python\n")
	writeFile(t, dir, "a.sql", "-- TODO: sql\n")
	writeFile(t, dir, "a.html", "<!-- TODO: html -->\n")
	writeFile(t, dir, "a.c", "/* TODO: c block */\n")
	writeFile(t, dir, "a.go", "package a\n\n/*\n * TODO: continuation line\n */\n")

	got, err := FindTodos(dir, TodoOptions{})
	require.NoError(t, err)
	require.Len(t, got.Todos, 5)
	for _, todo := range got.Todos {
		require.Equal(t, "TODO", todo.Kind)
		require.NotEmpty(t, todo.Text)
		// A block comment's closing token must not end up in the text.
		require.NotContains(t, todo.Text, "*/")
		require.NotContains(t, todo.Text, "-->")
	}
}

// OPTIMISE and OPTIMIZE are one marker; two counts split the tally.
func TestFindTodosNormalisesSpellingVariants(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", `package a

// OPTIMISE: british
// OPTIMIZE: american
`)

	got, err := FindTodos(dir, TodoOptions{})
	require.NoError(t, err)
	require.Equal(t, 2, got.ByKind["OPTIMIZE"])
	require.Len(t, got.ByKind, 1)
}

func TestFindTodosIsCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", `package a

// todo: lowercase
// Fixme: mixed
`)

	got, err := FindTodos(dir, TodoOptions{})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"TODO", "FIXME"}, todoKinds(got))
}

func TestFindTodosFiltersByKind(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", `package a

// TODO: keep
// HACK: drop
`)

	got, err := FindTodos(dir, TodoOptions{Kinds: []string{"todo"}})
	require.NoError(t, err)
	require.Equal(t, []string{"TODO"}, todoKinds(got))
}

func TestFindTodosSkipsTestFilesByDefault(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "package a\n\n// TODO: production\n")
	writeFile(t, dir, "a_test.go", "package a\n\n// TODO: test\n")

	got, err := FindTodos(dir, TodoOptions{})
	require.NoError(t, err)
	require.Len(t, got.Todos, 1)
	require.Equal(t, "production", got.Todos[0].Text)

	withTests, err := FindTodos(dir, TodoOptions{IncludeTests: true})
	require.NoError(t, err)
	require.Len(t, withTests.Todos, 2)
}

// A marker in a dependency is not this project's debt.
func TestFindTodosSkipsDependencyTrees(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "package a\n\n// TODO: ours\n")
	writeFile(t, dir, "node_modules/x/b.js", "// TODO: theirs\n")
	writeFile(t, dir, "vendor/y/c.go", "// TODO: vendored\n")

	got, err := FindTodos(dir, TodoOptions{})
	require.NoError(t, err)
	require.Len(t, got.Todos, 1)
	require.Equal(t, "ours", got.Todos[0].Text)
}

func TestFindTodosRestrictsByExtension(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "package a\n\n// TODO: go\n")
	writeFile(t, dir, "a.py", "# TODO: python\n")

	got, err := FindTodos(dir, TodoOptions{Extensions: []string{"go"}})
	require.NoError(t, err)
	require.Len(t, got.Todos, 1)
	require.Equal(t, "go", got.Todos[0].Text)
}

func TestFindTodosHonoursTheResultLimit(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	b.WriteString("package a\n")
	for range 50 {
		b.WriteString("// TODO: one of many\n")
	}
	writeFile(t, dir, "a.go", b.String())

	got, err := FindTodos(dir, TodoOptions{MaxResults: 5})
	require.NoError(t, err)
	require.Len(t, got.Todos, 5)
}

func TestFindTodosCountsByKind(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", `package a

// TODO: one
// TODO: two
// FIXME: three
`)

	got, err := FindTodos(dir, TodoOptions{})
	require.NoError(t, err)
	require.Equal(t, 2, got.ByKind["TODO"])
	require.Equal(t, 1, got.ByKind["FIXME"])
}

func TestFindTodosAcceptsASingleFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "a.go", "package a\n\n// TODO: here\n")

	got, err := FindTodos(path, TodoOptions{})
	require.NoError(t, err)
	require.Len(t, got.Todos, 1)
	require.Equal(t, 1, got.FilesScanned)
}

func TestFindTodosFailsClearlyOnAMissingPath(t *testing.T) {
	_, err := FindTodos(filepath.Join(t.TempDir(), "nope"), TodoOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot scan")
}

// A minified bundle on one line finds nothing worth having and costs
// real time to scan.
func TestFindTodosSkipsAbsurdlyLongLines(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bundle.js", strings.Repeat("a", 3000)+"// TODO: buried\n")

	got, err := FindTodos(dir, TodoOptions{})
	require.NoError(t, err)
	require.Empty(t, got.Todos)
}

func TestFindTodosSortsByFileThenLine(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "zzz.go", "package a\n\n// TODO: last file\n")
	writeFile(t, dir, "aaa.go", "package a\n\n// TODO: first\n// TODO: second\n")

	got, err := FindTodos(dir, TodoOptions{})
	require.NoError(t, err)
	require.Len(t, got.Todos, 3)
	require.Contains(t, got.Todos[0].File, "aaa.go")
	require.Equal(t, 3, got.Todos[0].Line)
	require.Equal(t, 4, got.Todos[1].Line)
	require.Contains(t, got.Todos[2].File, "zzz.go")
}
