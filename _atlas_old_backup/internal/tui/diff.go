package tui

import (
	"fmt"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// renderDiff produces a unified-style, line-colored diff of oldText vs
// newText: added lines in green, removed lines in red, unchanged lines
// dimmed. Uses line-mode diffing (via DiffLinesToChars) so output reads as
// whole changed lines rather than a noisy character-level diff.
func (a *App) renderDiff(oldText, newText string) string {
	dmp := diffmatchpatch.New()
	a1, b1, lines := dmp.DiffLinesToChars(oldText, newText)
	diffs := dmp.DiffMain(a1, b1, false)
	diffs = dmp.DiffCharsToLines(diffs, lines)

	const contextLines = 2

	var b strings.Builder
	first := true
	writeRaw := func(s string) {
		if !first {
			b.WriteString("\n")
		}
		first = false
		b.WriteString(s)
	}
	writeLine := func(prefix, content string, style func(...string) string) {
		for _, line := range strings.Split(strings.TrimSuffix(content, "\n"), "\n") {
			writeRaw(style(prefix + line))
		}
	}

	for _, d := range diffs {
		if d.Text == "" {
			continue
		}
		switch d.Type {
		case diffmatchpatch.DiffInsert:
			writeLine("+ ", d.Text, a.theme.DiffAdd.Render)
		case diffmatchpatch.DiffDelete:
			writeLine("- ", d.Text, a.theme.DiffRemove.Render)
		case diffmatchpatch.DiffEqual:
			lines := strings.Split(strings.TrimSuffix(d.Text, "\n"), "\n")
			if len(lines) <= contextLines*2+1 {
				for _, line := range lines {
					writeRaw(a.theme.HelpText.Render("  " + line))
				}
				continue
			}
			for _, line := range lines[:contextLines] {
				writeRaw(a.theme.HelpText.Render("  " + line))
			}
			writeRaw(a.theme.HelpText.Render(fmt.Sprintf("  … %d satır değişmedi …", len(lines)-contextLines*2)))
			for _, line := range lines[len(lines)-contextLines:] {
				writeRaw(a.theme.HelpText.Render("  " + line))
			}
		}
	}

	return b.String()
}
