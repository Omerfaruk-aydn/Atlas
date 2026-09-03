package agent

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/agent/notify"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/pubsub"
)

// advisorTimeout bounds how long one advisor pass may run. It reviews a
// turn that has already finished, so nothing is waiting on it but the next
// prompt's note; a hung advisor model must not accumulate goroutines
// forever across a long session.
const advisorTimeout = 45 * time.Second

// advisorMaxReviewChars truncates the prompt and response handed to the
// advisor. Reviewing is meant to be a quick second look, not a full replay
// of the turn's context -- and an unbounded transcript would make the
// advisor's own cost comparable to the turn it is reviewing.
const advisorMaxReviewChars = 4000

const advisorSystemPrompt = `You are a second reviewer watching another AI agent's coding work. You are given the ` +
	`user's request and the agent's final response for one completed turn, and you have read-only tools ` +
	`(glob, grep, ls, view) to inspect the resulting code yourself.

Reply with exactly one line, starting with one of these prefixes:
NONE: -- nothing worth flagging.
NIT: <note> -- a minor observation, not worth interrupting anyone over.
CONCERN: <note> -- something worth a human's attention soon.
BLOCKER: <note> -- something that should be addressed before the work continues.

Keep <note> to one or two sentences. Do not use tools unless the response actually claims something ` +
	`about the code that is worth checking against it.`

// advisorSeverityRank orders severities for the notify-threshold
// comparison. NONE is deliberately absent: it never reaches this check,
// since runAdvisorPass returns before queuing or notifying on NONE.
var advisorSeverityRank = map[string]int{"NIT": 1, "CONCERN": 2, "BLOCKER": 3}

// advisorShouldNotify reports whether severity meets or exceeds threshold
// (an unrecognized or empty threshold falls back to CONCERN, the
// advisor's original, pre-configurable behavior).
func advisorShouldNotify(severity, threshold string) bool {
	if _, ok := advisorSeverityRank[threshold]; !ok {
		threshold = "CONCERN"
	}
	return advisorSeverityRank[severity] >= advisorSeverityRank[threshold]
}

// shouldRunAdvisorPass reports whether the turn that just finished for
// sessionID falls on the configured review cadence (config.Advisor's
// EveryNTurns), advancing that session's turn counter as a side effect.
// Every call counts a turn even when it declines to review, so "every 3rd
// turn" means turns 3, 6, 9, ... rather than resetting whenever a review
// is skipped.
func (a *sessionAgent) shouldRunAdvisorPass(sessionID string) bool {
	interval := a.advisorEveryNTurns
	if interval <= 0 {
		interval = 1
	}

	count, _ := a.advisorTurnCounts.Get(sessionID)
	count++
	a.advisorTurnCounts.Set(sessionID, count)

	return count%interval == 0
}

// injectAdvisorNote pops any note the advisor left for sessionID (from the
// turn that just finished) and prepends it to prompt. Read-and-clear: a
// note is delivered to the very next prompt only, not repeated.
func (a *sessionAgent) injectAdvisorNote(sessionID, prompt string) string {
	note, ok := a.advisorNotes.Take(sessionID)
	if !ok || note == "" {
		return prompt
	}
	if prompt == "" {
		return note
	}
	return note + "\n\n" + prompt
}

// runAdvisorPass reviews one finished turn and, if the advisor flags
// anything above NONE, queues a note for the session's next prompt (see
// injectAdvisorNote) and, for CONCERN/BLOCKER, publishes a
// notify.TypeAdvisorNote notification so it surfaces before then too.
//
// Errors are logged and otherwise swallowed: this runs in the background
// after the turn it reviews has already completed, so there is no request
// left for an advisor failure to fail.
func (a *sessionAgent) runAdvisorPass(ctx context.Context, sessionID, userPrompt, assistantText string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Advisor pass panicked", "session_id", sessionID, "panic", r)
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, advisorTimeout)
	defer cancel()

	agent := fantasy.NewAgent(
		a.advisorModel.Model,
		fantasy.WithSystemPrompt(advisorSystemPrompt),
		fantasy.WithTools(a.advisorTools...),
		fantasy.WithUserAgent(userAgent),
	)

	review := "User asked:\n" + truncateForAdvisor(userPrompt) +
		"\n\nAgent responded:\n" + truncateForAdvisor(assistantText)

	result, err := agent.Stream(ctx, fantasy.AgentStreamCall{Prompt: review})
	if err != nil {
		slog.Warn("Advisor pass failed", "session_id", sessionID, "error", err)
		return
	}
	if result == nil {
		return
	}

	severity, note := parseAdvisorReply(result.Response.Content.Text())
	if severity == "" || severity == "NONE" {
		return
	}

	a.advisorNotes.Set(sessionID, "Advisor ("+strings.ToLower(severity)+"): "+note)

	if !advisorShouldNotify(severity, a.advisorNotifyThreshold) {
		// Below the configured floor: queued for the next prompt above,
		// but not worth interrupting the session over.
		slog.Info("Advisor left a note", "session_id", sessionID, "severity", severity, "note", note)
		return
	}

	slog.Warn("Advisor flagged the last turn", "session_id", sessionID, "severity", severity, "note", note)
	if a.notify != nil {
		a.notify.Publish(pubsub.CreatedEvent, notify.Notification{
			SessionID: sessionID,
			Type:      notify.TypeAdvisorNote,
			Message:   note,
		})
	}
}

// parseAdvisorReply extracts the severity prefix and note from the
// advisor's one-line reply. An unrecognized or empty reply is treated as
// NONE rather than guessed at: a reviewer that did not follow the format
// is not one to trust with a stronger signal.
func parseAdvisorReply(reply string) (severity, note string) {
	reply = strings.TrimSpace(reply)
	for _, prefix := range []string{"NONE:", "NIT:", "CONCERN:", "BLOCKER:"} {
		if rest, ok := strings.CutPrefix(reply, prefix); ok {
			return strings.TrimSuffix(prefix, ":"), strings.TrimSpace(rest)
		}
	}
	return "", ""
}

func truncateForAdvisor(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= advisorMaxReviewChars {
		return s
	}
	return s[:advisorMaxReviewChars] + "… (truncated)"
}
