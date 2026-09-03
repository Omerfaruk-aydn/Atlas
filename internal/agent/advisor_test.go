package agent

import (
	"strings"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/csync"
	"github.com/stretchr/testify/require"
)

func TestParseAdvisorReplyNone(t *testing.T) {
	severity, note := parseAdvisorReply("NONE: nothing to add")
	require.Equal(t, "NONE", severity)
	require.Equal(t, "nothing to add", note)
}

func TestParseAdvisorReplyEachSeverity(t *testing.T) {
	cases := map[string]string{
		"NIT: minor style thing":         "NIT",
		"CONCERN: missing a null check":  "CONCERN",
		"BLOCKER: this deletes the repo": "BLOCKER",
	}
	for reply, wantSeverity := range cases {
		severity, note := parseAdvisorReply(reply)
		require.Equal(t, wantSeverity, severity)
		require.NotEmpty(t, note)
	}
}

func TestParseAdvisorReplyUnrecognizedIsBlank(t *testing.T) {
	severity, note := parseAdvisorReply("looks fine to me")
	require.Empty(t, severity)
	require.Empty(t, note)
}

func TestParseAdvisorReplyTrimsWhitespace(t *testing.T) {
	severity, note := parseAdvisorReply("  \n CONCERN:   spacing issue  \n")
	require.Equal(t, "CONCERN", severity)
	require.Equal(t, "spacing issue", note)
}

func TestTruncateForAdvisorLeavesShortTextAlone(t *testing.T) {
	require.Equal(t, "hello", truncateForAdvisor("  hello  "))
}

func TestTruncateForAdvisorCutsLongText(t *testing.T) {
	long := strings.Repeat("x", advisorMaxReviewChars*2)
	got := truncateForAdvisor(long)
	require.Less(t, len(got), len(long))
	require.Contains(t, got, "truncated")
}

func TestInjectAdvisorNoteWithNoPendingNote(t *testing.T) {
	a := &sessionAgent{advisorNotes: csync.NewMap[string, string]()}
	require.Equal(t, "do the thing", a.injectAdvisorNote("s1", "do the thing"))
}

func TestInjectAdvisorNotePrependsAndClears(t *testing.T) {
	a := &sessionAgent{advisorNotes: csync.NewMap[string, string]()}
	a.advisorNotes.Set("s1", "Advisor (concern): watch the null check")

	got := a.injectAdvisorNote("s1", "next task")
	require.Equal(t, "Advisor (concern): watch the null check\n\nnext task", got)

	// Read-and-clear: a second call for the same session gets nothing.
	require.Equal(t, "next task 2", a.injectAdvisorNote("s1", "next task 2"))
}

func TestInjectAdvisorNoteIsPerSession(t *testing.T) {
	a := &sessionAgent{advisorNotes: csync.NewMap[string, string]()}
	a.advisorNotes.Set("s1", "note for s1")

	require.Equal(t, "p", a.injectAdvisorNote("s2", "p"), "a note queued for a different session must not leak")
}

func TestInjectAdvisorNoteWithNoPromptUsesTheNoteAlone(t *testing.T) {
	a := &sessionAgent{advisorNotes: csync.NewMap[string, string]()}
	a.advisorNotes.Set("s1", "note")
	require.Equal(t, "note", a.injectAdvisorNote("s1", ""))
}
