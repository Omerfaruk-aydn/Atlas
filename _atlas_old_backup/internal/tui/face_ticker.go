package tui

import (
	"strings"
	"time"
)

// FaceStyle is the busy indicator's frame-source family. Hermes supports
// four — kaomoji (rotating face pool + verb), emoji (6-frame emoji cycle
// + verb), ascii (|/-\\ + verb), and unicode (braille spinner, no verb,
// the spinner is the verb). Atlas mirrors the same four.
type FaceStyle int

const (
	FaceStyleKaomoji FaceStyle = iota
	FaceStyleEmoji
	FaceStyleAscii
	FaceStyleUnicode
)

// faceTickMS is the default cadence the busy face rotates at — 2500ms in
// Hermes, matches Claude Code's own pace. The unicode spinner overrides
// this with its own per-tick interval because a braille spinner reads as
// motion rather than meaning, so the verb pairing would be redundant.
const faceTickMS = 2500

// emojiFrameMS is the dedicated cadence for the emoji style — 6 frames
// at 100ms = 600ms per cycle. The face itself is the indicator; the
// verb fills the idle time so the status bar doesn't read as silent.
const emojiFrameMS = 100

// thinkingVerbsPad is the fixed width the verb string is padded to (with
// a trailing "…" attached directly to the word, not the padding) so the
// status bar doesn't jitter as the verb rotates. Sized to fit the
// longest default Turkish verb ("değerlendiriyor" = 15 chars) plus the
// 3-byte ellipsis.
const thinkingVerbsPad = 19

// thinkingVerbs is the pool of "what's the model doing" verb phrases the
// busy face pairs with. Hermes uses 15 English verbs; Atlas uses 8
// Turkish verbs with the same semantics (pondering, contemplating,
// musing, etc.).
var thinkingVerbs = []string{
	"düşünüyor",
	"değerlendiriyor",
	"araştırıyor",
	"tartıyor",
	"analiz ediyor",
	"planlıyor",
	"sentezliyor",
	"hesaplıyor",
}

// faceEmojiFrames is the 6-frame emoji cycle used by FaceStyleEmoji.
var faceEmojiFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴"}

// faceAsciiFrames is the 4-frame ascii spinner.
var faceAsciiFrames = []string{"|", "/", "-", "\\"}

// faceUnicodeSpinner is the braille spinner used by FaceStyleUnicode. The
// 8-frame dot-sweep reads as smooth rotation on every modern terminal.
var faceUnicodeSpinner = []string{"⠋", "⠙", "⠚", "⠒", "⠂", "⠂", "⠒", "⠲", "⠴", "⠦", "⠖", "⠒", "⠐", "⠐", "⠒", "⠓", "⠋"}

// faceTickerView is the stateful renderer the App calls once per render
// tick. It owns no goroutines — the tick comes from outside (Bubbletea's
// spinner.TickMsg already wired into App.Update). It only computes the
// current frame given (style, tickCount, startedAt).
type faceTickerView struct {
	style     FaceStyle
	tick      int
	startedAt time.Time
	now       time.Time
}

// View returns the rendered "face verb (elapsed)" string for the current
// tick. The trailing verb is padded to thinkingVerbsPad with a literal
// ellipsis glued to the word itself, so the visual baseline doesn't
// shift as verbs rotate.
func (f faceTickerView) View() string {
	glyph := f.glyph()
	verb := f.verb()
	elapsed := fmtDuration(f.now.Sub(f.startedAt))
	if verb == "" {
		// Unicode style: spinner is the verb, no text.
		return glyph + " " + elapsed
	}
	return glyph + " " + padVerb(verb) + " (" + elapsed + ")"
}

func (f faceTickerView) glyph() string {
	switch f.style {
	case FaceStyleEmoji:
		return faceEmojiFrames[f.tick%len(faceEmojiFrames)]
	case FaceStyleAscii:
		return faceAsciiFrames[f.tick%len(faceAsciiFrames)]
	case FaceStyleUnicode:
		return faceUnicodeSpinner[f.tick%len(faceUnicodeSpinner)]
	default:
		// Kaomoji: keep one stable kaomoji (no per-tick rotation here) —
		// the verb carries the "this is changing" signal, the face is the
		// "thinking" character. Matches Hermes's idle-face style.
		return "(•_•)"
	}
}

func (f faceTickerView) verb() string {
	if f.style == FaceStyleUnicode || f.style == FaceStyleAscii {
		// ASCII and Unicode styles still carry a verb (Hermes does this
		// too), but the ASCII spinner is so minimal that the verb
		// visually dominates; we keep the verb anyway for consistency.
		interval := int(faceTickIntervalForStyle(f.style) / time.Millisecond)
		if interval <= 0 {
			interval = 1
		}
		return thinkingVerbs[(f.tick/interval)%len(thinkingVerbs)]
	}
	if f.style == FaceStyleEmoji {
		// Emoji style ticks faster (100ms), so we rotate verb every
		// faceTickMS / emojiFrameMS = 25 ticks, not every tick.
		return thinkingVerbs[(f.tick*emojiFrameMS/faceTickMS)%len(thinkingVerbs)]
	}
	// Kaomoji: rotate every faceTickMS.
	return thinkingVerbs[f.tick%len(thinkingVerbs)]
}

func faceTickIntervalForStyle(s FaceStyle) time.Duration {
	if s == FaceStyleEmoji {
		return emojiFrameMS * time.Millisecond
	}
	return faceTickMS * time.Millisecond
}

// padVerb right-pads verb to thinkingVerbsPad-1 cells with spaces and
// appends a literal ellipsis. The ellipsis is glued to the last
// padding character so the visual baseline doesn't drift as verbs
// rotate. Verbs longer than the budget overflow the column; in
// practice none of the default verbs exceed it.
func padVerb(verb string) string {
	target := thinkingVerbsPad - 1
	if len(verb) >= target {
		return verb + "…"
	}
	return verb + strings.Repeat(" ", target-len(verb)) + "…"
}
