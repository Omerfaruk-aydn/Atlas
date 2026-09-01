package config

import (
	"strings"

	"github.com/charmbracelet/crush/internal/appenv"
	"github.com/charmbracelet/crush/internal/fsext"
)

// This program was called "crush" before it was rebranded to Atlas, and the
// names it reads off disk carried that word: crush.json, crushrc, .crush/,
// CRUSH_* in the environment. Renaming them outright would strand every
// existing installation — its config, its sessions, its database — with no
// error to explain the silence.
//
// So both names are honoured, and the rule is the same everywhere: the current
// name wins when it is present, the legacy name is used when it is the only
// one there, and anything created from scratch gets the current name. Nothing
// is moved or rewritten; an old installation simply keeps working, and a new
// one never sees the old word.
const (
	// envPrefix and legacyEnvPrefix bracket every environment variable this
	// program defines.
	envPrefix       = appenv.Prefix
	legacyEnvPrefix = appenv.LegacyPrefix
)

// legacyName rewrites a path to the names this program used before the
// rebrand. It works one path element at a time and only rewrites an element
// that *is* one of this program's own names — "atlas", ".atlas", "atlas.json",
// ".atlasrc", "atlas.db" — so a directory that merely contains the word, such
// as "atlas-projects", is left alone. A path names the program more than once
// (~/.config/atlas/atlas.json), so every element is considered, not just the
// first.
func legacyName(path string) string {
	if !strings.Contains(path, appName) {
		return path
	}
	sep := func(r rune) bool { return r == '/' || r == '\\' }
	var b strings.Builder
	b.Grow(len(path))
	start := 0
	for i, r := range path {
		if !sep(r) {
			continue
		}
		b.WriteString(legacyElement(path[start:i]))
		b.WriteRune(r)
		start = i + len(string(r))
	}
	b.WriteString(legacyElement(path[start:]))
	return b.String()
}

// legacyElement rewrites a single path element, or returns it unchanged when
// it is not one of this program's names.
func legacyElement(elem string) string {
	dot := ""
	rest := elem
	if strings.HasPrefix(rest, ".") {
		dot, rest = ".", rest[1:]
	}
	if !strings.HasPrefix(rest, appName) {
		return elem
	}
	// What follows the name must be nothing, an extension, or the "rc"
	// suffix; anything else means the element only starts with the word.
	switch suffix := rest[len(appName):]; {
	case suffix == "", suffix == "rc", strings.HasPrefix(suffix, "."):
		return dot + legacyAppName + suffix
	default:
		return elem
	}
}

// preferExisting resolves a path that may exist under either name: the current
// path when it is there, the legacy path when it is the only one, and the
// current path when neither exists so that anything created lands on the new
// name.
func preferExisting(current string) string {
	return fsext.PreferExisting(current, legacyName(current))
}

// lookupEnv reads one of this program's environment variables under its
// current name, falling back to the name it had before the rebrand. The
// suffix is the part after the prefix, e.g. "GLOBAL_CONFIG".
func lookupEnv(suffix string) (string, bool) {
	return appenv.Lookup(suffix)
}

// getEnv is [lookupEnv] for callers that treat unset and empty alike.
func getEnv(suffix string) string {
	return appenv.Get(suffix)
}

// withLegacyNames returns names followed by their pre-rebrand spellings, with
// duplicates dropped. Used where a lookup walks a list of candidate filenames
// and should find a config written under either name.
func withLegacyNames(names ...string) []string {
	out := make([]string, 0, len(names)*2)
	seen := make(map[string]bool, len(names)*2)
	for _, n := range append(names, mapLegacy(names)...) {
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

func mapLegacy(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = legacyName(n)
	}
	return out
}

// lookupDataDirectory finds a project's data directory by walking up from dir.
// The current name is searched first across the whole range, so a project that
// has both keeps the new one; only a project holding just the old directory
// falls back to it, and it keeps its sessions and database where they are.
func lookupDataDirectory(dir string) (string, bool) {
	for _, name := range withLegacyNames(defaultDataDirectory) {
		if path, ok := fsext.LookupClosestBounded(dir, projectBoundary(dir), name); ok {
			return path, true
		}
	}
	return "", false
}

// bothSpellings expands each path into its legacy and current spellings, in
// that order so the current name takes priority in a merge, with duplicates
// dropped. A path that names nothing of this program's yields just itself.
func bothSpellings(paths ...string) []string {
	out := make([]string, 0, len(paths)*2)
	seen := make(map[string]bool, len(paths)*2)
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, p := range paths {
		add(legacyName(p))
		add(p)
	}
	return out
}
