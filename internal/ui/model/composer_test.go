package model

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/maincodss/atlas-agent/internal/permission"
	"github.com/stretchr/testify/require"
)

// The layout, cursor placement, and mouse mapping are all computed from the
// frame's dimensions, so every mode — solid or rainbow — has to produce
// exactly the same box for the same input. A rainbow frame that came out a row
// short would put the cursor in the wrong place.
func TestComposerFrameDimensionsMatchAcrossModes(t *testing.T) {
	t.Parallel()

	content := "line one\nline two\nline three"
	wantRows := strings.Count(content, "\n") + 1 + composerBorderRows

	for _, tc := range []struct {
		name string
		mode permission.PermissionMode
		bang bool
	}{
		{"manual", permission.ModeManual, false},
		{"yolo", permission.ModeBypass, false},
		{"plan", permission.ModePlan, false},
		{"auto-accept", permission.ModeAutoAcceptEdits, false},
		{"shell", permission.ModeBypass, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			u := newTestUI()
			u.modeCache.val = tc.mode
			u.bangMode = tc.bang

			for _, width := range []int{40, 80, 120} {
				got := u.composerFrame(content, width)
				lines := strings.Split(got, "\n")
				require.Len(t, lines, wantRows, "width %d row count", width)
				for i, line := range lines {
					require.Equal(t, width, lipgloss.Width(line),
						"width %d, row %d is the wrong width: %q", width, i, line)
				}
			}
		})
	}
}

// Yolo is the only mode that sweeps, and bang mode overrides it the same way
// it overrides the prompt.
func TestComposerRainbowOnlyInYolo(t *testing.T) {
	t.Parallel()

	u := newTestUI()

	u.modeCache.val = permission.ModeBypass
	require.True(t, u.composerRainbow())

	u.bangMode = true
	require.False(t, u.composerRainbow(), "bang mode overrides yolo")

	u.bangMode = false
	for _, mode := range []permission.PermissionMode{
		permission.ModeManual, permission.ModePlan, permission.ModeAutoAcceptEdits,
	} {
		u.modeCache.val = mode
		require.False(t, u.composerRainbow(), "mode %q must stay solid", mode)
	}
}

// The rainbow has to actually advance with the tick, and the tick chain has to
// stay armed in the chat state where there is no wordmark to drive it.
func TestComposerRainbowAnimates(t *testing.T) {
	t.Parallel()

	u := newTestUI()
	u.modeCache.val = permission.ModeBypass
	u.state = uiChat
	require.True(t, u.bannerAnimating(), "yolo keeps the tick chain running outside the landing screen")

	const width = 80
	first := u.composerFrame("hello", width)
	u.bannerFrame += composerFrameDivisor
	require.NotEqual(t, first, u.composerFrame("hello", width), "the sweep should advance with the frame")

	// Manual mode is static: the same input renders identically whatever the
	// tick is doing.
	u.modeCache.val = permission.ModeManual
	still := u.composerFrame("hello", width)
	u.bannerFrame += composerFrameDivisor
	require.Equal(t, still, u.composerFrame("hello", width))
}
