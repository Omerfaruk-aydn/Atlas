package image

import (
	"image"
	"image/color"
	"image/draw"
	"testing"

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
