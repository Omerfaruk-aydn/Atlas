package agent

// shouldAutoSummarize reports whether a session that has used tokens of a
// contextWindow-sized window is close enough to the end of it to summarize.
//
// With usedRatio unset (zero, or out of range) it keeps the built-in
// behaviour: a fixed buffer for windows large enough that a proportional one
// would be wastefully big, and a proportion of the window for smaller ones.
// A configured usedRatio replaces both -- summarize once that fraction of the
// window is used.
//
// A contextWindow of 0 means "unknown" (custom and local models often report
// nothing), and never summarizes: truncating a session on a guess is worse
// than letting the provider reject an oversized request.
func shouldAutoSummarize(contextWindow, tokens int64, usedRatio float64) bool {
	if contextWindow <= 0 {
		return false
	}
	if validUsedRatio(usedRatio) {
		return float64(tokens) >= float64(contextWindow)*usedRatio
	}

	threshold := int64(float64(contextWindow) * smallContextWindowRatio)
	if contextWindow > largeContextWindowThreshold {
		threshold = largeContextWindowBuffer
	}
	return contextWindow-tokens <= threshold
}

// validUsedRatio rejects values that would make the agent summarize on every
// turn (<= 0) or never (>= 1), falling back to the built-in thresholds rather
// than honouring a setting that can only misbehave.
func validUsedRatio(r float64) bool {
	return r > 0 && r < 1
}
