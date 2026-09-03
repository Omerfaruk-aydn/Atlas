// Package credentials implements round-robin rotation between a provider's
// configured API keys, persisted to disk so the rotation survives process
// restarts -- a new one-shot CLI invocation, a restarted server -- and so a
// separate read-only consumer (see `atlas provider usage`) can show what a
// live session would pick next without needing one running.
package credentials

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// StateFileName is the file a Rotator's state is persisted to, relative to
// the workspace's data directory.
const StateFileName = "credential_rotation.json"

// State is the on-disk form of a Rotator: the next round-robin index per
// provider ID.
type State struct {
	Next map[string]int `json:"next"`
}

// Rotator round-robins between a provider's configured API keys
// (ProviderConfig.APIKey plus ProviderConfig.APIKeys) across separate
// session/model builds. Rotation happens with session affinity: a running
// session keeps whichever key Pick returned for it, and the next session or
// model rebuild for that provider gets the next key in line.
//
// This is deliberately not a live, mid-stream failover: swapping the API
// key underneath an in-flight request would mean rebuilding its client,
// which callers are not wired to do. What this gives instead is real value
// for the common "one subscription's quota ran out" case: Advance is meant
// to be called when a 429 hits a provider with no further model to fall
// back to, so the very next turn or session against that provider tries a
// different key/account instead of the one that just got rate-limited.
type Rotator struct {
	mu   sync.Mutex
	path string // empty means in-memory only, no persistence
	next map[string]int
}

// New returns a Rotator with no persistence: state lives only as long as
// the process does. Used by tests and by any caller with no data directory
// to persist to.
func New() *Rotator {
	return &Rotator{next: make(map[string]int)}
}

// Load returns a Rotator whose state is read from path if it exists.
// A missing or unreadable file is treated as empty state, not an error: a
// corrupt or absent rotation file should never stop a session from
// starting. State changes are saved back to path as they happen.
func Load(path string) *Rotator {
	r := &Rotator{path: path, next: make(map[string]int)}
	state := ReadState(path)
	if state.Next != nil {
		r.next = state.Next
	}
	return r
}

// ReadState reads the persisted state at path without constructing a
// Rotator, for a read-only consumer -- `atlas provider usage` -- that only
// wants to display it. Returns a zero-value State (an empty, non-nil map)
// on any error, so a caller never has to nil-check before ranging over it.
func ReadState(path string) State {
	state := State{Next: map[string]int{}}
	data, err := os.ReadFile(path)
	if err != nil {
		return state
	}
	var parsed State
	if err := json.Unmarshal(data, &parsed); err != nil || parsed.Next == nil {
		return state
	}
	return parsed
}

// Pick returns the next key in round-robin order for providerID. A single
// key is returned as-is without touching or persisting rotation state, so
// a provider with only one configured key never writes a state file nobody
// needs.
func (r *Rotator) Pick(providerID string, keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	if r == nil || len(keys) == 1 {
		return keys[0]
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	idx := r.next[providerID] % len(keys)
	r.next[providerID] = idx + 1
	r.saveLocked()
	return keys[idx]
}

// Advance skips providerID's rotation forward by one, without needing to
// know which key was actually in use. Called when a provider's current key
// hits a 429 that a model fallback chain had nowhere further to go on, so
// the next Pick for that provider is less likely to hand back the same
// exhausted key.
func (r *Rotator) Advance(providerID string) {
	if r == nil || providerID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next[providerID]++
	r.saveLocked()
}

// State returns a snapshot of the rotator's current state, for a caller
// that holds a live Rotator and wants to display it directly rather than
// re-reading the file it was loaded from.
func (r *Rotator) State() State {
	if r == nil {
		return State{Next: map[string]int{}}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	next := make(map[string]int, len(r.next))
	for k, v := range r.next {
		next[k] = v
	}
	return State{Next: next}
}

// saveLocked persists state to r.path, best-effort: a write failure (e.g. a
// read-only data directory) is logged and otherwise ignored, since
// rotation still works in-memory for the life of this process either way.
// Callers must hold r.mu.
func (r *Rotator) saveLocked() {
	if r.path == "" {
		return
	}
	data, err := json.MarshalIndent(State{Next: r.next}, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		slog.Warn("Failed to persist credential rotation state", "path", r.path, "error", err)
		return
	}
	if err := os.WriteFile(r.path, data, 0o644); err != nil {
		slog.Warn("Failed to persist credential rotation state", "path", r.path, "error", err)
	}
}

// CandidateAPIKeys returns every API key template configured for a
// provider -- the primary key followed by any additional ones -- skipping
// blanks, in the order Pick round-robins through.
func CandidateAPIKeys(apiKey string, apiKeys []string) []string {
	keys := make([]string, 0, 1+len(apiKeys))
	if apiKey != "" {
		keys = append(keys, apiKey)
	}
	for _, k := range apiKeys {
		if k != "" {
			keys = append(keys, k)
		}
	}
	return keys
}
