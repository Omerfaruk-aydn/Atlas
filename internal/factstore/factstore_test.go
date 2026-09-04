package factstore

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return New(filepath.Join(t.TempDir(), "facts.jsonl"))
}

func TestRetainRejectsEmptyText(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Retain("  ", nil)
	require.ErrorIs(t, err, ErrEmptyText)
}

func TestRetainAssignsAnIDAndTimestamp(t *testing.T) {
	s := newTestStore(t)
	fact, err := s.Retain("the build uses go 1.22", []string{"Build"})
	require.NoError(t, err)
	require.NotEmpty(t, fact.ID)
	require.Equal(t, "the build uses go 1.22", fact.Text)
	require.Equal(t, []string{"build"}, fact.Tags)
	require.WithinDuration(t, time.Now(), fact.CreatedAt, 5*time.Second)
}

func TestRetainGivesEachFactAUniqueID(t *testing.T) {
	s := newTestStore(t)
	a, err := s.Retain("fact one", nil)
	require.NoError(t, err)
	b, err := s.Retain("fact two", nil)
	require.NoError(t, err)
	require.NotEqual(t, a.ID, b.ID)
}

func TestRecallMatchesByTextKeyword(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Retain("the api key is loaded from an env var", nil)
	require.NoError(t, err)
	_, err = s.Retain("unrelated fact about widgets", nil)
	require.NoError(t, err)

	got, err := s.Recall("api key", 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Contains(t, got[0].Text, "api key")
}

func TestRecallWeighsTagMatchAboveTextMatch(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Retain("this mentions widget only in passing", nil)
	require.NoError(t, err)
	_, err = s.Retain("the real widget fact", []string{"widget"})
	require.NoError(t, err)

	got, err := s.Recall("widget", 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "the real widget fact", got[0].Text)
}

func TestRecallWithEmptyQueryReturnsMostRecentFirst(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Retain("first", nil)
	require.NoError(t, err)
	time.Sleep(2 * time.Millisecond)
	_, err = s.Retain("second", nil)
	require.NoError(t, err)

	got, err := s.Recall("", 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "second", got[0].Text)
	require.Equal(t, "first", got[1].Text)
}

func TestRecallRespectsLimit(t *testing.T) {
	s := newTestStore(t)
	for range 5 {
		_, err := s.Retain("widget fact", nil)
		require.NoError(t, err)
	}

	got, err := s.Recall("widget", 2)
	require.NoError(t, err)
	require.Len(t, got, 2)
}

func TestRecallReturnsNothingWhenStoreIsEmpty(t *testing.T) {
	s := newTestStore(t)
	got, err := s.Recall("anything", 10)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestReflectCountsAndTags(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Retain("fact a", []string{"build"})
	require.NoError(t, err)
	_, err = s.Retain("fact b", []string{"build", "ci"})
	require.NoError(t, err)

	got, err := s.Reflect()
	require.NoError(t, err)
	require.Equal(t, 2, got.Total)
	require.Equal(t, 2, got.ByTag["build"])
	require.Equal(t, 1, got.ByTag["ci"])
}

func TestReflectFindsDuplicates(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Retain("the same fact", nil)
	require.NoError(t, err)
	_, err = s.Retain("The Same Fact", nil)
	require.NoError(t, err)
	_, err = s.Retain("a different fact", nil)
	require.NoError(t, err)

	got, err := s.Reflect()
	require.NoError(t, err)
	require.Len(t, got.Duplicates, 1)
	require.Len(t, got.Duplicates[0], 2)
}

func TestReflectReportsOldestAndNewest(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Retain("first", nil)
	require.NoError(t, err)
	time.Sleep(2 * time.Millisecond)
	_, err = s.Retain("second", nil)
	require.NoError(t, err)

	got, err := s.Reflect()
	require.NoError(t, err)
	require.True(t, got.OldestAt.Before(got.NewestAt))
}

func TestReflectOnEmptyStore(t *testing.T) {
	s := newTestStore(t)
	got, err := s.Reflect()
	require.NoError(t, err)
	require.Equal(t, 0, got.Total)
	require.Empty(t, got.Duplicates)
}

func TestStoreSurvivesACorruptLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "facts.jsonl")
	s := New(path)
	_, err := s.Retain("good fact", nil)
	require.NoError(t, err)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("not valid json\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	facts, err := s.readAll()
	require.NoError(t, err)
	require.Len(t, facts, 1)
	require.Equal(t, "good fact", facts[0].Text)
}
