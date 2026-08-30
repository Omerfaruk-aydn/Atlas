package tui

import "strings"

// MessageQueue is the dedicated queue implementation. Atlas's App
// already has a queuedMessages []string; this is the principled
// version with the Hermes edit-preserves-payload semantics.
//
// Each item carries both the full submission text and a collapsed
// display label (e.g. a long paste becomes "[[ first.. [3 lines] ..
// last ]]" for the queued preview, while the full text is what gets
// sent on drain).
type QueueItem struct {
	Text    string // the actual payload sent to the agent
	Display string // the collapsed preview rendered in the chat pane
}

// MessageQueue is the dedicated queue with edit-preserves-payload
// semantics. Mutating methods (Enqueue, Prepend, Dequeue, Take,
// Remove) all keep the slice in place; the App can re-derive the
// display strings on demand for the rendering layer.
type MessageQueue struct {
	items  []QueueItem
	editIdx int // index of the queue item currently being edited in-place
}

// Enqueue adds a new item to the tail.
func (q *MessageQueue) Enqueue(text, display string) {
	q.items = append(q.items, QueueItem{Text: text, Display: display})
}

// Prepend inserts a new item at the head (used for "put this back at
// the front" scenarios like a soft-cancel of a draining message).
func (q *MessageQueue) Prepend(text, display string) {
	q.items = append([]QueueItem{{Text: text, Display: display}}, q.items...)
}

// Dequeue returns and removes the head item, or (zeroValue, false)
// when the queue is empty.
func (q *MessageQueue) Dequeue() (QueueItem, bool) {
	if len(q.items) == 0 {
		return QueueItem{}, false
	}
	it := q.items[0]
	q.items = q.items[1:]
	return it, true
}

// Take removes and returns the item at index i. If the caller also
// passes an edited display, the substitution is mirrored into the
// underlying Text payload via a substring swap of the old display —
// so editing a queued item's visible text also updates what actually
// gets sent, as long as the edited text still contains the original
// display substring.
func (q *MessageQueue) Take(i int, editedDisplay string) (QueueItem, bool) {
	if i < 0 || i >= len(q.items) {
		return QueueItem{}, false
	}
	it := q.items[i]
	q.items = append(q.items[:i], q.items[i+1:]...)
	if editedDisplay != "" && strings.Contains(editedDisplay, it.Display) {
		// Mirror the edit into the underlying payload via the old
		// display substring.
		it.Text = strings.Replace(it.Text, it.Display, editedDisplay, 1)
		it.Display = editedDisplay
	} else if editedDisplay != "" {
		// Edit replaced the label entirely — drop the paste linkage.
		it.Text = editedDisplay
		it.Display = editedDisplay
	}
	return it, true
}

// Remove removes the item at index i without returning it. No-op on
// out-of-bounds indices (matches Hermes's no-throw contract).
func (q *MessageQueue) Remove(i int) {
	if i < 0 || i >= len(q.items) {
		return
	}
	q.items = append(q.items[:i], q.items[i+1:]...)
}

// Items returns a copy of the underlying slice for safe iteration.
func (q *MessageQueue) Items() []QueueItem {
	out := make([]QueueItem, len(q.items))
	copy(out, q.items)
	return out
}

// Len returns the queue length.
func (q *MessageQueue) Len() int { return len(q.items) }
