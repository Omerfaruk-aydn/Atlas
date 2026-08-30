package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// maxInt/lipglossWidth/repeatSpace are tiny inline shims so this file
// doesn't have to import the broader "help" package just to spell a few
// primitives. Keeping them unexported here is intentional — banner.go is
// the only place that needs them at this level.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func lipglossWidth(s string) int { return lipgloss.Width(s) }

func repeatSpace(n int) string { return strings.Repeat(" ", n) }

// Banner width breakpoints. The Hermes banner degrades through three
// tiers, and a fully-hidden fallback below ~34 columns. The exact numbers
// are tuned to its logoWidth (the wide logo + tagline combo); Atlas's own
// logo is much narrower, so the thresholds are scaled accordingly while
// keeping the same "tag → name → nothing" descent.
const (
	bannerLogoFrom    = 80 // full logo + tagline fits cleanly
	bannerCompactFrom = 50 // drop the tagline, keep "Atlas"
	bannerNameFrom    = 28 // drop the model label, keep just "Atlas"
	bannerHideBelow   = 18 // under this, render nothing (let the bar be empty)
)

// bannerTier identifies which degradation level a given width resolves
// to. Useful when a renderer needs to know "is there a tagline here?"
// without re-checking the width itself.
type bannerTier int

const (
	bannerHidden bannerTier = iota
	bannerName               // just "Atlas"
	bannerCompact            // "Atlas" + optional model label
	bannerFull               // "Atlas — terminal AI agent" + right-side model
)

// resolveBannerTier maps a width to the highest tier it can support. The
// thresholds intentionally leave a 2-column safety margin so the rendered
// line never overflows the terminal on the boundary.
func resolveBannerTier(width int) bannerTier {
	switch {
	case width < bannerHideBelow:
		return bannerHidden
	case width < bannerNameFrom:
		return bannerName
	case width < bannerCompactFrom:
		return bannerCompact
	default:
		return bannerFull
	}
}

// renderBanner paints the brand title bar for the given width, in the
// highest tier that fits. The full and compact tiers append a right-side
// provider/model label so the user can see what's active without opening
// the picker; on narrow widths we drop the right label first (lower
// priority), then the tagline, before ever truncating mid-word.
func (a *App) renderBanner() string {
	tier := resolveBannerTier(a.width)
	if tier == bannerHidden {
		// Below the threshold, return a blank but still-1-tall line so
		// the layout math (rows == header + chat + input + status) stays
		// consistent regardless of terminal width.
		return strings.Repeat(" ", maxInt(a.width, 0))
	}

	left := bannerLeftForTier(tier)
	right := bannerRightForTier(tier, a)

	if right == "" {
		// No right label to align — just left, padded to the full width.
		gap := a.width - lipglossWidth(left) - 2
		if gap < 1 {
			gap = 1
		}
		return a.theme.HeaderBar.Width(a.width - 2).Render(left + repeatSpace(gap))
	}

	// Reserve at least 2 cells of gap between left and right.
	gap := a.width - lipglossWidth(left) - lipglossWidth(right) - 4
	if gap < 1 {
		gap = 1
	}
	return a.theme.HeaderBar.Width(a.width - 2).Render(left + repeatSpace(gap) + right)
}

// bannerLeftForTier returns the left text of the header for the given
// tier. At full width it's the tagline; compact keeps just "Atlas"; the
// narrow tier also keeps just "Atlas" but is reached only after a
// sub-threshold width already had to drop it.
func bannerLeftForTier(tier bannerTier) string {
	switch tier {
	case bannerFull:
		return "Atlas — terminal AI agent"
	default:
		return "Atlas"
	}
}

// bannerRightForTier returns the right-side provider/model label, or ""
// if the tier can't fit one. Tier 2 (compact) still tries; tier 1 and 0
// always return empty.
func bannerRightForTier(tier bannerTier, a *App) string {
	if tier < bannerCompact {
		return ""
	}
	provider := a.agent.ProviderName()
	model := a.agent.CurrentModel()
	if provider == "" && model == "" {
		return ""
	}
	return provider + "/" + model
}
