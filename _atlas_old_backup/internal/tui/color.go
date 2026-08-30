package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// parseHex parses "#rrggbb" (or "rrggbb") into 0-255 channel values.
func parseHex(hex string) (r, g, b float64) {
	h := strings.TrimPrefix(hex, "#")
	if len(h) != 6 {
		return 0, 0, 0
	}
	ri, _ := strconv.ParseInt(h[0:2], 16, 0)
	gi, _ := strconv.ParseInt(h[2:4], 16, 0)
	bi, _ := strconv.ParseInt(h[4:6], 16, 0)
	return float64(ri), float64(gi), float64(bi)
}

func toHex(r, g, b float64) string {
	clamp := func(v float64) int {
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return int(v + 0.5)
	}
	return fmt.Sprintf("#%02x%02x%02x", clamp(r), clamp(g), clamp(b))
}

// Mix blends two hex colors in sRGB space (like CSS color-mix): t=0 is a,
// t=1 is b.
func Mix(a, b string, t float64) string {
	ar, ag, ab := parseHex(a)
	br, bg, bb := parseHex(b)
	return toHex(
		ar+(br-ar)*t,
		ag+(bg-ag)*t,
		ab+(bb-ab)*t,
	)
}

// Desaturate pulls a color toward its own perceptual gray by amount (0-1).
func Desaturate(hex string, amount float64) string {
	r, g, b := parseHex(hex)
	gray := 0.2126*r + 0.7152*g + 0.0722*b
	return Mix(hex, toHex(gray, gray, gray), amount)
}

func srgbToLinear(c float64) float64 {
	c = c / 255
	if c <= 0.03928 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// RelativeLuminance computes WCAG relative luminance (0-1).
func RelativeLuminance(hex string) float64 {
	r, g, b := parseHex(hex)
	return 0.2126*srgbToLinear(r) + 0.7152*srgbToLinear(g) + 0.0722*srgbToLinear(b)
}

// ContrastRatio computes the WCAG contrast ratio between two colors (1-21).
func ContrastRatio(a, b string) float64 {
	la, lb := RelativeLuminance(a)+0.05, RelativeLuminance(b)+0.05
	if la < lb {
		la, lb = lb, la
	}
	return la / lb
}

// LiftForContrast nudges fg toward black or white (whichever increases
// contrast against bg) in 10% steps, re-mixing from the original fg each
// step, until minRatio is met or fg has been fully pushed to the pole.
// This keeps hue decaying gracefully instead of overshooting straight to
// pure black/white the moment a threshold is crossed.
func LiftForContrast(fg, bg string, minRatio float64) string {
	if ContrastRatio(fg, bg) >= minRatio {
		return fg
	}
	pole := "#000000"
	if RelativeLuminance(bg) < 0.5 {
		pole = "#ffffff"
	}
	result := fg
	for step := 1; step <= 10; step++ {
		result = Mix(fg, pole, float64(step)*0.1)
		if ContrastRatio(result, bg) >= minRatio {
			return result
		}
	}
	return result
}
