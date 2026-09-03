package teams

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestJoinPutsParentAndChildInTheSameTeam(t *testing.T) {
	r := NewRegistry()
	team := r.Join("parent", "child")
	require.Equal(t, "parent", team)
	require.Equal(t, team, r.TeamFor("parent"))
	require.Equal(t, team, r.TeamFor("child"))
}

func TestJoinIsTransitiveAcrossNestedSubAgents(t *testing.T) {
	r := NewRegistry()
	root := r.Join("root", "child")
	// A sub-agent that itself spawns a sub-agent passes its own session
	// ID as parentID -- the grandchild must land in the same team as the
	// original root, not start a new one.
	got := r.Join("child", "grandchild")
	require.Equal(t, root, got)
	require.Equal(t, root, r.TeamFor("grandchild"))
}

func TestTeamForRegistersAnUnseenSessionAsItsOwnRoot(t *testing.T) {
	r := NewRegistry()
	require.Equal(t, "solo", r.TeamFor("solo"))
}

func TestSendAssignsIncreasingSequenceNumbersPerTeam(t *testing.T) {
	r := NewRegistry()
	m1 := r.Send("team-a", "alice", "hello")
	m2 := r.Send("team-a", "bob", "hi")
	require.Equal(t, 1, m1.Seq)
	require.Equal(t, 2, m2.Seq)

	// A different team gets its own independent counter.
	m3 := r.Send("team-b", "carol", "hey")
	require.Equal(t, 1, m3.Seq)
}

func TestSinceOnlyReturnsMessagesAfterTheGivenSequence(t *testing.T) {
	r := NewRegistry()
	r.Send("team-a", "alice", "one")
	r.Send("team-a", "alice", "two")
	r.Send("team-a", "alice", "three")

	got := r.Since("team-a", 1)
	require.Len(t, got, 2)
	require.Equal(t, "two", got[0].Text)
	require.Equal(t, "three", got[1].Text)
}

func TestSinceOnAnUnknownTeamReturnsEmpty(t *testing.T) {
	r := NewRegistry()
	require.Empty(t, r.Since("nobody-here", 0))
}

func TestWaitReturnsImmediatelyWhenAMessageIsAlreadyAvailable(t *testing.T) {
	r := NewRegistry()
	r.Send("team-a", "alice", "hello")

	start := time.Now()
	got := r.Wait(context.Background(), "team-a", 0, 5*time.Second)
	require.Less(t, time.Since(start), time.Second)
	require.Len(t, got, 1)
}

func TestWaitReturnsEmptyImmediatelyWhenTimeoutIsZero(t *testing.T) {
	r := NewRegistry()
	start := time.Now()
	got := r.Wait(context.Background(), "team-a", 0, 0)
	require.Less(t, time.Since(start), time.Second)
	require.Empty(t, got)
}

// This is the actual point of the feature: one goroutine (a sub-agent
// still running) blocks in Wait, another goroutine (a sibling sub-agent
// running in parallel) sends a message, and the waiter must wake up with
// it well before its timeout -- not merely after it, which a broken
// broadcast (or none at all) would still pass under a long enough sleep.
func TestWaitWakesUpAssoonAsASiblingSends(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup
	var got []Message
	start := time.Now()

	wg.Add(1)
	go func() {
		defer wg.Done()
		got = r.Wait(context.Background(), "team-a", 0, 10*time.Second)
	}()

	time.Sleep(50 * time.Millisecond)
	r.Send("team-a", "sibling", "found it")
	wg.Wait()

	require.Less(t, time.Since(start), 5*time.Second)
	require.Len(t, got, 1)
	require.Equal(t, "found it", got[0].Text)
}

func TestWaitGivesUpWhenContextIsCanceled(t *testing.T) {
	r := NewRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	got := r.Wait(ctx, "team-a", 0, 10*time.Second)
	require.Less(t, time.Since(start), 5*time.Second)
	require.Empty(t, got)
}

func TestWaitTimesOutWhenNothingArrives(t *testing.T) {
	r := NewRegistry()
	start := time.Now()
	got := r.Wait(context.Background(), "team-a", 0, 100*time.Millisecond)
	elapsed := time.Since(start)
	require.GreaterOrEqual(t, elapsed, 100*time.Millisecond)
	require.Less(t, elapsed, 5*time.Second)
	require.Empty(t, got)
}

// Multiple waiters on the same team must all wake up from one send --
// the waiters map entry is deleted on send, so a second concurrent
// waiter must not get stuck waiting on an already-closed channel nobody
// will ever replace.
func TestMultipleWaitersOnTheSameTeamAllWakeUp(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup
	results := make([][]Message, 3)

	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = r.Wait(context.Background(), "team-a", 0, 10*time.Second)
		}(i)
	}

	time.Sleep(50 * time.Millisecond)
	r.Send("team-a", "sibling", "broadcast")
	wg.Wait()

	for _, got := range results {
		require.Len(t, got, 1)
		require.Equal(t, "broadcast", got[0].Text)
	}
}
