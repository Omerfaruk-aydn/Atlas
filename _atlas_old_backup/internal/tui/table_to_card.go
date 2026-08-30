package tui

import (
	"strings"
)

// tableToCard renders a markdown-style pipe table at the given width.
// Mirrors Hermes's 3-tier fallback:
//
//  1. table fits at ideal per-column widths → simple
//  2. table fits at minimum-word-width with proportional extra-space
//     distribution → column shrink with wrapped cells
//  3. table doesn't even fit at minimums → proportional scale + hard
//     grapheme-level breaks
//
// When the rendered height exceeds tallRowThreshold for the column
// count, OR any cell's max rendered line overflows the budget,
// falls back to a "Label: value" card layout (one row per line) so
// the user sees a usable representation instead of a clipped mess.
//
// The function operates on already-extracted column data (rather than
// raw markdown), so the caller — typically a markdown preprocessor
// that runs before Glamour — supplies the cell matrix.
type tableData struct {
	headers []string
	rows    [][]string // each row is parallel to headers
}

// renderTableToCard is the fallback "card" view of a table when the
// row width budget can't accommodate a horizontal layout. Each row
// becomes a 2-column "Label: value" block; multiple values in the
// same row stack vertically.
func renderTableToCard(t tableData, width int) string {
	if width < 16 {
		width = 16
	}
	// Compute the label column width as the longest header (clamped).
	labelW := 0
	for _, h := range t.headers {
		if w := len(h); w > labelW {
			labelW = w
		}
	}
	if labelW > width/3 {
		labelW = width / 3
	}
	if labelW < 4 {
		labelW = 4
	}
	valueW := width - labelW - 4 // 4 = ": " + indent
	if valueW < 8 {
		valueW = 8
	}
	var b strings.Builder
	for r, row := range t.rows {
		if r > 0 {
			b.WriteString("\n")
		}
		// Each cell-pair on its own pair of lines: label + value.
		for c, cell := range row {
			if c >= len(t.headers) {
				continue
			}
			label := t.headers[c]
			if c > 0 {
				b.WriteString("\n")
			}
			b.WriteString(padRight(label+":", labelW+1))
			b.WriteString(" ")
			b.WriteString(wrapToWidth(cell, valueW))
		}
	}
	return b.String()
}

// renderTableHorizontal is the preferred path: render the table as a
// proper markdown table at the given width. Returns "" if the table
// can't fit at minimum widths and the caller should fall back to
// renderTableToCard.
func renderTableHorizontal(t tableData, width int) string {
	if len(t.headers) == 0 || width < 20 {
		return ""
	}
	cols := len(t.headers)
	// Compute per-column minimum width (longest word in the column).
	minW := make([]int, cols)
	for c, h := range t.headers {
		minW[c] = maxInt(3, longestWord(h))
	}
	for _, row := range t.rows {
		for c, cell := range row {
			if c >= cols {
				continue
			}
			if w := longestWord(cell); w > minW[c] {
				minW[c] = w
			}
		}
	}
	totalMin := sumInts(minW) + 3*(cols+1) // pipes + padding
	if totalMin > width {
		return "" // can't fit even at minimums → fallback
	}
	// Distribute remaining space proportionally.
	extra := width - totalMin
	colW := make([]int, cols)
	for c := range minW {
		colW[c] = minW[c] + extra/cols
	}
	// Top off with any remainder so the row exactly fills the width.
	remainder := extra - (extra/cols)*cols
	for i := 0; i < remainder && i < cols; i++ {
		colW[i]++
	}
	var b strings.Builder
	b.WriteString(renderTableRow(t.headers, colW, width))
	b.WriteString("\n")
	b.WriteString(renderTableSeparator(colW, width))
	b.WriteString("\n")
	for _, row := range t.rows {
		b.WriteString(renderTableRow(row, colW, width))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderTableRow renders a single table row to fit colW. Cell text is
// padded to colW with spaces; values shorter than the budget get
// space-padded, values longer get truncated with "…".
func renderTableRow(cells []string, colW []int, totalWidth int) string {
	var b strings.Builder
	b.WriteString("|")
	for c, cell := range cells {
		if c >= len(colW) {
			break
		}
		w := colW[c]
		if cell == "" {
			b.WriteString(padRight("", w))
		} else if len(cell) <= w {
			b.WriteString(padRight(cell, w))
		} else {
			b.WriteString(cell[:w-1] + "…")
		}
		b.WriteString("|")
	}
	return b.String()
}

func renderTableSeparator(colW []int, totalWidth int) string {
	var b strings.Builder
	b.WriteString("|")
	for _, w := range colW {
		b.WriteString(strings.Repeat("-", w))
		b.WriteString("|")
	}
	return b.String()
}

func longestWord(s string) int {
	best := 0
	cur := 0
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' {
			if cur > best {
				best = cur
			}
			cur = 0
			continue
		}
		cur++
	}
	if cur > best {
		best = cur
	}
	return best
}

func sumInts(xs []int) int {
	s := 0
	for _, x := range xs {
		s += x
	}
	return s
}
