package common

import (
	"image/color"

	"github.com/lucasb-eyer/go-colorful"
)

// BlendColor interpolates from base (progress 0) to accent (progress 1) in
// a perceptual (Luv) color space. Used by short-lived highlight/pulse
// effects that fade a UI element from an accent color back to its resting
// color.
func BlendColor(base, accent color.Color, progress float64) color.Color {
	if progress <= 0 {
		return base
	}
	if progress >= 1 {
		return accent
	}
	bc, ok1 := colorful.MakeColor(base)
	ac, ok2 := colorful.MakeColor(accent)
	if !ok1 || !ok2 {
		return base
	}
	return bc.BlendLuv(ac, progress)
}
