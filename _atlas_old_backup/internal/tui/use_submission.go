package tui

import (
	"sync"
	"time"
)

// SubmissionPolicy is the public surface of the useSubmission hook.
// It captures Hermes's queue/steer/interrupt three-way policy and
// exposes a single Submit() entry point that all of the App's
// submission paths go through. The Atlas port implements queue fully
// (the existing queuedMessages slice) and records the chosen mode
// so steer/interrupt can be promoted to full behavior without
// changing the call sites.
type SubmissionPolicy struct {
	mu   sync.Mutex
	mode BusyMode

	// queue is the Atlas MessageQueue. Atlas currently still uses
	// the legacy []string field on the App for the rendering layer
	// (so existing tests don't break); the queue is the principled
	// long-term home.
	queue *MessageQueue
}

// newSubmissionPolicy returns a policy in queue mode with an
// associated MessageQueue. The defaults match Hermes's TUI default
// (queue is what the TUI overrides any other setting to).
func newSubmissionPolicy() *SubmissionPolicy {
	return &SubmissionPolicy{
		mode:  BusyQueue,
		queue: &MessageQueue{},
	}
}

// SetMode records the active mode.
func (p *SubmissionPolicy) SetMode(m BusyMode) {
	p.mu.Lock()
	p.mode = m
	p.mu.Unlock()
}

// Mode returns the active mode.
func (p *SubmissionPolicy) Mode() BusyMode {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.mode == "" {
		return BusyQueue
	}
	return p.mode
}

// submit is the public entry point. The App calls this with the
// text the user wants to send; the policy returns the disposition:
//
//	"submit"     — start a new turn now (interrupt or no current turn)
//	"queue"      — append to the queue, drain after current turn
//	"steer"      — inject after the next tool boundary (Atlas no-op today;
//	               recorded as a "queued" entry tagged with the intent so
//	               a future wiring can fire at the right moment)
//
// In every case, the policy updates the busy mode flag the App can
// use to swap the input's behavior.
func (p *SubmissionPolicy) submit(text string, isStreaming bool) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !isStreaming {
		return "submit"
	}
	switch p.mode {
	case BusyInterrupt:
		return "submit" // App's startTurn will cancel the running one
	case BusySteer:
		// "Steer" semantically means: don't start a new turn now,
		// inject after the next tool call. The Atlas agent today
		// doesn't expose a per-tool-boundary hook, so this collapses
		// to "queue" — but the intent is recorded on the queue item
		// for a future post-tool drain.
		p.queue.Enqueue(text, "[steer] "+truncateForNote(text))
		return "queue"
	default: // BusyQueue
		p.queue.Enqueue(text, truncateForNote(text))
		return "queue"
	}
}

// submitDirect is the "-q startup query" path. Mirrors Hermes's
// `skipDetectDrop` parameter: the text is submitted verbatim with
// no drop-detection scan, no shell-metacharacter escaping, no
// paste-detection.
func (p *SubmissionPolicy) submitDirect(text string) string {
	return "submit-direct"
}

// IsQueuedForSteer reports whether the most-recently-enqueued item
// is a steer-mode entry (for future wiring).
func (p *SubmissionPolicy) IsQueuedForSteer() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	items := p.queue.Items()
	if len(items) == 0 {
		return false
	}
	last := items[len(items)-1]
	return len(last.Display) > 9 && last.Display[:9] == "[steer] "
}

// QueueLength returns the current queue depth.
func (p *SubmissionPolicy) QueueLength() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.queue.Len()
}

// QueueItems returns a copy of the queue for safe iteration.
func (p *SubmissionPolicy) QueueItems() []QueueItem {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.queue.Items()
}

// DrainQueue removes and returns the head item (FIFO).
func (p *SubmissionPolicy) DrainQueue() (QueueItem, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.queue.Dequeue()
}

// UseSubmissionState is the legacy "submission busy" gate that
// predates SubmissionPolicy. Kept as a small shim so the App's
// existing IsBusy / TryClaim / Release calls don't need to change.
type UseSubmissionState struct {
	gate *submissionState
}

func newUseSubmissionState() *UseSubmissionState {
	return &UseSubmissionState{gate: &submissionState{}}
}

// TryClaim wraps submissionState.TryClaim.
func (u *UseSubmissionState) TryClaim() error { return u.gate.TryClaim() }

// Release wraps submissionState.Release.
func (u *UseSubmissionState) Release() { u.gate.Release() }

// MarkSessionReady wraps submissionState.SetSessionReady.
func (u *UseSubmissionState) MarkSessionReady() { u.gate.SetSessionReady() }

// IsBusy wraps submissionState.IsBusy.
func (u *UseSubmissionState) IsBusy() bool { return u.gate.IsBusy() }

// setIdleDelay is a small helper to record the last-input wall clock
// — used by the App to drive the TYPING_IDLE_MS cooldown for the
// stream delay decay.
var lastInputAtMu sync.Mutex
var lastInputAt time.Time

// NoteInput records the wall clock of a user input event. The App
// calls this from the key handler so the SubmissionPolicy (or any
// other consumer) can know when the user was last typing.
func NoteInput() {
	lastInputAtMu.Lock()
	lastInputAt = time.Now()
	lastInputAtMu.Unlock()
}

// LastInputAt returns the wall clock of the most recent input event,
// or the zero time if no input has been recorded.
func LastInputAt() time.Time {
	lastInputAtMu.Lock()
	defer lastInputAtMu.Unlock()
	return lastInputAt
}
