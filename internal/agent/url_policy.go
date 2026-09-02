package agent

import (
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/agent/tools"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
)

// urlPolicy builds the fetch/download domain policy from config. The zero
// value (both lists empty) permits everything, matching pre-policy behavior.
func urlPolicy(cfg *config.ConfigStore) tools.URLPolicy {
	opts := cfg.Config().Options
	return tools.URLPolicy{
		Allow: opts.AllowedDomains,
		Deny:  opts.BlockedDomains,
	}
}
