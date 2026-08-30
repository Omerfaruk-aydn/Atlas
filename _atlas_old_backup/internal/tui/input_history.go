package tui

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// InputHistory is a persistent ring of previously submitted messages.
// The file lives at $XDG_STATE_HOME/atlas/history (default
// ~/.local/state/atlas/history). Each line is one message; the file
// is append-only. A 1024-entry cap keeps the file from growing
// without bound.
type InputHistory struct {
	mu       sync.Mutex
	items    []string
	loaded   bool
	filePath string
	cap      int
}

const historyCap = 1024

// newInputHistory returns an empty history with the file path computed
// from XDG_STATE_HOME (or ~/.local/state as the Atlas-default).
func newInputHistory() *InputHistory {
	return &InputHistory{
		cap:      historyCap,
		filePath: defaultHistoryPath(),
	}
}

func defaultHistoryPath() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "atlas", "history")
}

// Load reads the history file (if any) into memory. Idempotent.
// Stops at historyCap; the file is never loaded past that bound.
func (h *InputHistory) Load() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.loaded {
		return nil
	}
	f, err := os.Open(h.filePath)
	if err != nil {
		// Missing file is fine — the user just hasn't sent anything yet.
		if os.IsNotExist(err) {
			h.loaded = true
			return nil
		}
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024) // 1 MiB max line
	var items []string
	for sc.Scan() {
		line := sc.Text()
		if line != "" {
			items = append(items, line)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if len(items) > h.cap {
		items = items[len(items)-h.cap:]
	}
	h.items = items
	h.loaded = true
	return nil
}

// Append adds a new entry to the in-memory list (and the on-disk file).
// Empty / whitespace-only messages are ignored — those would never
// have been submitted, and storing them just clutters recall.
func (h *InputHistory) Append(text string) error {
	t := strings.TrimSpace(text)
	if t == "" {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.loaded {
		// Lazy-load on first append. We don't error out on failure
		// to keep Append() always non-blocking for the submit path.
		_ = h.loadNoLock()
	}
	h.items = append(h.items, t)
	if len(h.items) > h.cap {
		// Drop the oldest entry. We rewrite the whole file rather
		// than try to surgically edit the head.
		h.items = h.items[len(h.items)-h.cap:]
	}
	return h.flushNoLock()
}

func (h *InputHistory) loadNoLock() error {
	if h.loaded {
		return nil
	}
	f, err := os.Open(h.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			h.loaded = true
			return nil
		}
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var items []string
	for sc.Scan() {
		line := sc.Text()
		if line != "" {
			items = append(items, line)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if len(items) > h.cap {
		items = items[len(items)-h.cap:]
	}
	h.items = items
	h.loaded = true
	return nil
}

// flushNoLock writes the in-memory list to disk. Errors are
// swallowed (logged nowhere) — failing to persist a history entry
// shouldn't break the user's submit path.
func (h *InputHistory) flushNoLock() error {
	if err := os.MkdirAll(filepath.Dir(h.filePath), 0o755); err != nil {
		return err
	}
	f, err := os.Create(h.filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, it := range h.items {
		if _, err := w.WriteString(it + "\n"); err != nil {
			return err
		}
	}
	return w.Flush()
}

// Items returns a copy of the in-memory history. Index 0 is the most
// recently sent message.
func (h *InputHistory) Items() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.loaded {
		_ = h.loadNoLock()
	}
	out := make([]string, len(h.items))
	copy(out, h.items)
	return out
}
