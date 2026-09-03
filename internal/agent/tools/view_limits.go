package tools

import "github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"

// ViewLimits are the per-workspace limits on a view call: how many lines it
// returns when the model does not say, and how much of a single long line
// survives before it is cut.
//
// Zero fields mean the built-in defaults, so the zero value is usable
// rather than one that returns nothing.
type ViewLimits struct {
	DefaultReadLimit int
	MaxLineLength    int
	HashAnchors      bool
}

// NewViewLimits reads the limits out of the config's tools section.
func NewViewLimits(cfg *config.Config) ViewLimits {
	if cfg == nil {
		return ViewLimits{}
	}
	readLimit, lineLength := cfg.Tools.View.Limits()
	return ViewLimits{
		DefaultReadLimit: readLimit,
		MaxLineLength:    lineLength,
		HashAnchors:      cfg.Tools.View.HashAnchorsEnabled(),
	}
}

func (l ViewLimits) readLimit() int {
	if l.DefaultReadLimit > 0 {
		return l.DefaultReadLimit
	}
	return DefaultReadLimit
}

func (l ViewLimits) lineLength() int {
	if l.MaxLineLength > 0 {
		return l.MaxLineLength
	}
	return MaxLineLength
}
