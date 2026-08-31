package styles

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Every framed surface has to agree about its corners: the ones drawn by hand
// (the composer, the landing cards) read the package variables directly, while
// the ones drawn through lipgloss and Ultraviolet go through the border
// constructors. A user whose font lacks a glyph must not get a clean composer
// and boxed-out dialogs.
func TestBordersFollowTheActiveCornerStyle(t *testing.T) {
	restore := [4]string{BoxTopLeft, BoxTopRight, BoxBottomLeft, BoxBottomRight}
	t.Cleanup(func() {
		BoxTopLeft, BoxTopRight, BoxBottomLeft, BoxBottomRight = restore[0], restore[1], restore[2], restore[3]
	})

	for _, style := range BoxCornerStyles() {
		SetBoxCorners(style)
		want := boxCorners[style]

		b := Border()
		require.Equal(t, want, [4]string{b.TopLeft, b.TopRight, b.BottomLeft, b.BottomRight}, style)
		require.Equal(t, BoxHorizontal, b.Top, style)
		require.Equal(t, BoxVertical, b.Left, style)

		u := UVBorder()
		require.Equal(t, want,
			[4]string{u.TopLeft.Content, u.TopRight.Content, u.BottomLeft.Content, u.BottomRight.Content}, style)
	}
}

// An empty or unknown style is not an error: it means "pick one for this
// terminal", which is what an unset config looks like.
func TestSetBoxCornersFallsBackToDetection(t *testing.T) {
	restore := [4]string{BoxTopLeft, BoxTopRight, BoxBottomLeft, BoxBottomRight}
	t.Cleanup(func() {
		BoxTopLeft, BoxTopRight, BoxBottomLeft, BoxBottomRight = restore[0], restore[1], restore[2], restore[3]
	})

	want := boxCorners[DetectCornerStyle()]
	for _, style := range []string{"", "not-a-style"} {
		SetBoxCorners(style)
		require.Equal(t, want, [4]string{BoxTopLeft, BoxTopRight, BoxBottomLeft, BoxBottomRight}, style)
	}
}
