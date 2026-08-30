package tui

import (
	"sync"
	"time"
)

// TurnController is a class-based controller that owns the per-turn
// ephemeral state — the streaming buffer, the reasoning buffer, the
// tool trail, the activity feed, and the pending notice slot. It
// exists in Hermes as a separate class because React effects can't
// cleanly own mutable per-turn timers themselves; Bubbletea's Update
// loop already serializes everything, so Atlas can keep most of this
// on the App directly, but pulling the most contested fields into a
// dedicated struct makes the lifetime explicit and gives us a
// testable surface independent of the full App.
type TurnController struct {
	mu sync.Mutex

	// Streaming buffer: the live text the model is producing.
	streamingBuf string

	// Reasoning buffer: the model's hidden "thinking" channel
	// (not yet rendered to the chat). Hermes pulses a marker when
	// this is non-empty so the user knows the model is reasoning
	// even if no text has arrived yet.
	reasoningBuf   string
	reasoningActive bool
	reasoningPulse time.Time

	// Tool trail (capped at TRAIL_LIMIT).
	trail    []trailEntry
	trailBuf []string

	// Activity feed (capped at ACTIVITY_LIMIT).
	activity []activityEntry

	// Subagent tree: orphaned children promoted to top-level,
	// sorted by (depth, index). Atlas's current agent doesn't emit
	// subagent events, so this stays empty in practice — but the
	// struct is here for when multi-agent lands.
	subagents []subagentNode

	// Pending notice: held while a turn is busy; flushed at the
	// three real turn-end sites (message complete, interrupt,
	// error) but NEVER inside resetTurn.
	pendingNotice *Notice

	// Tool-progress debounce.
	toolProgressLastFlush time.Time
	toolProgressDirty    bool

	// Activity-trail limiting.
	trailLimit  int
	activityLimit int
}

const (
	trailLimitDefault    = 8
	activityLimitDefault = 8
)

func newTurnController() *TurnController {
	return &TurnController{
		trailLimit:    trailLimitDefault,
		activityLimit: activityLimitDefault,
	}
}

// trailEntry is one row in the tool/reasoning trail. Each entry has
// a kind so the renderer can show the right glyph.
type trailEntry struct {
	Kind   string // "tool" | "reasoning" | "tool_result"
	Text   string
	At     time.Time
}

// activityEntry is one row in the ambient activity feed.
type activityEntry struct {
	Text string
	At   time.Time
}

// subagentNode is one node in the subagent delegation tree.
type subagentNode struct {
	ID       string
	Parent   string
	Depth    int
	Index    int
	ToolName string
	Status   string
	Started  time.Time
}

// StartMessage resets the per-turn buffers. Called when a new turn
// begins. Note: pendingNotice is intentionally NOT cleared here —
// it's flushed at the real turn-end sites instead, so a notice that
// arrived during this turn can be surfaced to the user.
func (c *TurnController) StartMessage() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.streamingBuf = ""
	c.reasoningBuf = ""
	c.reasoningActive = false
	c.reasoningPulse = time.Time{}
	c.trail = nil
	c.trailBuf = nil
	c.activity = nil
	c.toolProgressLastFlush = time.Time{}
	c.toolProgressDirty = false
}

// AppendStreaming appends a text delta to the live buffer. Caller is
// responsible for marking the App dirty so the next render tick picks
// it up; the controller doesn't own the render loop.
func (c *TurnController) AppendStreaming(delta string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.streamingBuf += delta
}

// FlushStreamingSegment seals the current streaming buffer into a
// committed message and resets the buffer. Returns the sealed text.
func (c *TurnController) FlushStreamingSegment() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.streamingBuf
	c.streamingBuf = ""
	return out
}

// AppendReasoning appends to the reasoning buffer and turns on the
// pulse marker.
func (c *TurnController) AppendReasoning(delta string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reasoningBuf += delta
	c.reasoningActive = true
	c.reasoningPulse = time.Now()
}

// ClearReasoning turns off the reasoning marker and clears the buffer.
func (c *TurnController) ClearReasoning() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reasoningBuf = ""
	c.reasoningActive = false
}

// AppendTrail adds a tool/reasoning/result row, evicting the oldest
// when the cap is reached.
func (c *TurnController) AppendTrail(kind, text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.trail = append(c.trail, trailEntry{Kind: kind, Text: text, At: time.Now()})
	if len(c.trail) > c.trailLimit {
		c.trail = c.trail[len(c.trail)-c.trailLimit:]
	}
}

// AppendActivity adds an activity-feed row, evicting the oldest.
func (c *TurnController) AppendActivity(text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.activity = append(c.activity, activityEntry{Text: text, At: time.Now()})
	if len(c.activity) > c.activityLimit {
		c.activity = c.activity[len(c.activity)-c.activityLimit:]
	}
}

// EnqueueNotice holds a notice that arrived mid-turn; the App flushes
// it at one of the three real turn-end sites.
func (c *TurnController) EnqueueNotice(n Notice) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pendingNotice = &n
}

// FlushPendingNotice returns the held notice (if any) and clears the
// slot. Returns nil when nothing is pending. App calls this from the
// three real turn-end sites only.
func (c *TurnController) FlushPendingNotice() *Notice {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := c.pendingNotice
	c.pendingNotice = nil
	return n
}

// UpsertSubagent inserts or updates a subagent node by ID. Used by
// the gateway event handler when subagent.* events arrive. create=true
// creates the node if missing; create=false silently no-ops on miss
// (so a late "running" event for a completed subagent can't resurrect
// it into the live tree).
func (c *TurnController) UpsertSubagent(id, parent string, depth, index int, toolName, status string, started time.Time, create bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, n := range c.subagents {
		if n.ID == id {
			c.subagents[i].Status = status
			return
		}
	}
	if !create {
		return
	}
	c.subagents = append(c.subagents, subagentNode{
		ID:       id,
		Parent:   parent,
		Depth:    depth,
		Index:    index,
		ToolName: toolName,
		Status:   status,
		Started:  started,
	})
}

// IsReasoningActive is the testable surface for "is the model
// currently reasoning?" — used by the busy indicator to swap the
// verb and by the transcript renderer to show the reasoning glyph.
func (c *TurnController) IsReasoningActive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reasoningActive
}

// Trail returns a copy of the current tool/reasoning trail.
func (c *TurnController) Trail() []trailEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]trailEntry, len(c.trail))
	copy(out, c.trail)
	return out
}
