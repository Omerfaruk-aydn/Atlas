package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/session"
	"github.com/stretchr/testify/require"
)

func childrenFrom(m map[string][]session.Session) childrenFunc {
	return func(_ context.Context, parentID string) ([]session.Session, error) {
		return m[parentID], nil
	}
}

func TestSessionTreeWithNoSessions(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, writeSessionTree(t.Context(), &out, nil, childrenFrom(nil)))
	require.Contains(t, out.String(), "No sessions found.")
}

func TestSessionTreeNestsChildrenUnderTheirParent(t *testing.T) {
	root := session.Session{ID: "root", Title: "the task"}
	child := session.Session{ID: "child", Title: "a sub-agent"}
	grandchild := session.Session{ID: "grandchild", Title: "deeper still"}

	var out bytes.Buffer
	require.NoError(t, writeSessionTree(t.Context(), &out, []session.Session{root}, childrenFrom(map[string][]session.Session{
		"root":  {child},
		"child": {grandchild},
	})))

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	require.Len(t, lines, 3)
	require.Contains(t, lines[0], "the task")
	require.False(t, strings.HasPrefix(lines[0], " "), "a root is not indented")
	require.Contains(t, lines[1], "a sub-agent")
	require.True(t, strings.HasPrefix(lines[1], "  "))
	require.Contains(t, lines[2], "deeper still")
	require.True(t, strings.HasPrefix(lines[2], "    "))
}

// A parent cycle -- which an older build could have written -- must not
// print forever.
func TestSessionTreeStopsAtTheDepthLimit(t *testing.T) {
	self := session.Session{ID: "loop", Title: "cycle"}

	var out bytes.Buffer
	require.NoError(t, writeSessionTree(t.Context(), &out, []session.Session{self}, childrenFrom(map[string][]session.Session{
		"loop": {self},
	})))

	require.Contains(t, out.String(), "deeper sessions not shown")
	require.Equal(t, maxSessionTreeDepth+1, strings.Count(out.String(), "cycle"))
}

func TestSessionTreeReportsAChildLookupFailure(t *testing.T) {
	var out bytes.Buffer
	err := writeSessionTree(t.Context(), &out, []session.Session{{ID: "root"}},
		func(context.Context, string) ([]session.Session, error) {
			return nil, errors.New("database is gone")
		})
	require.Error(t, err)
	require.Contains(t, err.Error(), "database is gone")
}

// A newline in a title would otherwise break the one-session-per-line shape
// the indentation depends on.
func TestSessionTreeFlattensMultilineTitles(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, writeSessionTree(t.Context(), &out,
		[]session.Session{{ID: "root", Title: "first\nsecond"}}, childrenFrom(nil)))

	require.Equal(t, 1, strings.Count(strings.TrimSpace(out.String()), "\n")+1)
	require.Contains(t, out.String(), "first second")
}
