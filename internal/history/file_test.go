package history

import (
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/db"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/session"
	"github.com/stretchr/testify/require"
)

// TestResolveAsOf pins the off-by-one-sensitive selection semantics
// documented on Service.ResolveAsOf: for each path, the highest-version
// entry whose MessageID is in the allowed set wins; failing that, the
// version-0 baseline (no MessageID) wins; failing that, the path is
// reported for deletion (Content == nil).
func TestResolveAsOf(t *testing.T) {
	dataDir := t.TempDir()
	t.Cleanup(func() {
		require.NoError(t, db.Release(dataDir))
		db.ResetPool()
	})

	conn, err := db.Connect(t.Context(), dataDir)
	require.NoError(t, err)

	sessions := session.NewService(db.New(conn), conn)
	sess, err := sessions.Create(t.Context(), "test")
	require.NoError(t, err)

	files := NewService(db.New(conn), conn)
	ctx := t.Context()

	// Baseline for A, predates any message (genuine pre-chat content).
	_, err = files.Create(ctx, sess.ID, "a.txt", "baseline-a", "")
	require.NoError(t, err)

	// msg1: edit A.
	_, err = files.CreateVersion(ctx, sess.ID, "a.txt", "a-after-msg1", "msg1")
	require.NoError(t, err)

	// msg2: create B fresh (brand-new file placeholder tagged with msg2,
	// mirroring how write.go/edit.go/multiedit.go tag a new-file
	// placeholder with the creating message rather than leaving it bare).
	_, err = files.Create(ctx, sess.ID, "b.txt", "", "msg2")
	require.NoError(t, err)
	_, err = files.CreateVersion(ctx, sess.ID, "b.txt", "b-after-msg2", "msg2")
	require.NoError(t, err)

	// msg3: edit A again. B untouched.
	_, err = files.CreateVersion(ctx, sess.ID, "a.txt", "a-after-msg3", "msg3")
	require.NoError(t, err)

	t.Run("as of msg1: A is msg1 content, B does not exist yet", func(t *testing.T) {
		resolved, err := files.ResolveAsOf(ctx, sess.ID, []string{"msg1"})
		require.NoError(t, err)
		byPath := indexByPath(resolved)

		require.NotNil(t, byPath["a.txt"])
		require.Equal(t, "a-after-msg1", *byPath["a.txt"])

		require.Contains(t, byPath, "b.txt")
		require.Nil(t, byPath["b.txt"], "b.txt was created after msg1, must resolve to delete")
	})

	t.Run("as of msg2: A still msg1 content (not re-touched), B is msg2 content", func(t *testing.T) {
		resolved, err := files.ResolveAsOf(ctx, sess.ID, []string{"msg1", "msg2"})
		require.NoError(t, err)
		byPath := indexByPath(resolved)

		require.NotNil(t, byPath["a.txt"])
		require.Equal(t, "a-after-msg1", *byPath["a.txt"])

		require.NotNil(t, byPath["b.txt"])
		require.Equal(t, "b-after-msg2", *byPath["b.txt"])
	})

	t.Run("as of msg3: A is msg3 content, B is still msg2 content", func(t *testing.T) {
		resolved, err := files.ResolveAsOf(ctx, sess.ID, []string{"msg1", "msg2", "msg3"})
		require.NoError(t, err)
		byPath := indexByPath(resolved)

		require.NotNil(t, byPath["a.txt"])
		require.Equal(t, "a-after-msg3", *byPath["a.txt"])

		require.NotNil(t, byPath["b.txt"])
		require.Equal(t, "b-after-msg2", *byPath["b.txt"])
	})

	t.Run("before any message: A is baseline, B does not exist", func(t *testing.T) {
		resolved, err := files.ResolveAsOf(ctx, sess.ID, nil)
		require.NoError(t, err)
		byPath := indexByPath(resolved)

		require.NotNil(t, byPath["a.txt"])
		require.Equal(t, "baseline-a", *byPath["a.txt"])

		require.Contains(t, byPath, "b.txt")
		require.Nil(t, byPath["b.txt"])
	})
}

func indexByPath(resolved []ResolvedFile) map[string]*string {
	m := make(map[string]*string, len(resolved))
	for _, r := range resolved {
		m[r.Path] = r.Content
	}
	return m
}
