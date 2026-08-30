package tui

import "testing"

// mathUnicodeConvert: symbols, sub/super, and \frac parenthesization.
func TestMathUnicodeGreekLowercase(t *testing.T) {
	if got := mathUnicodeConvert(`\alpha + \beta`); got != "α + β" {
		t.Errorf("got %q, want %q", got, "α + β")
	}
}

func TestMathUnicodeSubSuperscript(t *testing.T) {
	if got := mathUnicodeConvert(`x^2 + y_1`); got != "x² + y₁" {
		t.Errorf("got %q, want %q", got, "x² + y₁")
	}
}

func TestMathUnicodeFracParenthesizes(t *testing.T) {
	if got := mathUnicodeConvert(`\frac{a+b}{c}`); got != "(a+b)/c" {
		t.Errorf("got %q, want %q", got, "(a+b)/c")
	}
	if got := mathUnicodeConvert(`\frac{a}{b}`); got != "a/b" {
		t.Errorf("got %q, want %q", got, "a/b")
	}
}

func TestMathUnicodeBoxedSentinel(t *testing.T) {
	got := mathUnicodeConvert(`\boxed{x = 1}`)
	want := "\x01x = 1\x02"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMathUnicodeXArrow(t *testing.T) {
	if got := mathUnicodeConvert(`A \xrightarrow{f} B`); got != "A ─f→ B" {
		t.Errorf("got %q, want %q", got, "A ─f→ B")
	}
}
