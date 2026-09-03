package shell

import (
	"log/slog"
	"sync"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/sandbox"
)

var sandboxState struct {
	mu      sync.RWMutex
	enabled bool
	limits  sandbox.Limits
}

// SetSandboxLimits turns Job Object containment on or off for every
// external process the shell interpreter spawns from here on (bash tool
// commands, hook commands, and scripts they invoke). Call it once during
// setup, before commands run; safe to call again to change limits,
// though a process already contained under an old job keeps running
// under it.
//
// Logs once and leaves containment off if enabled is true on a platform
// sandbox.Supported reports false for, rather than silently doing
// nothing -- a user who turned this on should know it isn't taking
// effect.
func SetSandboxLimits(enabled bool, limits sandbox.Limits) {
	if enabled && !sandbox.Supported() {
		slog.Warn("Sandbox requested but not supported on this OS; running without containment")
		enabled = false
	}
	sandboxState.mu.Lock()
	defer sandboxState.mu.Unlock()
	sandboxState.enabled = enabled
	sandboxState.limits = limits
}

// currentSandboxLimits reports whether containment is on and, if so, the
// limits to apply.
func currentSandboxLimits() (sandbox.Limits, bool) {
	sandboxState.mu.RLock()
	defer sandboxState.mu.RUnlock()
	return sandboxState.limits, sandboxState.enabled
}
