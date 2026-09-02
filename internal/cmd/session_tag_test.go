package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/db"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/session"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestNormalizeTagsDropsBlanksAndDuplicates(t *testing.T) {
	require.Equal(t, []string{"a", "b"}, normalizeTags([]string{" a ", "b", "a", "", "  "}))
}

func TestFilterSessionsByTagIsExactMatch(t *testing.T) {
	sessions := []session.Session{
		{ID: "one", Tags: []string{"wip"}},
		{ID: "two", Tags: []string{"wip-2"}},
		{ID: "three", Tags: []string{"done", "wip"}},
	}
	got := filterSessionsByTag(sessions, "wip")
	require.Len(t, got, 2)
	require.Equal(t, "one", got[0].ID)
	require.Equal(t, "three", got[1].ID)
}

func newSessionTagTestCmd(t *testing.T, dataDir string) *cobra.Command {
	t.Helper()
	c := &cobra.Command{RunE: runSessionTag}
	c.Flags().String("data-dir", dataDir, "")
	c.Flags().BoolVar(&sessionTagJSON, "json", false, "")
	c.Flags().BoolVar(&sessionTagClear, "clear", false, "")
	c.SetContext(t.Context())
	t.Cleanup(func() {
		sessionTagJSON = false
		sessionTagClear = false
	})
	return c
}

func seedTaggedSession(t *testing.T, dataDir, title string) string {
	t.Helper()
	conn, err := db.Connect(t.Context(), dataDir)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Release(dataDir)) }()

	sess, err := session.NewService(db.New(conn), conn).Create(t.Context(), title)
	require.NoError(t, err)
	return sess.ID
}

func TestSessionTagShowsNoTagsInitially(t *testing.T) {
	dataDir := t.TempDir()
	id := seedTaggedSession(t, dataDir, "s")

	c := newSessionTagTestCmd(t, dataDir)
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, []string{id}))
	require.Contains(t, out.String(), "no tags")
}

func TestSessionTagSetAndShow(t *testing.T) {
	dataDir := t.TempDir()
	id := seedTaggedSession(t, dataDir, "s")

	c := newSessionTagTestCmd(t, dataDir)
	var out bytes.Buffer
	c.SetOut(&out)
	require.NoError(t, c.RunE(c, []string{id, "urgent", "billing"}))
	require.Contains(t, out.String(), "urgent, billing")

	c2 := newSessionTagTestCmd(t, dataDir)
	var out2 bytes.Buffer
	c2.SetOut(&out2)
	require.NoError(t, c2.RunE(c2, []string{id}))
	require.Contains(t, out2.String(), "urgent, billing")
}

// Setting tags replaces the list wholesale -- the same shape as rename --
// rather than appending to whatever was there before.
func TestSessionTagReplacesWholesale(t *testing.T) {
	dataDir := t.TempDir()
	id := seedTaggedSession(t, dataDir, "s")

	c := newSessionTagTestCmd(t, dataDir)
	require.NoError(t, c.RunE(c, []string{id, "a", "b"}))

	c2 := newSessionTagTestCmd(t, dataDir)
	require.NoError(t, c2.Flags().Set("json", "true"))
	var out bytes.Buffer
	c2.SetOut(&out)
	require.NoError(t, c2.RunE(c2, []string{id, "c"}))

	var got sessionTagResult
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	require.Equal(t, []string{"c"}, got.Tags)
}

func TestSessionTagClear(t *testing.T) {
	dataDir := t.TempDir()
	id := seedTaggedSession(t, dataDir, "s")

	c := newSessionTagTestCmd(t, dataDir)
	require.NoError(t, c.RunE(c, []string{id, "a"}))

	c2 := newSessionTagTestCmd(t, dataDir)
	require.NoError(t, c2.Flags().Set("clear", "true"))
	var out bytes.Buffer
	c2.SetOut(&out)
	require.NoError(t, c2.RunE(c2, []string{id}))
	require.Contains(t, out.String(), "no tags")
}

func TestSessionTagJSON(t *testing.T) {
	dataDir := t.TempDir()
	id := seedTaggedSession(t, dataDir, "s")

	c := newSessionTagTestCmd(t, dataDir)
	require.NoError(t, c.Flags().Set("json", "true"))
	var out bytes.Buffer
	c.SetOut(&out)
	require.NoError(t, c.RunE(c, []string{id, "urgent"}))

	var got sessionTagResult
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	require.Equal(t, []string{"urgent"}, got.Tags)
	require.Equal(t, id, got.UUID)
}

func TestSessionTagUnknownSession(t *testing.T) {
	c := newSessionTagTestCmd(t, t.TempDir())
	err := c.RunE(c, []string{"no-such-session"})
	require.Error(t, err)
}
