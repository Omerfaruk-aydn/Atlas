package rewind

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/maincodss/atlas-agent/internal/db"
	"github.com/maincodss/atlas-agent/internal/history"
	"github.com/maincodss/atlas-agent/internal/message"
	"github.com/maincodss/atlas-agent/internal/session"
	"github.com/stretchr/testify/require"
)

func TestForkAt(t *testing.T) {
	dataDir := t.TempDir()
	t.Cleanup(func() {
		require.NoError(t, db.Release(dataDir))
		db.ResetPool()
	})

	conn, err := db.Connect(t.Context(), dataDir)
	require.NoError(t, err)

	sessions := session.NewService(db.New(conn), conn)
	messages := message.NewService(db.New(conn))
	files := history.NewService(db.New(conn), conn)
	rewind := NewService(sessions, messages, files)

	ctx := t.Context()
	workDir := t.TempDir()
	aPath := filepath.Join(workDir, "a.txt")
	bPath := filepath.Join(workDir, "b.txt")

	src, err := sessions.Create(ctx, "original")
	require.NoError(t, err)

	msg1, err := messages.Create(ctx, src.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "edit a"}},
	})
	require.NoError(t, err)
	_, err = files.Create(ctx, src.ID, aPath, "baseline-a", "")
	require.NoError(t, err)
	_, err = files.CreateVersion(ctx, src.ID, aPath, "a-after-msg1", msg1.ID)
	require.NoError(t, err)

	msg2, err := messages.Create(ctx, src.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "create b"}},
	})
	require.NoError(t, err)
	_, err = files.Create(ctx, src.ID, bPath, "", msg2.ID)
	require.NoError(t, err)
	_, err = files.CreateVersion(ctx, src.ID, bPath, "b-after-msg2", msg2.ID)
	require.NoError(t, err)

	msg3, err := messages.Create(ctx, src.ID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: "done"}},
	})
	require.NoError(t, err)

	// Simulate the working directory currently reflecting the LATEST state
	// (post msg3, i.e. post msg2): both files exist.
	require.NoError(t, os.WriteFile(aPath, []byte("a-after-msg1"), 0o644))
	require.NoError(t, os.WriteFile(bPath, []byte("b-after-msg2"), 0o644))

	result, err := rewind.ForkAt(ctx, src.ID, msg1.ID)
	require.NoError(t, err)

	t.Run("forked session is a child and does not touch the source", func(t *testing.T) {
		require.Equal(t, src.ID, result.Session.ParentSessionID)
		require.NotEqual(t, src.ID, result.Session.ID)

		stillThere, err := sessions.Get(ctx, src.ID)
		require.NoError(t, err)
		require.Equal(t, src.ID, stillThere.ID)
	})

	t.Run("forked session contains only messages up to and including msg1", func(t *testing.T) {
		copied, err := messages.List(ctx, result.Session.ID)
		require.NoError(t, err)
		require.Len(t, copied, 1)
		require.Equal(t, "edit a", copied[0].Content().Text)
	})

	t.Run("source session is untouched", func(t *testing.T) {
		original, err := messages.List(ctx, src.ID)
		require.NoError(t, err)
		require.Len(t, original, 3)
		_ = msg3
	})

	t.Run("disk reflects state as of msg1: a restored, b deleted", func(t *testing.T) {
		aContent, err := os.ReadFile(aPath)
		require.NoError(t, err)
		require.Equal(t, "a-after-msg1", string(aContent))

		_, err = os.Stat(bPath)
		require.True(t, os.IsNotExist(err), "b.txt did not exist as of msg1 and must be deleted")

		require.Equal(t, 1, result.FilesWritten)
		require.Equal(t, 1, result.FilesDeleted)
	})
}
