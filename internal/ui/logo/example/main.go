package main

// This is an example for testing logo treatments. Do not remove.

import (
	"fmt"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/maincodss/atlas-agent/internal/ui/logo"
	"github.com/maincodss/atlas-agent/internal/ui/styles"
	"github.com/charmbracelet/x/term"
)

func main() {
	w, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not get terminal size: %s", err)
	}

	s := styles.AtlasPantera()
	opts := logo.Opts{
		TitleColorA:  s.Logo.TitleColorA,
		TitleColorB:  s.Logo.TitleColorB,
		AtlasColor:   s.Logo.AtlasColor,
		VersionColor: s.Logo.VersionColor,
		Width:        w,
		Unstable:     true,
	}

	renderCompact := func(hyper bool) string {
		opts.Hyper = hyper
		return logo.Render(s.Logo.GradCanvas, "v1.0.0", true, opts)
	}

	renderWide := func(hyper bool) string {
		opts.Hyper = hyper
		return logo.Render(s.Logo.GradCanvas, "v1.0.0", false, opts)
	}

	lipgloss.Println(
		lipgloss.JoinHorizontal(lipgloss.Top, renderCompact(false), "  ", renderCompact(true)),
	)

	for i := range 6 {
		lipgloss.Println(renderWide(i > 0))
	}
}
