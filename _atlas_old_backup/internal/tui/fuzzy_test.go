package tui

import "testing"

// Hermes tier 0: exact match on the id.
func TestFuzzyScoreExactMatchBeatsDescription(t *testing.T) {
	item := FuzzyScoreItem{ID: "model", Description: "change the LLM model"}
	if got := scoreFuzzyItem(item, "model"); got != 0 {
		t.Errorf("expected tier 0 for exact id match, got %d", got)
	}
}

// Hermes tier 1: prefix match beats tier 2 (substring).
func TestFuzzyScorePrefixBeatsSubstring(t *testing.T) {
	prefix := FuzzyScoreItem{ID: "model"}
	substring := FuzzyScoreItem{ID: "xmodel"}
	if scoreFuzzyItem(prefix, "mod") >= scoreFuzzyItem(substring, "mod") {
		t.Error("prefix match (tier 1) must beat substring (tier 2)")
	}
}

// Description match returns at +3 from its base tier — so a tier-2
// description match (tier 5) never outranks a tier-0 name match.
func TestFuzzyScoreDescriptionNeverOutranksName(t *testing.T) {
	descOnly := FuzzyScoreItem{ID: "totally-unrelated", Description: "models for you"}
	nameExact := FuzzyScoreItem{ID: "model"}
	if scoreFuzzyItem(descOnly, "model") < scoreFuzzyItem(nameExact, "model") {
		t.Errorf("description match (%d) outranked name match (%d) — should never happen",
			scoreFuzzyItem(descOnly, "model"), scoreFuzzyItem(nameExact, "model"))
	}
}

// Hermes-fuzzy: 'recaps' (exact) → 0, 'rec' (prefix) → 1, 'caps' (substring) → 2.
func TestFuzzyScoreTiersMatchHermesTest(t *testing.T) {
	item := FuzzyScoreItem{ID: "recaps"}
	if s := scoreFuzzyItem(item, "recaps"); s != 0 {
		t.Errorf("recaps exact = %d, want 0", s)
	}
	if s := scoreFuzzyItem(item, "rec"); s != 1 {
		t.Errorf("rec prefix = %d, want 1", s)
	}
	if s := scoreFuzzyItem(item, "caps"); s != 2 {
		t.Errorf("caps substring = %d, want 2", s)
	}
}

// rankFuzzy preserves original order on ties.
func TestRankFuzzyStableOnTies(t *testing.T) {
	items := []FuzzyScoreItem{
		{ID: "model"},
		{ID: "moderation"},
		{ID: "morning"},
	}
	ranked := rankFuzzy(items, "mo")
	if len(ranked) < 3 {
		t.Fatalf("expected 3 results, got %d", len(ranked))
	}
	// All three should be tier 1 (prefix); ties → original order.
	for i := 1; i < 3; i++ {
		if ranked[i].idx < ranked[i-1].idx {
			t.Errorf("rank order broken at %d", i)
		}
	}
}

// Empty query returns the input untouched (preserves browse order).
func TestRankFuzzyEmptyQueryReturnsAll(t *testing.T) {
	items := []FuzzyScoreItem{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	ranked := rankFuzzy(items, "")
	if len(ranked) != 3 {
		t.Errorf("empty query should return all items, got %d", len(ranked))
	}
	for i, r := range ranked {
		if r.idx != i {
			t.Errorf("at position %d: expected idx %d, got %d", i, i, r.idx)
		}
	}
}
