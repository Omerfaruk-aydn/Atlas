package cmd

import (
	"bytes"
	"testing"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/db"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/session"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func days(now time.Time, n int) int64 { return now.AddDate(0, 0, -n).Unix() }

func TestNothingIsPrunedWithoutCriteria(t *testing.T) {
	now := time.Now()
	sessions := []session.Session{
		{ID: "old", UpdatedAt: days(now, 500)},
		{ID: "new", UpdatedAt: days(now, 1)},
	}
	require.Empty(t, sessionsToPrune(sessions, pruneCriteria{}, now))
}

func TestPruneByAge(t *testing.T) {
	now := time.Now()
	sessions := []session.Session{
		{ID: "recent", UpdatedAt: days(now, 5)},
		{ID: "stale", UpdatedAt: days(now, 40)},
	}
	got := sessionsToPrune(sessions, pruneCriteria{OlderThanDays: 30}, now)
	require.Len(t, got, 1)
	require.Equal(t, "stale", got[0].ID)
}

func TestPruneByCount(t *testing.T) {
	now := time.Now()
	sessions := []session.Session{
		{ID: "third", UpdatedAt: days(now, 3)},
		{ID: "first", UpdatedAt: days(now, 1)},
		{ID: "second", UpdatedAt: days(now, 2)},
	}
	got := sessionsToPrune(sessions, pruneCriteria{Keep: 2}, now)
	require.Len(t, got, 1)
	require.Equal(t, "third", got[0].ID)
}

// With both given, a session has to fail both tests: "keep the 2 newest, and
// anything from the last 30 days".
func TestBothCriteriaMustAgree(t *testing.T) {
	now := time.Now()

	// Old, but inside --keep: kept.
	sessions := []session.Session{
		{ID: "newest", UpdatedAt: days(now, 1)},
		{ID: "old-but-kept", UpdatedAt: days(now, 300)},
		{ID: "old-and-past-keep", UpdatedAt: days(now, 400)},
	}
	got := sessionsToPrune(sessions, pruneCriteria{OlderThanDays: 30, Keep: 2}, now)
	require.Len(t, got, 1)
	require.Equal(t, "old-and-past-keep", got[0].ID)

	// Past --keep, but inside --older-than: also kept, so nothing goes.
	recent := []session.Session{
		{ID: "a", UpdatedAt: days(now, 1)},
		{ID: "b", UpdatedAt: days(now, 2)},
		{ID: "c", UpdatedAt: days(now, 3)},
	}
	require.Empty(t, sessionsToPrune(recent, pruneCriteria{OlderThanDays: 30, Keep: 2}, now))
}

func TestPrunedSessionsComeBackOldestFirst(t *testing.T) {
	now := time.Now()
	sessions := []session.Session{
		{ID: "b", UpdatedAt: days(now, 100)},
		{ID: "c", UpdatedAt: days(now, 50)},
		{ID: "a", UpdatedAt: days(now, 200)},
	}
	got := sessionsToPrune(sessions, pruneCriteria{OlderThanDays: 30}, now)
	require.Equal(t, []string{"a", "b", "c"}, []string{got[0].ID, got[1].ID, got[2].ID})
}

func newPruneTestCmd(t *testing.T, dataDir string) *cobra.Command {
	t.Helper()
	c := &cobra.Command{RunE: runSessionPrune}
	c.Flags().String("data-dir", dataDir, "")
	c.Flags().IntVar(&sessionPruneOlderThan, "older-than", 0, "")
	c.Flags().IntVar(&sessionPruneKeep, "keep", 0, "")
	c.Flags().BoolVar(&sessionPruneDryRun, "dry-run", false, "")
	c.SetContext(t.Context())
	t.Cleanup(func() {
		sessionPruneOlderThan = 0
		sessionPruneKeep = 0
		sessionPruneDryRun = false
	})
	return c
}

// A bare `session prune` must not wipe the history.
func TestSessionPruneRefusesWithoutCriteria(t *testing.T) {
	c := newPruneTestCmd(t, t.TempDir())
	err := c.RunE(c, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "--older-than")
}

// seedPruneSessions writes titles as sessions and releases its handle
// again. Release, not Close: db.Connect pools one handle per data directory
// (see TestSessionSetupCanBeUsedTwiceInOneProcess).
func seedPruneSessions(t *testing.T, dataDir string, titles ...string) {
	t.Helper()
	conn, err := db.Connect(t.Context(), dataDir)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Release(dataDir)) }()

	sessions := session.NewService(db.New(conn), conn)
	for _, title := range titles {
		_, err := sessions.Create(t.Context(), title)
		require.NoError(t, err)
	}
}

func remainingSessions(t *testing.T, dataDir string) []session.Session {
	t.Helper()
	conn, err := db.Connect(t.Context(), dataDir)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Release(dataDir)) }()

	list, err := session.NewService(db.New(conn), conn).List(t.Context())
	require.NoError(t, err)
	return list
}

func TestSessionPruneDryRunDeletesNothing(t *testing.T) {
	dataDir := t.TempDir()
	seedPruneSessions(t, dataDir, "one", "two", "three")

	c := newPruneTestCmd(t, dataDir)
	require.NoError(t, c.Flags().Set("keep", "1"))
	require.NoError(t, c.Flags().Set("dry-run", "true"))
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	require.Contains(t, out.String(), "2 session(s) would be deleted")
	require.Len(t, remainingSessions(t, dataDir), 3)
}

func TestSessionPruneDeletes(t *testing.T) {
	dataDir := t.TempDir()
	seedPruneSessions(t, dataDir, "one", "two", "three")

	c := newPruneTestCmd(t, dataDir)
	require.NoError(t, c.Flags().Set("keep", "1"))
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	require.Contains(t, out.String(), "Deleted 2 session(s)")
	require.Len(t, remainingSessions(t, dataDir), 1)
}

func TestSessionPruneReportsNothingToDo(t *testing.T) {
	dataDir := t.TempDir()
	seedPruneSessions(t, dataDir, "only one")

	c := newPruneTestCmd(t, dataDir)
	require.NoError(t, c.Flags().Set("older-than", "3650"))
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	require.Contains(t, out.String(), "Nothing to prune.")
}
