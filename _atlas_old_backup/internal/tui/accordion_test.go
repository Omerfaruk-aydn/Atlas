package tui

import (
	"strings"
	"testing"
)

func TestRenderAccordionCollapsedShowsOnlyHeader(t *testing.T) {
	a := &App{theme: DefaultTheme()}
	n := 3
	out := a.renderAccordion(accordion{title: "Araçlar", count: &n, open: false, body: "gizli içerik"})

	if !strings.Contains(out, "▸") {
		t.Error("expected a collapsed (▸) chevron")
	}
	if !strings.Contains(out, "(3)") {
		t.Error("expected the count to appear in the header")
	}
	if strings.Contains(out, "gizli içerik") {
		t.Error("collapsed accordion must not render its body")
	}
}

func TestRenderAccordionOpenShowsBody(t *testing.T) {
	a := &App{theme: DefaultTheme()}
	out := a.renderAccordion(accordion{title: "Araçlar", open: true, body: "görünür içerik"})

	if !strings.Contains(out, "▾") {
		t.Error("expected an expanded (▾) chevron")
	}
	if !strings.Contains(out, "görünür içerik") {
		t.Error("expanded accordion must render its body")
	}
}
