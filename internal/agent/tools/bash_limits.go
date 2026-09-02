package tools

import "github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"

// BashLimits are the per-workspace limits on a bash tool call: how wide the
// output it returns may be, and how long a command runs in the foreground
// before it is moved to a background job.
//
// Zero fields mean the built-in defaults, so the zero value is a usable
// BashLimits rather than one that truncates everything to nothing and
// backgrounds every command immediately.
type BashLimits struct {
	MaxOutputLength     int
	AutoBackgroundAfter int
}

// NewBashLimits reads the limits out of the config's tools section.
func NewBashLimits(cfg *config.Config) BashLimits {
	if cfg == nil {
		return BashLimits{}
	}
	maxOutput, autoBackground := cfg.Tools.Bash.Limits()
	return BashLimits{
		MaxOutputLength:     maxOutput,
		AutoBackgroundAfter: autoBackground,
	}
}

// autoBackgroundAfter is the threshold in seconds, given what the model
// asked for on this specific call. An explicit per-call value wins over the
// workspace setting: the model knows which command it just wrote.
func (l BashLimits) autoBackgroundAfter(requested int) int {
	if requested > 0 {
		return requested
	}
	if l.AutoBackgroundAfter > 0 {
		return l.AutoBackgroundAfter
	}
	return DefaultAutoBackgroundAfter
}
