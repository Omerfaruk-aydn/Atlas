package debugger

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeSession is a closer double for testing Manager's bookkeeping without
// launching a real dlv process.
type fakeSession struct {
	closed bool
}

func (f *fakeSession) Close() { f.closed = true }

func newTestManager() (*Manager[*fakeSession], *int) {
	created := 0
	m := newManager[*fakeSession](Options{}, func(context.Context, Options, string, []string) (*fakeSession, error) {
		created++
		return &fakeSession{}, nil
	})
	return m, &created
}

func TestManagerStartLaunchesASessionForANewID(t *testing.T) {
	t.Parallel()
	m, created := newTestManager()

	s, err := m.Start(t.Context(), "chat-1", "prog", nil)
	require.NoError(t, err)
	require.NotNil(t, s)
	require.Equal(t, 1, *created)
}

func TestManagerStartClosesAnExistingSessionForTheSameID(t *testing.T) {
	t.Parallel()
	m, created := newTestManager()

	first, err := m.Start(t.Context(), "chat-1", "prog", nil)
	require.NoError(t, err)

	second, err := m.Start(t.Context(), "chat-1", "prog2", nil)
	require.NoError(t, err)

	require.True(t, first.closed, "starting a new session for the same chat should close the old one")
	require.False(t, second.closed)
	require.Equal(t, 2, *created)
}

func TestManagerGetReturnsTheOpenSession(t *testing.T) {
	t.Parallel()
	m, _ := newTestManager()

	started, err := m.Start(t.Context(), "chat-1", "prog", nil)
	require.NoError(t, err)

	got, ok := m.Get("chat-1")
	require.True(t, ok)
	require.Same(t, started, got)
}

func TestManagerGetReportsNothingForAnUnknownID(t *testing.T) {
	t.Parallel()
	m, _ := newTestManager()

	_, ok := m.Get("never-started")
	require.False(t, ok)
}

func TestManagerCloseClosesAndDropsTheSession(t *testing.T) {
	t.Parallel()
	m, _ := newTestManager()

	s, err := m.Start(t.Context(), "chat-1", "prog", nil)
	require.NoError(t, err)

	m.Close("chat-1")
	require.True(t, s.closed)

	_, ok := m.Get("chat-1")
	require.False(t, ok)
}

func TestManagerCloseOnUnknownIDIsANoop(t *testing.T) {
	t.Parallel()
	m, _ := newTestManager()
	m.Close("never-started") // must not panic
}

func TestManagerCloseAllClosesEverySession(t *testing.T) {
	t.Parallel()
	m, _ := newTestManager()

	s1, err := m.Start(t.Context(), "chat-1", "prog", nil)
	require.NoError(t, err)
	s2, err := m.Start(t.Context(), "chat-2", "prog", nil)
	require.NoError(t, err)

	m.CloseAll()

	require.True(t, s1.closed)
	require.True(t, s2.closed)
	_, ok := m.Get("chat-1")
	require.False(t, ok)
	_, ok = m.Get("chat-2")
	require.False(t, ok)
}

func TestManagerStartPropagatesLaunchErrors(t *testing.T) {
	t.Parallel()
	wantErr := fmt.Errorf("dlv not found")
	m := newManager[*fakeSession](Options{}, func(context.Context, Options, string, []string) (*fakeSession, error) {
		return nil, wantErr
	})

	_, err := m.Start(t.Context(), "chat-1", "prog", nil)
	require.ErrorIs(t, err, wantErr)

	_, ok := m.Get("chat-1")
	require.False(t, ok, "a failed start must not register a session")
}
