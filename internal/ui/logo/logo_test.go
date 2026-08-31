package logo

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/ui/styles"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func testOpts(width int, hyper bool) (lipgloss.Style, Opts) {
	t := styles.ThemeForProvider("")
	return t.Logo.GradCanvas, Opts{
		TitleColorA:  t.Logo.TitleColorA,
		TitleColorB:  t.Logo.TitleColorB,
		CharmColor:   t.Logo.CharmColor,
		VersionColor: t.Logo.VersionColor,
		Width:        width,
		Hyper:        hyper,
	}
}

// The compact logo goes into surfaces that own their width — the sidebar and
// the exit banner — so it has to fit whatever it is given, right down to the
// widths where no lettering fits at all.
func TestCompactRenderFitsTheGivenWidth(t *testing.T) {
	t.Parallel()

	for _, hyper := range []bool{false, true} {
		for width := 1; width <= 80; width++ {
			base, o := testOpts(width, hyper)
			got := Render(base, "v1.2.3-longish", true, o)
			for _, line := range strings.Split(got, "\n") {
				require.LessOrEqual(t, ansi.StringWidth(line), width,
					"compact logo overflows width %d (hyper=%v): %q", width, hyper, line)
			}
		}
	}
}

// A zero width means "no limit", which is what callers that do not track a
// width rely on. It must still produce the full wordmark.
func TestCompactRenderUnboundedWidth(t *testing.T) {
	t.Parallel()

	base, o := testOpts(0, false)
	got := Render(base, "v1.2.3", true, o)
	require.Equal(t, lipgloss.Width(fitWordmark(false, 0)), lipgloss.Width(got))
}

// Narrowing the width should degrade the logo in steps rather than falling
// straight from the full wordmark to plain text.
func TestFitWordmarkDegradesInSteps(t *testing.T) {
	t.Parallel()

	full := lipgloss.Width(fitWordmark(false, 0))
	short := lipgloss.Width(fitWordmark(false, full-1))
	require.Less(t, short, full, "the narrow variant must actually be narrower")
	require.NotEmpty(t, fitWordmark(false, short))
	require.Empty(t, fitWordmark(false, short-1), "nothing narrower than the short variant should fit")
}
