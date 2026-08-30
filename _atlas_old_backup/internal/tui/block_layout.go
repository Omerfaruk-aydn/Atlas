package tui

import "strings"

// BlockGroup enumerates the 8 visual bands a transcript message can
// belong to. The grouping is the unit hasLeadGap reasons about — a new
// block only gets a leading blank line if its group differs from the
// immediately preceding rendered block's group.
type BlockGroup int

const (
	GroupUser BlockGroup = iota
	GroupModel
	GroupTrail
	GroupNote
	GroupDiff
	GroupSlash
	GroupIntro
	GroupEvent
)

// SELF_SPACED groups already paint their own margins and so must NEVER
// get an extra leading gap from hasLeadGap. The "user" group is the
// classic case — its bubble style has top margin, so the gap algorithm
// has to know to skip it (otherwise the transcript gets double-spaced
// after every user turn).
var selfSpaced = map[BlockGroup]bool{
	GroupUser:  true,
	GroupDiff:  true,
	GroupEvent: true,
	GroupIntro: true,
	GroupSlash: true,
}

// paintsTrailingGap groups already add a blank line BELOW themselves, so
// the NEXT block must not also get a leading gap. Without this, the
// classic "user prompt → user bubble has bottom-margin + assistant gets
// leading-gap" bug would insert an extra blank line between every turn.
var paintsTrailingGap = map[BlockGroup]bool{
	GroupUser:  true,
	GroupDiff:  true,
	GroupEvent: true,
}

// messageGroup returns the visual band a chat message belongs to.
// Mirrors Hermes's blockLayout.messageGroup: kind is checked first
// (intro/panel → intro, slash → slash, event → event, diff → diff,
// trail → trail) and then falls back to role. Note that "tool" /
// "info" / "error" all map to GroupNote here — they're ambient noise,
// not structural turns.
func messageGroup(role, kind string) BlockGroup {
	switch kind {
	case "intro", "panel":
		return GroupIntro
	case "slash":
		return GroupSlash
	case "event":
		return GroupEvent
	case "diff":
		return GroupDiff
	case "trail":
		return GroupTrail
	}
	switch role {
	case "user":
		return GroupUser
	case "assistant":
		return GroupModel
	default:
		return GroupNote
	}
}

// hasLeadGap decides whether a block needs a leading blank line given the
// group of the *previous* rendered block. The rule (transcribed from
// Hermes's blockLayout.hasLeadGap): a block gets exactly one blank line
// above it iff
//
//  1. its group differs from the predecessor's group, AND
//  2. it's not in selfSpaced (i.e. it doesn't already paint its own top
//     margin), AND
//  3. the predecessor doesn't already paint a trailing gap.
//
// The decision depends ONLY on the predecessor — never on the current
// block's live/mutable state. This is the crucial property: a streaming
// assistant block computes the same gap as the same block does once
// settled, so the spacing never jumps when a turn finishes streaming.
func hasLeadGap(prev, cur BlockGroup) bool {
	if cur == prev {
		return false
	}
	if selfSpaced[cur] {
		return false
	}
	if paintsTrailingGap[prev] {
		return false
	}
	return true
}

// visibleGroup walks back over a list of messages to find the *last*
// rendered group, skipping invisible trail blocks (empty trail, or
// every-section-hidden via /details). Mirrors Hermes's prevRenderedMsg:
// a hidden/empty trail must not introduce a floating gap, must not
// double the gap after a user prompt, and must not pad space above the
// final reply.
func visibleGroup(messages []chatMessage, idx int) (BlockGroup, bool) {
	for i := idx - 1; i >= 0; i-- {
		m := messages[i]
		// Trail rows are skipped when they're invisible, regardless of
		// whether the trail-ness is recorded on the kind field or only
		// on the role field. Atlas's app uses `role: "trail"` without
		// setting kind; Hermes uses both — handle either shape.
		isTrailRow := m.role == "trail" || m.kind == "trail"
		if isTrailRow && isInvisibleTrail(m) {
			continue
		}
		return messageGroup(m.role, m.kind), true
	}
	return GroupIntro, false
}

// isInvisibleTrail returns true for a trail block that would render
// nothing — either it's literally empty, or the active details mode
// hides every section it would show. Atlas currently doesn't have a
// /details toggle, so this collapses to "empty body" only; the function
// is left as a seam for that future addition.
func isInvisibleTrail(m chatMessage) bool {
	if m.role != "trail" {
		return false
	}
	return strings.TrimSpace(m.text) == ""
}
