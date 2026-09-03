package agent

import "sync"

// credentialRotator round-robins between a provider's configured API keys
// (ProviderConfig.APIKey plus ProviderConfig.APIKeys) across separate
// session/model builds. Rotation happens with session affinity, matching
// the pattern OAuth account rotation uses elsewhere: a running session
// keeps whichever key buildProvider picked for it, and the NEXT session or
// model rebuild for that provider gets the next key in line.
//
// This is deliberately not a live, mid-stream failover: swapping the API
// key underneath an in-flight fantasy.LanguageModel would mean rebuilding
// its provider client, which the model-fallback chain (see model_fallback.go)
// is not wired to do. What this gives instead is real value for the common
// "one subscription's quota ran out" case: Advance is called when a 429 hits
// a provider with no further model to fall back to, so the very next turn
// or session against that provider tries a different key/account instead of
// the one that just got rate-limited.
type credentialRotator struct {
	mu   sync.Mutex
	next map[string]int
}

func newCredentialRotator() *credentialRotator {
	return &credentialRotator{next: make(map[string]int)}
}

// Pick returns the next key in round-robin order for providerID. A single
// key is returned as-is without touching the rotation state, so a provider
// with only one configured key never advances an index nobody needs.
func (r *credentialRotator) Pick(providerID string, keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	if r == nil || len(keys) == 1 {
		return keys[0]
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	idx := r.next[providerID] % len(keys)
	r.next[providerID] = idx + 1
	return keys[idx]
}

// Advance skips providerID's rotation forward by one, without needing to
// know which key was actually in use. Called when a provider's current key
// hits a 429 that the model fallback chain had nowhere further to go on,
// so the next Pick for that provider is less likely to hand back the same
// exhausted key.
func (r *credentialRotator) Advance(providerID string) {
	if r == nil || providerID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next[providerID]++
}

// candidateAPIKeys returns every API key template configured for a
// provider -- the primary APIKey field followed by any additional APIKeys
// -- skipping blanks, in the order Pick round-robins through.
func candidateAPIKeys(apiKey string, apiKeys []string) []string {
	keys := make([]string, 0, 1+len(apiKeys))
	if apiKey != "" {
		keys = append(keys, apiKey)
	}
	for _, k := range apiKeys {
		if k != "" {
			keys = append(keys, k)
		}
	}
	return keys
}
