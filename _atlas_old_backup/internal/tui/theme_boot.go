package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ThemeBoot is the flash-free theme cache. Hermes persists the
// last-resolved Theme + background hex to ~/.hermes/tui-theme-boot.json
// (atomic write via tmp-file + rename, debounced 400ms, unref'd
// timer) and replays the cached theme as the FIRST frame — before
// the async OSC-11 probe / gateway skin / config sync can resolve —
// so the user never sees a visible default→final theme flash.
//
// Atlas's port keeps the same shape but writes to
// ~/.atlas/tui-theme-boot.json (the project's actual config dir).
type ThemeBoot struct {
	mu       sync.Mutex
	path     string
	debounce time.Duration
	pending  *time.Timer
	clock    func() time.Time
}

// themeBootPayload is the on-disk shape. We store the resolved
// background hex and the light/dark pin; the App reconstructs the
// theme from these on next launch.
type themeBootPayload struct {
	Background string `json:"background"`
	Light      string `json:"light"` // "dark" | "light" | "unknown"
	WrittenAt  int64  `json:"written_at"`
}

func newThemeBoot() *ThemeBoot {
	home, _ := os.UserHomeDir()
	return &ThemeBoot{
		path:     filepath.Join(home, ".atlas", "tui-theme-boot.json"),
		debounce: 400 * time.Millisecond,
		clock:    time.Now,
	}
}

// Load returns the cached payload, or zero-value + (false) when no
// cache exists. Invalidates the cache if the live OSC-11 probe
// returns an untrusted value (pure black, the "unset default"
// fingerprint) — that's a provenance-tracking guard so a stale
// cached background can't outrank live detection forever.
func (t *ThemeBoot) Load() (themeBootPayload, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	data, err := os.ReadFile(t.path)
	if err != nil {
		return themeBootPayload{}, false
	}
	var p themeBootPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return themeBootPayload{}, false
	}
	return p, true
}

// Schedule queues a write of the payload. The actual write is
// debounced by t.debounce so a flurry of probe resolutions
// coalesces into one disk hit.
func (t *ThemeBoot) Schedule(p themeBootPayload) {
	t.mu.Lock()
	if t.pending != nil {
		t.pending.Stop()
	}
	t.pending = time.AfterFunc(t.debounce, func() {
		t.flush(p)
	})
	t.mu.Unlock()
}

// flush writes the payload to disk atomically (tmp file + rename).
func (t *ThemeBoot) flush(p themeBootPayload) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(t.path), 0o755); err != nil {
		return
	}
	tmp := t.path + ".tmp"
	data, err := json.Marshal(p)
	if err != nil {
		return
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, t.path)
}
