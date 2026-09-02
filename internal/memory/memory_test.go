package memory

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	return New(Options{
		ProjectDir: filepath.Join(dir, "project"),
		UserDir:    filepath.Join(dir, "user"),
	})
}

func TestReadOfAnUnwrittenStoreIsEmpty(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	for _, scope := range Scopes {
		got, err := s.Read(scope)
		require.NoError(t, err, "never having learned anything is not a failure")
		require.Empty(t, got)
	}
}

func TestAddWritesOneEntryPerLine(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	_, err := s.Add(ScopeProject, "the build needs CGO_ENABLED=1 for -race")
	require.NoError(t, err)
	got, err := s.Add(ScopeProject, "tests live beside the code, not in a tests/ tree")
	require.NoError(t, err)

	require.Equal(t,
		"- the build needs CGO_ENABLED=1 for -race\n"+
			"- tests live beside the code, not in a tests/ tree\n",
		got)

	onDisk, err := os.ReadFile(s.Path(ScopeProject))
	require.NoError(t, err)
	require.Equal(t, got, string(onDisk), "what was returned is what was stored")
}

func TestAddIsIdempotent(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	first, err := s.Add(ScopeUser, "prefers Turkish")
	require.NoError(t, err)
	second, err := s.Add(ScopeUser, "prefers Turkish")
	require.NoError(t, err)

	require.Equal(t, first, second, "the same fact twice is one entry")
	require.Equal(t, 1, strings.Count(second, "prefers Turkish"))
}

func TestAddRejectsMultilineEntries(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	_, err := s.Add(ScopeProject, "one thing\nand another")
	require.Error(t, err, "a multi-line entry would break replace and remove")
}

func TestAddRefusesToExceedTheLimit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := New(Options{
		ProjectDir:   dir,
		UserDir:      dir,
		ProjectLimit: 40,
	})

	_, err := s.Add(ScopeProject, strings.Repeat("x", 60))

	var tooLong *ErrTooLong
	require.ErrorAs(t, err, &tooLong, "the write fails rather than the store silently dropping text")
	require.Equal(t, 40, tooLong.Limit)
	require.Contains(t, tooLong.Error(), "over the 40 limit")

	// Nothing was written, so a failed write leaves the store as it was.
	got, err := s.Read(ScopeProject)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestANegativeLimitIsUnbounded(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := New(Options{ProjectDir: dir, UserDir: dir, ProjectLimit: -1})

	_, err := s.Add(ScopeProject, strings.Repeat("x", 100_000))
	require.NoError(t, err)
}

func TestReplaceNeedsAUniqueMatch(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	_, err := s.Add(ScopeProject, "go 1.26")
	require.NoError(t, err)
	_, err = s.Add(ScopeProject, "go 1.26 toolchain pinned in go.mod")
	require.NoError(t, err)

	_, err = s.Replace(ScopeProject, "go 1.26", "go 1.27")
	require.ErrorIs(t, err, ErrAmbiguous)

	got, err := s.Replace(ScopeProject, "go 1.26 toolchain", "go 1.27 toolchain")
	require.NoError(t, err)
	require.Contains(t, got, "go 1.27 toolchain pinned")
	require.Contains(t, got, "- go 1.26\n", "the other entry is untouched")
}

func TestReplaceReportsAMissingMatch(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	_, err := s.Replace(ScopeProject, "nothing like this", "x")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestRemoveTakesTheWholeLine(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	_, err := s.Add(ScopeProject, "first")
	require.NoError(t, err)
	_, err = s.Add(ScopeProject, "second")
	require.NoError(t, err)

	got, err := s.Remove(ScopeProject, "first")
	require.NoError(t, err)
	require.Equal(t, "- second\n", got, "no empty bullet is left behind")
}

func TestRemovingTheLastEntryRemovesTheFile(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	_, err := s.Add(ScopeProject, "only")
	require.NoError(t, err)
	got, err := s.Remove(ScopeProject, "only")
	require.NoError(t, err)
	require.Empty(t, got)

	_, err = os.Stat(s.Path(ScopeProject))
	require.True(t, errors.Is(err, os.ErrNotExist), "an empty store is no file")
}

func TestSetReplacesTheWholeStore(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	_, err := s.Add(ScopeProject, "one")
	require.NoError(t, err)

	got, err := s.Set(ScopeProject, "- consolidated\n- into two lines")
	require.NoError(t, err)
	require.Equal(t, "- consolidated\n- into two lines\n", got)
}

func TestSetIsBoundedToo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := New(Options{ProjectDir: dir, UserDir: dir, ProjectLimit: 10})

	_, err := s.Set(ScopeProject, strings.Repeat("y", 50))
	var tooLong *ErrTooLong
	require.ErrorAs(t, err, &tooLong, "no caller routes around the bound")
}

func TestTheTwoScopesAreSeparateFiles(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	_, err := s.Add(ScopeProject, "about the code")
	require.NoError(t, err)
	_, err = s.Add(ScopeUser, "about the person")
	require.NoError(t, err)

	project, err := s.Read(ScopeProject)
	require.NoError(t, err)
	user, err := s.Read(ScopeUser)
	require.NoError(t, err)

	require.Contains(t, project, "about the code")
	require.NotContains(t, project, "about the person")
	require.Contains(t, user, "about the person")
	require.Equal(t, "MEMORY.md", filepath.Base(s.Path(ScopeProject)))
	require.Equal(t, "USER.md", filepath.Base(s.Path(ScopeUser)))
}

func TestUnknownScopeIsRejected(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	_, err := s.Add(Scope("everything"), "x")
	require.Error(t, err)
	_, err = s.Read(Scope("everything"))
	require.Error(t, err)
}

func TestUsedReportsTheBound(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	_, err := s.Add(ScopeProject, "abc")
	require.NoError(t, err)

	used, limit, err := s.Used(ScopeProject)
	require.NoError(t, err)
	require.Equal(t, len("- abc\n"), used)
	require.Equal(t, DefaultProjectLimit, limit)
}

func TestTheLimitCountsCharactersNotBytes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// "ö" is two bytes but one character. A byte-counting limit would
	// give a Turkish user a smaller store than an English one.
	s := New(Options{ProjectDir: dir, UserDir: dir, ProjectLimit: 12})

	_, err := s.Add(ScopeProject, "öööööööö") // "- " + 8 + "\n" = 11 characters
	require.NoError(t, err)
}
