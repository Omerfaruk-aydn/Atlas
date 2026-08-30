package tui

import (
	"fmt"
	"runtime/debug"
	"strings"
)

// ErrorOverview is the crash screen — a bordered round box showing the
// panic message and a few source lines around the crash site, with the
// offending line highlighted. Atlas doesn't yet ship a real source
// reader (Hermes uses `stack-utils` + `code-excerpt`), so the
// implementation falls back to: stack trace + the panic value
// highlighted in error color.
type ErrorOverview struct {
	Err   error
	Stack string
	Hint  string
}

// renderErrorOverview paints the crash screen. If Stack is empty, the
// runtime stack is captured here (so the caller doesn't have to
// remember to set it).
func (a *App) renderErrorOverview(e ErrorOverview) string {
	width := a.width - 4
	if width < 40 {
		width = 40
	}
	stack := e.Stack
	if stack == "" {
		stack = string(debug.Stack())
	}
	var b strings.Builder
	b.WriteString(a.theme.ErrorText.Render("✗ bir hata oluştu"))
	b.WriteString("\n\n")
	if e.Err != nil {
		b.WriteString(a.theme.Title.Render(e.Err.Error()))
		b.WriteString("\n\n")
	}
	// Stack frame highlighting: split by line, mark the top 12 frames.
	lines := strings.Split(stack, "\n")
	for i, line := range lines {
		if i >= 16 {
			b.WriteString(a.theme.HelpText.Render("  …"))
			break
		}
		// Indent function/line info; bold the function name.
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "goroutine") {
			b.WriteString(a.theme.HelpText.Render(line))
		} else if strings.Contains(line, ".go:") {
			// Source location: highlight the file:line.
			b.WriteString(a.theme.HelpText.Render("  "))
			b.WriteString(a.theme.UserLabel.Render(trimmed))
		} else {
			b.WriteString(a.theme.HelpText.Render("  " + trimmed))
		}
		b.WriteString("\n")
	}
	if e.Hint != "" {
		b.WriteString("\n")
		b.WriteString(a.theme.HelpText.Render(e.Hint))
	}
	return a.theme.ApprovalBox.Width(width).Render(b.String())
}

// capturePanic renders a recovered panic as an ErrorOverview. Useful
// for wrapping Bubbletea program.Run() with a defer/recover so a
// crash shows the styled screen instead of dumping to stderr.
func capturePanic(r any) ErrorOverview {
	if r == nil {
		return ErrorOverview{}
	}
	err, ok := r.(error)
	if !ok {
		err = fmt.Errorf("%v", r)
	}
	return ErrorOverview{Err: err}
}
