package prompt

import (
	"log/slog"
	"path/filepath"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/home"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/memory"
)

// loadMemory reads one of the persistent stores into the system prompt.
//
// It is read here and nowhere else, which makes the contents a frozen
// snapshot: a write during a session lands on disk immediately but does not
// appear in the conversation it was written in. That is the point. The
// system prompt is the stable prefix every request in a session shares, and
// providers cache it; rewriting it mid-session throws that cache away and
// re-bills the whole prefix, which is a large price for a fact that will be
// just as true at the start of the next session.
//
// A store that cannot be read is not fatal. Running without a memory is the
// normal state for a fresh project, so a permission problem or a corrupt
// file degrades to that rather than refusing to start.
func loadMemory(store *config.ConfigStore, scope memory.Scope) string {
	cfg := store.Config()
	opts := memory.Options{
		ProjectDir: filepath.Join(cfg.Options.DataDirectory, "memory"),
		UserDir:    filepath.Join(home.Config(), "atlas"),
	}
	if m := cfg.Options.Memory; m != nil {
		opts.ProjectLimit = m.ProjectLimit
		opts.UserLimit = m.UserLimit
	}

	content, err := memory.New(opts).Read(scope)
	if err != nil {
		slog.Warn("Could not read memory; continuing without it", "scope", scope, "error", err)
		return ""
	}
	return content
}
