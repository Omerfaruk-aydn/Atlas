// Package appenv reads this program's own environment variables.
//
// The variables are prefixed ATLAS_AGENT_. (The project was called
// "crush" before this rebrand; the old CRUSH_-prefixed env vars and
// on-disk filenames from a prior install are NOT read any more, per
// the no-legacy decision.)
package appenv

import "os"

// Prefix is the prefix every environment variable this program
// defines shares. Callers add the suffix that names a specific
// variable, e.g. Prefix + "GLOBAL_CONFIG".
const Prefix = "ATLAS_AGENT_"

// Lookup returns the value of the variable Prefix+suffix, and whether
// it was set.
func Lookup(suffix string) (string, bool) {
	return os.LookupEnv(Prefix + suffix)
}

// Get is [Lookup] for callers that treat unset and empty alike.
func Get(suffix string) string {
	v, _ := Lookup(suffix)
	return v
}
