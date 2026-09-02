package common

import (
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/styles"
)

// minThumbSize keeps the thumb from shrinking to a single cell in a long
// conversation, where the proportional size rounds down to almost nothing.
// A one-cell thumb is hard to see and harder to aim at; two cells reads as a
// handle. Capped by the track height, so short tracks are unaffected.
const minThumbSize = 2

// ScrollbarGeometry describes where a scrollbar's thumb sits within its
// track. It is what [Scrollbar] draws from and what mouse handlers hit-test
// against, so a thumb the user grabs is always the thumb they can see.
type ScrollbarGeometry struct {
	// ThumbPos is the thumb's first row, counted from the top of the track.
	ThumbPos int
	// ThumbSize is the thumb's height in rows, at least 1.
	ThumbSize int
	// TrackSpace is how many rows the thumb can travel: the track height
	// less the thumb's own height. Zero when the thumb fills the track.
	TrackSpace int
	// MaxOffset is the largest content offset the track maps to.
	MaxOffset int
}

// ScrollbarLayout computes the thumb geometry for a scrollbar of the given
// height. The second return is false when there is nothing to scroll, in
// which case no scrollbar is drawn and none can be grabbed.
func ScrollbarLayout(height, contentSize, viewportSize, offset int) (ScrollbarGeometry, bool) {
	if height <= 0 || contentSize <= viewportSize {
		return ScrollbarGeometry{}, false
	}
	maxOffset := contentSize - viewportSize
	if maxOffset <= 0 {
		return ScrollbarGeometry{}, false
	}

	g := ScrollbarGeometry{MaxOffset: maxOffset}
	g.ThumbSize = max(minThumbSize, min(height, height*viewportSize/contentSize))
	g.TrackSpace = height - g.ThumbSize
	if g.TrackSpace > 0 {
		g.ThumbPos = min(g.TrackSpace, max(0, offset)*g.TrackSpace/maxOffset)
	}
	return g, true
}

// OffsetForThumbPos inverts the thumb placement in [ScrollbarLayout]: it
// returns the content offset that puts the thumb's top edge at row pos. The
// division rounds to nearest so a thumb dragged to a row and read back lands
// on that same row rather than drifting a row up over a long drag.
func (g ScrollbarGeometry) OffsetForThumbPos(pos int) int {
	if g.TrackSpace <= 0 {
		return 0
	}
	pos = min(g.TrackSpace, max(0, pos))
	return min(g.MaxOffset, (pos*g.MaxOffset+g.TrackSpace/2)/g.TrackSpace)
}

// Scrollbar renders a vertical scrollbar based on content and viewport size.
// Returns an empty string if content fits within viewport (no scrolling needed).
func Scrollbar(s *styles.Styles, height, contentSize, viewportSize, offset int) string {
	g, ok := ScrollbarLayout(height, contentSize, viewportSize, offset)
	if !ok {
		return ""
	}
	return ScrollbarFromLayout(s, height, g)
}

// ScrollbarFromLayout renders a scrollbar from geometry already computed by
// [ScrollbarLayout]. Callers that need the geometry for themselves — mouse
// hit-testing, say — use this so the content and offset measurements behind
// it are not paid for twice per frame.
func ScrollbarFromLayout(s *styles.Styles, height int, g ScrollbarGeometry) string {
	if height <= 0 || g.ThumbSize <= 0 {
		return ""
	}

	// Build the scrollbar.
	var sb strings.Builder
	for i := range height {
		if i > 0 {
			sb.WriteString("\n")
		}
		if i >= g.ThumbPos && i < g.ThumbPos+g.ThumbSize {
			sb.WriteString(s.Dialog.ScrollbarThumb.Render(styles.ScrollbarThumb))
		} else {
			sb.WriteString(s.Dialog.ScrollbarTrack.Render(styles.ScrollbarTrack))
		}
	}

	return sb.String()
}
