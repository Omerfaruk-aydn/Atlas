// Package appenv reads this program's own environment variables under both the
// name it uses now and the name it used before it was rebranded.
//
// The variables were originally prefixed CRUSH_. Dropping that prefix would
// silently ignore every shell profile, CI job, and launcher script that already
// sets it, so both are read: the current prefix wins, the legacy one is used
// when it is the only one set.
package appenv

import "os"

const (
	// Prefix is the current prefix for this program's environment variables.
	Prefix = "ATLAS_"
	// LegacyPrefix is the prefix used before the rebrand.
	LegacyPrefix = "CRUSH_"
)

// Lookup returns the value of the variable with the given suffix — the part
// after the prefix, e.g. "GLOBAL_CONFIG" — and whether it was set under either
// prefix.
func Lookup(suffix string) (string, bool) {
	if v, ok := os.LookupEnv(Prefix + suffix); ok {
		return v, true
	}
	return os.LookupEnv(LegacyPrefix + suffix)
}

// Get is [Lookup] for callers that treat unset and empty alike.
func Get(suffix string) string {
	v, _ := Lookup(suffix)
	return v
}
