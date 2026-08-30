package tui

import (
	"regexp"
	"strings"
)

// gfmTaskListRe matches a GFM task list item at the start of a line:
// "- [ ] task text" or "- [x] task text" (case-insensitive on the
// check character).
var gfmTaskListRe = regexp.MustCompile(`(?m)^(\s*)([-*+])\s+\[([ xX])\]\s+(.*)$`)

// GFMCheckChar returns the literal char the GFM spec expects for a
// task list item's check state: ' ' (open) or 'x'/'X' (done).
func GFMCheckChar(s string) string {
	m := gfmTaskListRe.FindStringSubmatch(s)
	if len(m) < 4 {
		return ""
	}
	return m[3]
}

// GFMCheckLabel returns the human-friendly label for a GFM check
// state: "☐" for open, "☑" for done. The GFM spec says the source
// uses " " or "x"; the rendered output is up to the renderer.
func GFMCheckLabel(s string) string {
	switch strings.ToLower(GFMCheckChar(s)) {
	case "x":
		return "☑"
	default:
		return "☐"
	}
}

// renderGFMTaskList walks a markdown block and rewrites task list
// lines to use the Unicode checkbox glyphs the terminal can show.
// Non-task-list lines pass through unchanged.
func renderGFMTaskList(s string) string {
	return gfmTaskListRe.ReplaceAllStringFunc(s, func(line string) string {
		m := gfmTaskListRe.FindStringSubmatch(line)
		if len(m) != 5 {
			return line
		}
		indent, bullet, check, text := m[1], m[2], m[3], m[4]
		glyph := "☐"
		if check != " " {
			glyph = "☑"
		}
		return indent + bullet + " " + glyph + " " + text
	})
}
