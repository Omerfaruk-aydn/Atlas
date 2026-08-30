package tui

// DetailsMode is the per-section visibility for /details. Hermes
// supports a 3-state toggle (hidden/collapsed/expanded) with optional
// per-section overrides. Atlas's port mirrors the same shape so a
// future /details command can be added without re-architecting.
type DetailsMode int

const (
	DetailsHidden DetailsMode = iota
	DetailsCollapsed
	DetailsExpanded
)

// SectionOverrides lets the user override the global mode for one
// specific section (e.g. keep "tools" expanded even when global is
// hidden). The keys are section names: "thinking", "tools",
// "subagents", "activity".
type SectionOverrides map[string]DetailsMode

// DetailsState is the App's full details state. Global is the
// catch-all; Sections carries per-section overrides.
type DetailsState struct {
	Global   DetailsMode
	Sections SectionOverrides
}

// Default details: thinking/tools default to Expanded (so the
// transcript reads as a live process by default), activity defaults
// to Hidden (ambient gateway hints shouldn't dominate), subagents
// fall through to global.
func (d DetailsState) SectionMode(name string) DetailsMode {
	if override, ok := d.Sections[name]; ok {
		return override
	}
	switch name {
	case "thinking", "tools":
		return DetailsExpanded
	case "activity":
		return DetailsHidden
	}
	return d.Global
}

// nextDetailsMode cycles hidden → collapsed → expanded → hidden.
func nextDetailsMode(d DetailsMode) DetailsMode {
	switch d {
	case DetailsHidden:
		return DetailsCollapsed
	case DetailsCollapsed:
		return DetailsExpanded
	default:
		return DetailsHidden
	}
}

// shouldRenderTrail reports whether a trail block should be rendered
// at all, based on the active DetailsState. A trail block with no
// todos/tools/thinking, or with every section in the active detail
// mode set to hidden, renders nothing (and so doesn't introduce a
// floating gap, doesn't double the gap after a user prompt, and
// doesn't pad space above the final reply).
func shouldRenderTrail(d DetailsState, hasThinking, hasTools, hasActivity bool) bool {
	if d.Global == DetailsHidden {
		// Per-section overrides can still let some sections through.
		if hasThinking && d.SectionMode("thinking") != DetailsHidden {
			return true
		}
		if hasTools && d.SectionMode("tools") != DetailsHidden {
			return true
		}
		return false
	}
	if d.Global == DetailsCollapsed {
		// Collapsed means "header only" — render the block, not its
		// body. The actual "header only" rendering is the caller's
		// job; from this gate's perspective, the block is visible.
		return true
	}
	// Expanded: render everything unless a per-section override hides it.
	return hasThinking || hasTools || hasActivity
}
