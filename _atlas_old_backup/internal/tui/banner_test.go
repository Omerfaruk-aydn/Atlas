package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// resolveBannerTier boundary checks.
func TestResolveBannerTier(t *testing.T) {
	cases := []struct {
		width int
		want  bannerTier
	}{
		{10, bannerHidden},
		{bannerHideBelow - 1, bannerHidden},
		{bannerHideBelow, bannerName},
		{bannerNameFrom - 1, bannerName},
		{bannerNameFrom, bannerCompact},
		{bannerCompactFrom - 1, bannerCompact},
		{bannerCompactFrom, bannerFull},
		{500, bannerFull},
	}
	for _, c := range cases {
		if got := resolveBannerTier(c.width); got != c.want {
			t.Errorf("resolveBannerTier(%d) = %d, want %d", c.width, got, c.want)
		}
	}
}

// Banner fits within the terminal width at every tier.
func TestBannerFitsAtEveryTier(t *testing.T) {
	for _, w := range []int{18, 28, 50, 80, 120, 200} {
		app := newTestApp(w, 24)
		out := app.renderBanner()
		if lipgloss.Width(out) > w {
			t.Errorf("banner at width %d = %d wide, must fit", w, lipgloss.Width(out))
		}
	}
}

// Narrow banner shows the short "Atlas" label.
func TestBannerNarrowDropsTagline(t *testing.T) {
	app := newTestApp(35, 24)
	out := app.renderBanner()
	if !strings.Contains(out, "Atlas") {
		t.Error("narrow banner should still show the brand name")
	}
	if strings.Contains(out, "terminal AI agent") {
		t.Error("narrow banner should drop the tagline")
	}
}
