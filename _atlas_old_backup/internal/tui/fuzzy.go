package tui

import (
	"math"
	"sort"
	"strings"
)

// FuzzyScoreItem is the input shape the slash-command fuzzy scorer walks
// over. It carries the id (the canonical command name), zero or more
// aliases, an optional display label, and an optional help/description
// line that can also be matched against (at a +3 offset) so commands like
// `/clock` whose help text mentions "timer" can still be discovered when
// the user types "/timer".
type FuzzyScoreItem struct {
	ID          string
	Aliases     []string
	Label       string
	Description string
}

// scoreInfinity is the sentinel "no match at all" — distinct from a real
// low score so rankFuzzy can filter misses in O(n) before sorting.
const scoreInfinity = math.MaxInt32

// normalizeSearch lowercases and trims; we don't strip diacritics because
// slash command names are ASCII anyway, and lowercasing once is cheap
// enough to do per item.
func normalizeSearch(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// scoreField returns the match tier for a single field, or scoreInfinity
// if nothing matches. Tiers mirror Hermes's slashScoreItem exactly:
//
//	tier 0: exact match
//	tier 1: prefix match
//	tier 2: substring match
//
// A field is split by non-alphanumerics (mirroring Hermes's tokenizeSearchText
// split, simplified to just one string for the whole field) so a query
// like "session" matches "active-session" via the split's "session"
// segment, not just literal substring.
func scoreField(field, query string) int {
	if field == "" || query == "" {
		return scoreInfinity
	}
	if field == query {
		return 0
	}
	if strings.HasPrefix(field, query) {
		return 1
	}
	// Split the field on non-alphanumerics and check each segment for
	// exact/prefix/substring match — Hermes's tier scheme applied to
	// each word of multi-word command names.
	segments := splitAlphanum(field)
	for _, seg := range segments {
		if seg == query {
			return 0
		}
		if strings.HasPrefix(seg, query) {
			return 1
		}
		if strings.Contains(seg, query) {
			return 2
		}
	}
	if strings.Contains(field, query) {
		return 2
	}
	return scoreInfinity
}

// splitAlphanum splits a field on any non-[a-z0-9] character after
// lowercasing. "active-session" → ["active", "session"].
func splitAlphanum(s string) []string {
	s = strings.ToLower(s)
	var out []string
	start := 0
	for i, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// scoreFuzzyItem scores a single FuzzyScoreItem against a query. The
// returned value is the minimum across (id, all aliases, label) at their
// natural tier plus (description) at tier+3 — so a description match can
// surface a command whose name doesn't match, but never outranks a name
// match. Ties resolve by registration order (the stable index in the
// caller's slice), which the rankFuzzy helper preserves.
func scoreFuzzyItem(item FuzzyScoreItem, query string) int {
	if query == "" {
		return 0 // empty query = match-all, treat as best-tier
	}
	nameFields := []string{normalizeSearch(item.ID)}
	if item.Label != "" {
		nameFields = append(nameFields, normalizeSearch(item.Label))
	}
	for _, a := range item.Aliases {
		if a != "" {
			nameFields = append(nameFields, normalizeSearch(a))
		}
	}
	bestName := scoreInfinity
	for _, f := range nameFields {
		if s := scoreField(f, query); s < bestName {
			bestName = s
		}
	}
	best := bestName
	if item.Description != "" {
		if s := scoreField(normalizeSearch(item.Description), query); s+3 < best {
			best = s + 3
		}
	}
	return best
}

// rankedFuzzy is a (index, item, score) triple used internally to sort
// while preserving the caller's original order on ties.
type rankedFuzzy struct {
	idx   int
	item  FuzzyScoreItem
	score int
}

// rankFuzzy filters and sorts items by ascending score. An empty query
// returns the input slice untouched (preserves caller/browse order).
func rankFuzzy(items []FuzzyScoreItem, query string) []rankedFuzzy {
	if query == "" {
		out := make([]rankedFuzzy, len(items))
		for i, it := range items {
			out[i] = rankedFuzzy{idx: i, item: it, score: 0}
		}
		return out
	}
	q := normalizeSearch(query)
	out := make([]rankedFuzzy, 0, len(items))
	for i, it := range items {
		s := scoreFuzzyItem(it, q)
		if s >= scoreInfinity {
			continue
		}
		out = append(out, rankedFuzzy{idx: i, item: it, score: s})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score < out[j].score
		}
		return out[i].idx < out[j].idx
	})
	return out
}
