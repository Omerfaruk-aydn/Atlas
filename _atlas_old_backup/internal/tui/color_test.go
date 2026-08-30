package tui

import (
	"math"
	"testing"
)

func TestMixEndpoints(t *testing.T) {
	if got := Mix("#ff0000", "#0000ff", 0); got != "#ff0000" {
		t.Errorf("Mix(t=0) = %s, want #ff0000", got)
	}
	if got := Mix("#ff0000", "#0000ff", 1); got != "#0000ff" {
		t.Errorf("Mix(t=1) = %s, want #0000ff", got)
	}
	if got := Mix("#000000", "#ffffff", 0.5); got != "#808080" {
		t.Errorf("Mix(t=0.5) black/white = %s, want #808080", got)
	}
}

func TestContrastRatioBlackWhite(t *testing.T) {
	ratio := ContrastRatio("#000000", "#ffffff")
	if ratio < 20.9 || ratio > 21.1 {
		t.Errorf("ContrastRatio(black, white) = %.2f, want ~21", ratio)
	}
}

func TestContrastRatioSymmetric(t *testing.T) {
	a := ContrastRatio("#333333", "#eeeeee")
	b := ContrastRatio("#eeeeee", "#333333")
	if a != b {
		t.Errorf("ContrastRatio should be symmetric, got %.4f vs %.4f", a, b)
	}
}

func TestLiftForContrastMeetsThreshold(t *testing.T) {
	// A mid-gray on a near-black background starts under-contrasted;
	// lifting should push it toward white until the ratio clears.
	fg, bg := "#444444", "#0d1117"
	before := ContrastRatio(fg, bg)
	lifted := LiftForContrast(fg, bg, 4.5)
	after := ContrastRatio(lifted, bg)

	if before >= 4.5 {
		t.Fatalf("test setup invalid: fg/bg already meets 4.5 contrast (%.2f)", before)
	}
	if after < 4.5 {
		t.Errorf("LiftForContrast did not reach the 4.5 floor: got %.2f from %s", after, lifted)
	}
}

func TestLiftForContrastNoOpWhenAlreadyReadable(t *testing.T) {
	fg, bg := "#ffffff", "#000000"
	if got := LiftForContrast(fg, bg, 4.5); got != fg {
		t.Errorf("expected no change when already above threshold, got %s", got)
	}
}

func TestDesaturateReducesColorfulness(t *testing.T) {
	vivid := "#ff0000"
	desat := Desaturate(vivid, 1.0)
	r, g, b := parseHex(desat)
	if math.Abs(r-g) > 1 || math.Abs(g-b) > 1 {
		t.Errorf("fully desaturated color should be gray, got %s", desat)
	}
}
