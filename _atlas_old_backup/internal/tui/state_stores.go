package tui

import (
	"sync"
	"time"
)

// turnStore is the per-turn state slice. It holds everything that
// should reset when a new turn begins — streaming buffer, tool trail,
// reasoning marker, todos, activity feed — and survives across
// render ticks. Mirrors Hermes's `turnStore` (nanostore in the
// original; Atlas uses a plain struct with a mutex).
//
// The split exists so a turn ending (`resetTurnState`) never touches
// session-level UI prefs (which live in uiStore), and so subscribers
// (the App) can re-render narrowly on a per-field change rather than
// the whole-tree.
type turnStore struct {
	mu sync.Mutex

	// Streaming / response text.
	streamingBuf string
	finalText    string

	// Reasoning marker (Hermes pulses this while the model is
	// thinking).
	reasoningActive bool
	reasoningBuf    string

	// Tool trail (capped at TRAIL_LIMIT=8).
	trail []trailEntry

	// Live todo tree (Atlas doesn't ship todos yet; the field is
	// here for the future /todos command).
	todos []TodoItem

	// Activity feed (capped at ACTIVITY_LIMIT=8).
	activity []activityEntry

	// Subagent tree (Atlas doesn't ship subagents; the field is
	// here for the future /subagents overlay).
	subagents []subagentNode

	// Turn-scoped notice held while a turn is busy; flushed at
	// the three real turn-end sites.
	pendingNotice *Notice

	// Was this turn interrupted? Hermes uses this to append a
	// `[interrupted]` marker to the transcript.
	wasInterrupted bool

	// Final messages after dedup against the streaming text.
	finalMessages []string
}

// TodoItem is one node in the live todo tree. Atlas doesn't emit
// todos today, but the field exists so a future /todos overlay can
// drop in without architectural change.
type TodoItem struct {
	ID       string
	Parent   string
	Content  string
	Status   string // "pending" | "in_progress" | "completed"
	Depth    int
	Index    int
}

// patch atomically updates the turn store. The mutator runs under
// the store's lock so a concurrent reader (the App's render thread)
// sees a consistent snapshot.
func (t *turnStore) patch(mutate func(s *turnStore)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	mutate(t)
}

// snapshot returns a copy of the current state. Cheap; called from
// the App's render path on every tick.
func (t *turnStore) snapshot() turnStoreSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return turnStoreSnapshot{
		StreamingBuf:    t.streamingBuf,
		FinalText:       t.finalText,
		ReasoningActive: t.reasoningActive,
		ReasoningBuf:    t.reasoningBuf,
		Trail:           append([]trailEntry(nil), t.trail...),
		Todos:           append([]TodoItem(nil), t.todos...),
		Activity:        append([]activityEntry(nil), t.activity...),
	}
}

// turnStoreSnapshot is the immutable view of the turn store the
// renderer reads. New fields added here don't require touching the
// mutation paths (those go through `patch`).
type turnStoreSnapshot struct {
	StreamingBuf    string
	FinalText       string
	ReasoningActive bool
	ReasoningBuf    string
	Trail           []trailEntry
	Todos           []TodoItem
	Activity        []activityEntry
}

// resetTurnState wipes per-turn fields. Called at turn START. The
// pendingNotice is intentionally NOT reset here — it's flushed at
// the real turn-end sites instead, so a notice that arrived during
// this turn can be surfaced to the user (matching the Hermes
// "Strategy B" contract that flushPendingNotice is the only safe
// site for that operation).
func (t *turnStore) resetTurnState() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.streamingBuf = ""
	t.finalText = ""
	t.reasoningActive = false
	t.reasoningBuf = ""
	t.trail = nil
	t.todos = nil
	t.activity = nil
	t.subagents = nil
	t.wasInterrupted = false
	t.finalMessages = nil
	// pendingNotice intentionally preserved.
}

// enqueueNotice holds a notice that arrived mid-turn. Flushed by
// flushPendingNotice at the real turn-end sites only.
func (t *turnStore) enqueueNotice(n Notice) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pendingNotice = &n
}

// flushPendingNotice returns the held notice (if any) and clears the
// slot. Returns nil when nothing is pending. The App calls this from
// the three real turn-end sites (message complete, interrupt, error)
// and NEVER from resetTurnState.
func (t *turnStore) flushPendingNotice() *Notice {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := t.pendingNotice
	t.pendingNotice = nil
	return n
}

// uiStore is the session-scoped state slice. Everything that survives
// turn boundaries lives here: theme, busy mode, status text, notice
// (the currently-visible one), usage, statusBar position, etc. The
// Hermes split exists so resetTurnState never touches these prefs.
type uiStore struct {
	mu sync.Mutex

	// Busy/idle status.
	busy     bool
	indeterminate bool

	// Status text rendered in the status bar's left slot when not
	// streaming.
	statusText string

	// Notice currently visible (mirrors NoticeBoard, kept here
	// for state-split completeness).
	notice *Notice

	// Usage running total (giriş/çıkış token).
	usage    usage
	lastTurn time.Duration

	// Theme (skinnable, set at startup).
	theme *Theme

	// Status bar position.
	statusBarTop bool

	// Whether the FPS overlay is enabled (HERMES_TUI_FPS=1).
	fpsEnabled bool

	// Whether the terminal window currently has OS focus (DECSET
	// 1004). Used to halve the render tick rate while blurred.
	terminalFocused bool

	// Whether the user has enabled details mode globally.
	details DetailsMode

	// Indent / queueing.
	busyMode BusyMode

	// One-line last user message; surfaced as the sticky-prompt
	// breadcrumb when the chat has scrolled past it.
	lastUserText string
}

// usage is the running session total of input/output tokens.
type usage struct {
	InputTokens  int64
	OutputTokens int64
}

// patch atomically updates the UI store. Same pattern as turnStore.
func (u *uiStore) patch(mutate func(s *uiStore)) {
	u.mu.Lock()
	defer u.mu.Unlock()
	mutate(u)
}

// snapshot returns a copy of the current UI state for the renderer.
func (u *uiStore) snapshot() uiStoreSnapshot {
	u.mu.Lock()
	defer u.mu.Unlock()
	return uiStoreSnapshot{
		Busy:             u.busy,
		Indeterminate:    u.indeterminate,
		StatusText:       u.statusText,
		Usage:            u.usage,
		LastTurn:         u.lastTurn,
		StatusBarTop:     u.statusBarTop,
		FPSEnabled:       u.fpsEnabled,
		TerminalFocused:  u.terminalFocused,
		Details:          u.details,
		BusyMode:         u.busyMode,
		LastUserText:     u.lastUserText,
	}
}

// uiStoreSnapshot is the immutable view the renderer reads.
type uiStoreSnapshot struct {
	Busy             bool
	Indeterminate    bool
	StatusText       string
	Usage            usage
	LastTurn         time.Duration
	StatusBarTop     bool
	FPSEnabled       bool
	TerminalFocused  bool
	Details          DetailsMode
	BusyMode         BusyMode
	LastUserText     string
}

// newUIStore returns a uiStore with defaults.
func newUIStore() *uiStore {
	return &uiStore{
		terminalFocused: true, // assume focused until DECSET 1004 says otherwise
		details:         DetailsExpanded,
		busyMode:        BusyQueue,
	}
}

// newTurnStore returns an empty turn store.
func newTurnStore() *turnStore {
	return &turnStore{}
}

// _ = time — keeps the import in scope if the snapshot ever grows
// fields that need time import (LastTurn is already time.Duration but
// the file doesn't reference time.* directly; this avoids future
// "imported and not used" churn if a refactor changes a field).
var _ = time.Time{}
