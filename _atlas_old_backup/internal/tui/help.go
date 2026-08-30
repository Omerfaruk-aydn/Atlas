package tui

import (
	"fmt"
	"strings"
)

// Hotkey is one entry in the /help popover. Platform lets us scope a
// binding to a specific class — for now we only have "all", "mac", and
// "remote" (SSH). The render step only shows the entries that match the
// current platform.
type Hotkey struct {
	Keys    string
	Desc    string
	Platform string // "" = show everywhere; "mac" / "remote" = conditional
}

// helpHintPairs is the canonical hotkey table for the /help popover and
// the /help transcript. Mirrors Hermes's hotkeys.ts layout: two columns
// (label / description), with the label column padded to the longest
// label across BOTH columns so the alignment stays consistent.
var helpHintPairs = []Hotkey{
	{Keys: "Enter", Desc: "Mesaj gönder", Platform: ""},
	{Keys: "↑/↓", Desc: "Geçmiş / öneri / kuyruk gezin", Platform: ""},
	{Keys: "Tab", Desc: "Öneriyi uygula", Platform: ""},
	{Keys: "Shift+Enter", Desc: "Yeni satır (çok satırlı taslak)", Platform: ""},
	{Keys: "Esc Esc", Desc: "Taslağı sil (↑ ile geri çağır)", Platform: ""},
	{Keys: "Ctrl+C", Desc: "Önce: turu iptal et · sonra: taslağı sil · sonra: çık", Platform: ""},
	{Keys: "Ctrl+L", Desc: "Yeniden çiz", Platform: ""},
	{Keys: "?", Desc: "Bu yardım popover'ı", Platform: ""},
	{Keys: "1-9", Desc: "Öneri listesinde hızlı seçim", Platform: ""},
	{Keys: "Esc", Desc: "Öneriyi kapat", Platform: ""},
	{Keys: "/", Desc: "Komut yazmaya başla", Platform: ""},
}

// renderHelpHint paints the "?" popup: a bordered round box, two-column
// layout (Common Commands | Hotkeys) with the label column padded to
// the longest key across BOTH columns so the alignment reads cleanly.
func (a *App) renderHelpHint() string {
	cmds, _ := a.slash.Grouped()
	// Build a flat list of (key, desc) for the commands column, prefixed
	// with "/".
	var cmdRows [][2]string
	for _, g := range cmds {
		for _, c := range a.slash.byGroup[g] {
			cmdRows = append(cmdRows, [2]string{"/" + c.Name, c.Help})
		}
	}
	var kbdRows [][2]string
	for _, h := range helpHintPairs {
		kbdRows = append(kbdRows, [2]string{h.Keys, h.Desc})
	}

	// Compute the label column width: longest label across both lists.
	labelW := 0
	for _, r := range cmdRows {
		if len(r[0]) > labelW {
			labelW = len(r[0])
		}
	}
	for _, r := range kbdRows {
		if len(r[0]) > labelW {
			labelW = len(r[0])
		}
	}
	labelW += 2 // 1 separator + 1 trailing space

	// Lay out the two columns side by side. On narrow widths we stack
	// them vertically so neither column gets squashed.
	width := a.width - 4
	if width < 30 {
		width = 30
	}
	colW := (width - 3) / 2 // 3 = inter-column gutter
	if colW < 14 {
		colW = 14
	}

	left := renderAlignedColumn(cmdRows, labelW, colW-labelW-1, "Komutlar")
	right := renderAlignedColumn(kbdRows, labelW, colW-labelW-1, "Kısayollar")
	body := strings.Join([]string{left, right}, strings.Repeat(" ", 3))

	boxed := a.theme.InputBox.Width(width).Render(body)
	return boxed
}

// renderAlignedColumn lays out one side of the /help popup: a header line
// then "(label, description)" rows, with the label column padded to
// labelW and description word-wrapped to colW-labelW.
func renderAlignedColumn(rows [][2]string, labelW, colW int, header string) string {
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n")
	if colW < 8 {
		colW = 8
	}
	for _, r := range rows {
		label := r[0]
		if len(label) > labelW-1 {
			label = label[:labelW-1]
		}
		desc := wrapToWidth(r[1], colW)
		first := true
		for _, line := range strings.Split(desc, "\n") {
			if first {
				fmt.Fprintf(&b, "%-*s%s\n", labelW, label, line)
				first = false
			} else {
				fmt.Fprintf(&b, "%-*s%s\n", labelW, "", line)
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// wrapToWidth is a small word-wrapping helper (not ANSI-aware; inputs are
// plain help text). Used by /help's two-column description wrap.
func wrapToWidth(s string, w int) string {
	if w <= 0 || len(s) <= w {
		return s
	}
	var b strings.Builder
	words := strings.Fields(s)
	col := 0
	for i, w0 := range words {
		if i > 0 && col+1+len(w0) > w {
			b.WriteString("\n")
			col = 0
		} else if i > 0 {
			b.WriteString(" ")
			col++
		}
		b.WriteString(w0)
		col += len(w0)
	}
	return b.String()
}
