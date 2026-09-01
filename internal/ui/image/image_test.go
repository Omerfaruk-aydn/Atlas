package image

import (
	"image"
	"image/color"
	"image/draw"
	"strings"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-ansi"

	"github.com/stretchr/testify/require"
)

func TestResetCache(t *testing.T) {
	t.Parallel()

	cachedMutex.Lock()
	cachedImages[imageKey{id: "a", cols: 10, rows: 10}] = cachedImage{
		img:  image.NewRGBA(image.Rect(0, 0, 1, 1)),
		cols: 10,
		rows: 10,
	}
	cachedImages[imageKey{id: "b", cols: 20, rows: 20}] = cachedImage{
		img:  image.NewRGBA(image.Rect(0, 0, 1, 1)),
		cols: 20,
		rows: 20,
	}
	cachedMutex.Unlock()

	ResetCache()

	cachedMutex.RLock()
	length := len(cachedImages)
	cachedMutex.RUnlock()

	require.Equal(t, 0, length)
}

func TestResetIdempotent(t *testing.T) {
	t.Parallel()

	// Calling Reset on an empty cache should not panic.
	ResetCache()

	cachedMutex.RLock()
	length := len(cachedImages)
	cachedMutex.RUnlock()

	require.Equal(t, 0, length)
}

// solidImage returns a w×h image painted a single color.
func solidImage(w, h int, c color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: c}, image.Point{}, draw.Src)
	return img
}

// The preview is drawn into a fixed box in the chat, so the renderer must
// never hand back more rows or columns than it was given — an overflowing
// preview would push the message layout out of shape.
func TestRenderHalfBlocksFitsTheBox(t *testing.T) {
	t.Parallel()

	red := color.RGBA{R: 0xff, A: 0xff}
	for _, tc := range []struct{ w, h int }{
		{64, 64},  // square
		{200, 40}, // wide
		{40, 200}, // tall
		{3, 3},    // smaller than the box
		{101, 51}, // odd dimensions, exercising the last-row fallback
	} {
		const cols, rows = 24, 12
		got := renderHalfBlocks(solidImage(tc.w, tc.h, red), cols, rows)
		require.NotEmpty(t, got)

		lines := strings.Split(got, "\n")
		require.LessOrEqual(t, len(lines), rows, "%dx%d source overflows %d rows", tc.w, tc.h, rows)
		for i, line := range lines {
			require.LessOrEqual(t, ansi.StringWidth(line), cols,
				"%dx%d source, row %d overflows %d columns", tc.w, tc.h, i, cols)
		}
	}
}

// Each cell carries two pixels: the upper one as the foreground, the lower as
// the background. That is the whole point of the encoding — losing it would
// silently halve the vertical resolution.
func TestRenderHalfBlocksCarriesTwoPixelsPerCell(t *testing.T) {
	t.Parallel()

	// Top half red, bottom half blue, sized so one cell row covers the seam.
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	draw.Draw(img, image.Rect(0, 0, 2, 1), &image.Uniform{C: color.RGBA{R: 0xff, A: 0xff}}, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(0, 1, 2, 2), &image.Uniform{C: color.RGBA{B: 0xff, A: 0xff}}, image.Point{}, draw.Src)

	got := renderHalfBlocks(img, 2, 1)
	require.Contains(t, got, halfBlock, "the half block glyph is what splits the cell")
	require.Contains(t, got, "38;2;255;0;0", "the upper pixel becomes the foreground")
	require.Contains(t, got, "48;2;0;0;255", "the lower pixel becomes the background")
	require.True(t, strings.HasSuffix(got, ansi.ResetStyle),
		"the row must close its color run so the background does not bleed right")
}

func TestRenderHalfBlocksRejectsEmptyBoxes(t *testing.T) {
	t.Parallel()

	img := solidImage(8, 8, color.RGBA{A: 0xff})
	require.Empty(t, renderHalfBlocks(img, 0, 4))
	require.Empty(t, renderHalfBlocks(img, 4, 0))
	require.Empty(t, renderHalfBlocks(nil, 4, 4))
}
