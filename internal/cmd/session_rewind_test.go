package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/db"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/history"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/message"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/session"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func newRewindTestCmd(t *testing.T, dataDir string) *cobra.Command {
	t.Helper()
	c := &cobra.Command{RunE: runSessionRewind}
	c.Flags().String("data-dir", dataDir, "")
	c.Flags().BoolVar(&sessionRewindApply, "apply", false, "")
	c.SetContext(t.Context())
	t.Cleanup(func() { sessionRewindApply = false })
	return c
}

// seedRewindSession builds a two-message session with a file history entry
// on the first message, mirroring internal/session/rewind's own test setup,
// so ForkAt has something real to preview and apply.
func seedRewindSession(t *testing.T, dataDir, workDir string) (session.Session, string) {
	t.Helper()

	conn, err := db.Connect(t.Context(), dataDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	q := db.New(conn)
	sessions := session.NewService(q, conn)
	messages := message.NewService(q)
	files := history.NewService(q, conn)

	sess, err := sessions.Create(t.Context(), "rewind test")
	require.NoError(t, err)

	msg1, err := messages.Create(t.Context(), sess.ID, message.CreateMessageParams{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: "edit a"}},
	})
	require.NoError(t, err)

	aPath := filepath.Join(workDir, "a.txt")
	_, err = files.Create(t.Context(), sess.ID, aPath, "baseline", "")
	require.NoError(t, err)
	_, err = files.CreateVersion(t.Context(), sess.ID, aPath, "after-msg1", msg1.ID)
	require.NoError(t, err)

	_, err = messages.Create(t.Context(), sess.ID, message.CreateMessageParams{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: "done"}},
	})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(aPath, []byte("after-msg1"), 0o644))

	return sess, msg1.ID
}

func TestSessionRewindPreviewDoesNotTouchFiles(t *testing.T) {
	dataDir := t.TempDir()
	workDir := t.TempDir()
	sess, msg1ID := seedRewindSession(t, dataDir, workDir)

	c := newRewindTestCmd(t, dataDir)
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, []string{sess.ID, msg1ID}))
	require.Contains(t, out.String(), "Would write")
	require.Contains(t, out.String(), "--apply")
	require.NotContains(t, out.String(), "Forked into session", "a preview must not fork")
}

func TestSessionRewindApplyForksAndRestores(t *testing.T) {
	dataDir := t.TempDir()
	workDir := t.TempDir()
	sess, msg1ID := seedRewindSession(t, dataDir, workDir)

	c := newRewindTestCmd(t, dataDir)
	require.NoError(t, c.Flags().Set("apply", "true"))
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, []string{sess.ID, msg1ID}))
	require.Contains(t, out.String(), "Forked into session")
	require.NotContains(t, out.String(), session.HashID(sess.ID),
		"the forked session must be a new one, not the source session")
}

func TestSessionRewindUnknownMessage(t *testing.T) {
	dataDir := t.TempDir()
	workDir := t.TempDir()
	sess, _ := seedRewindSession(t, dataDir, workDir)

	c := newRewindTestCmd(t, dataDir)
	require.Error(t, c.RunE(c, []string{sess.ID, "not-a-real-message"}))
}
