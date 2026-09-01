package image

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"io"
	"log/slog"
	"strings"
	"sync"

	tea "github.com/maincodss/atlas-agent/internal/deps/bubbletea/v2"
	"github.com/maincodss/atlas-agent/internal/ui/util"
	"github.com/maincodss/atlas-agent/internal/deps/cb/x/ansi"
	"github.com/maincodss/atlas-agent/internal/deps/cb/x/ansi/kitty"
	"github.com/disintegration/imaging"
)

// TransmittedMsg is a message indicating that an image has been transmitted to
// the terminal.
type TransmittedMsg struct {
	ID string
}

// Encoding represents the encoding format of the image.
type Encoding byte

// Image encodings.
const (
	EncodingBlocks Encoding = iota
	EncodingKitty
)

// halfBlock is the upper half block glyph. Drawn with a foreground and a
// background color it paints two stacked pixels in one cell, which is what
// lets [EncodingBlocks] reach a square pixel: a terminal cell is about twice
// as tall as it is wide, so half of one is roughly square.
const halfBlock = "▀"

type imageKey struct {
	id   string
	cols int
	rows int
}

// Hash returns a hash value for the image key.
// This uses FNV-32a for simplicity and speed.
func (k imageKey) Hash() uint32 {
	h := fnv.New32a()
	_, _ = io.WriteString(h, k.ID())
	return h.Sum32()
}

// ID returns a unique string representation of the image key.
func (k imageKey) ID() string {
	return fmt.Sprintf("%s-%dx%d", k.id, k.cols, k.rows)
}

// CellSize represents the size of a single terminal cell in pixels.
type CellSize struct {
	Width, Height int
}

type cachedImage struct {
	img        image.Image
	cols, rows int
}

var (
	cachedImages = map[imageKey]cachedImage{}
	cachedMutex  sync.RWMutex
)

// ResetCache clears the image cache, freeing all cached decoded images.
func ResetCache() {
	cachedMutex.Lock()
	clear(cachedImages)
	cachedMutex.Unlock()
}

// fitImage resizes the image to fit within the specified dimensions in
// terminal cells, maintaining the aspect ratio.
func fitImage(id string, img image.Image, cs CellSize, cols, rows int) image.Image {
	if img == nil {
		return nil
	}

	key := imageKey{id: id, cols: cols, rows: rows}

	cachedMutex.RLock()
	cached, ok := cachedImages[key]
	cachedMutex.RUnlock()
	if ok {
		return cached.img
	}

	if cs.Width == 0 || cs.Height == 0 {
		return img
	}

	maxWidth := cols * cs.Width
	maxHeight := rows * cs.Height

	img = imaging.Fit(img, maxWidth, maxHeight, imaging.Lanczos)

	cachedMutex.Lock()
	cachedImages[key] = cachedImage{
		img:  img,
		cols: cols,
		rows: rows,
	}
	cachedMutex.Unlock()

	return img
}

// HasTransmitted checks if the image with the given ID has already been
// transmitted to the terminal.
func HasTransmitted(id string, cols, rows int) bool {
	key := imageKey{id: id, cols: cols, rows: rows}

	cachedMutex.RLock()
	_, ok := cachedImages[key]
	cachedMutex.RUnlock()
	return ok
}

// Transmit transmits the image data to the terminal if needed. This is used to
// cache the image on the terminal for later rendering.
func (e Encoding) Transmit(id string, img image.Image, cs CellSize, cols, rows int, tmux bool) tea.Cmd {
	if img == nil {
		return nil
	}

	key := imageKey{id: id, cols: cols, rows: rows}

	cachedMutex.RLock()
	_, ok := cachedImages[key]
	cachedMutex.RUnlock()
	if ok {
		return nil
	}

	cmd := func() tea.Msg {
		if e != EncodingKitty {
			cachedMutex.Lock()
			cachedImages[key] = cachedImage{
				img:  img,
				cols: cols,
				rows: rows,
			}
			cachedMutex.Unlock()
			return TransmittedMsg{ID: key.ID()}
		}

		var buf bytes.Buffer
		img := fitImage(id, img, cs, cols, rows)
		bounds := img.Bounds()
		imgWidth := bounds.Dx()
		imgHeight := bounds.Dy()
		imgID := int(key.Hash())
		if err := kitty.EncodeGraphics(&buf, img, &kitty.Options{
			ID:               imgID,
			Action:           kitty.TransmitAndPut,
			Transmission:     kitty.Direct,
			Format:           kitty.RGBA,
			ImageWidth:       imgWidth,
			ImageHeight:      imgHeight,
			Columns:          cols,
			Rows:             rows,
			VirtualPlacement: true,
			Quite:            1,
			Chunk:            true,
			ChunkFormatter: func(chunk string) string {
				if tmux {
					return ansi.TmuxPassthrough(chunk)
				}
				return chunk
			},
		}); err != nil {
			slog.Error("Failed to encode image for kitty graphics", "err", err)
			return util.InfoMsg{
				Type: util.InfoTypeError,
				Msg:  "failed to encode image",
			}
		}

		return tea.RawMsg{Msg: buf.String()}
	}

	return cmd
}

// Render renders the given image within the specified dimensions using the
// specified encoding.
func (e Encoding) Render(id string, cols, rows int) string {
	key := imageKey{id: id, cols: cols, rows: rows}
	cachedMutex.RLock()
	cached, ok := cachedImages[key]
	cachedMutex.RUnlock()
	if !ok {
		return ""
	}

	img := cached.img

	switch e {
	case EncodingBlocks:
		return renderHalfBlocks(img, cols, rows)
	case EncodingKitty:
		// Build Kitty graphics unicode place holders
		var fg color.Color
		var extra int
		var r, g, b int
		hashedID := key.Hash()
		id := int(hashedID)
		extra, r, g, b = id>>24&0xff, id>>16&0xff, id>>8&0xff, id&0xff

		if id <= 255 {
			fg = ansi.IndexedColor(b)
		} else {
			fg = color.RGBA{
				R: uint8(r), //nolint:gosec
				G: uint8(g), //nolint:gosec
				B: uint8(b), //nolint:gosec
				A: 0xff,
			}
		}

		fgStyle := ansi.NewStyle().ForegroundColor(fg).String()

		var buf bytes.Buffer
		for y := range rows {
			// As an optimization, we only write the fg color sequence id, and
			// column-row data once on the first cell. The terminal will handle
			// the rest.
			buf.WriteString(fgStyle)
			buf.WriteRune(kitty.Placeholder)
			buf.WriteRune(kitty.Diacritic(y))
			buf.WriteRune(kitty.Diacritic(0))
			if extra > 0 {
				buf.WriteRune(kitty.Diacritic(extra))
			}
			for x := 1; x < cols; x++ {
				buf.WriteString(fgStyle)
				buf.WriteRune(kitty.Placeholder)
			}
			if y < rows-1 {
				buf.WriteByte('\n')
			}
		}

		return buf.String()

	default:
		return ""
	}
}

// renderHalfBlocks draws the image out of half-block glyphs: each cell carries
// two vertically stacked pixels, the upper one as the foreground color and the
// lower one as the background. That doubles the vertical resolution a cell grid
// can hold and reproduces the image's own colors, where a shape-matching ASCII
// renderer only approximates them with glyph outlines.
//
// It needs nothing but SGR color, so it is the fallback for every terminal
// without a graphics protocol — which, Kitty aside, is all of them. Sixel is
// not an option here: it paints pixels at the cursor rather than into cells,
// so it cannot survive a diff-based cell renderer.
//
// Alpha is ignored. A transparent pixel comes out as whatever its stored RGB
// is, usually black, because the encoding has no way to leave a cell
// unpainted while still using its background for the lower pixel.
func renderHalfBlocks(img image.Image, cols, rows int) string {
	if img == nil || cols <= 0 || rows <= 0 {
		return ""
	}

	// Two pixel rows per cell row. Fit preserves the aspect ratio, so a wide
	// image simply uses fewer rows than it was offered.
	fitted := imaging.Fit(img, cols, rows*2, imaging.Lanczos)
	b := fitted.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return ""
	}

	var buf strings.Builder
	for y := 0; y < h; y += 2 {
		if y > 0 {
			buf.WriteByte('\n')
		}
		for x := range w {
			top := fitted.At(b.Min.X+x, b.Min.Y+y)
			// An odd pixel height leaves the last row without a lower
			// neighbour. Reusing the upper pixel paints the cell as a solid
			// block instead of half the image sitting on the terminal
			// background.
			bottom := top
			if y+1 < h {
				bottom = fitted.At(b.Min.X+x, b.Min.Y+y+1)
			}
			buf.WriteString(ansi.NewStyle().ForegroundColor(top).BackgroundColor(bottom).String())
			buf.WriteString(halfBlock)
		}
		// Close the run so the row's last background doesn't bleed into
		// whatever the layout draws to the right of the preview.
		buf.WriteString(ansi.ResetStyle)
	}
	return buf.String()
}
