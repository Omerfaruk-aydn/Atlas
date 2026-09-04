package agent

import (
	"path/filepath"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/factstore"
)

// factsStore builds the in-session fact store for a workspace.
//
// It lives beside project memory in the project's data directory, but is
// a separate file: memory is loaded once and bounded, this is queryable
// mid-session and unbounded. See facts.md for the distinction the agent
// itself is told.
func factsStore(cfg *config.ConfigStore) *factstore.Store {
	return factstore.New(filepath.Join(cfg.Config().Options.DataDirectory, "facts.jsonl"))
}
