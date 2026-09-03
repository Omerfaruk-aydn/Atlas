package gitx

import (
	"context"
	"regexp"
	"sort"
	"strings"
)

// ChangeEntry is one commit reduced to what a changelog cares about.
type ChangeEntry struct {
	Hash    string
	Subject string
	// Type is the Conventional Commits type (feat, fix, docs, ...),
	// empty when the subject does not follow that convention.
	Type string
	// Scope is the "(scope)" part of "feat(scope): ...", when present.
	Scope string
	// Breaking reports a "!" after the type/scope, or a "BREAKING
	// CHANGE:" footer -- either form marks a breaking change under the
	// Conventional Commits spec, and treating only one of them as
	// authoritative would silently miss real breaks.
	Breaking bool
	// Description is the subject with the "type(scope): " prefix
	// stripped, or the whole subject when there was no prefix to strip.
	Description string
}

// conventionalPattern matches "type(scope)!: description" and its
// variants. Scope and "!" are both optional.
var conventionalPattern = regexp.MustCompile(
	`^(\w+)(?:\(([^)]+)\))?(!)?:\s*(.+)$`)

// knownTypes are the Conventional Commits types worth a changelog
// section. A commit whose "type" is not one of these (a colon in prose,
// for instance) is treated as unconventional rather than invented as a
// new category.
var knownTypes = map[string]bool{
	"feat": true, "fix": true, "perf": true, "revert": true,
	"docs": true, "style": true, "refactor": true, "test": true,
	"build": true, "ci": true, "chore": true,
}

// breakingFooter matches a BREAKING CHANGE footer anywhere in the body.
var breakingFooter = regexp.MustCompile(`(?m)^BREAKING[ -]CHANGE:`)

// ParseConventionalCommit reads one commit's subject and body into a
// ChangeEntry.
func ParseConventionalCommit(c Commit) ChangeEntry {
	entry := ChangeEntry{
		Hash:        c.Short,
		Subject:     c.Subject,
		Description: c.Subject,
	}

	m := conventionalPattern.FindStringSubmatch(c.Subject)
	if m != nil && knownTypes[strings.ToLower(m[1])] {
		entry.Type = strings.ToLower(m[1])
		entry.Scope = m[2]
		entry.Breaking = m[3] == "!"
		entry.Description = m[4]
	}

	if breakingFooter.MatchString(c.Body) {
		entry.Breaking = true
	}
	return entry
}

// ChangelogSection groups entries under one heading.
type ChangelogSection struct {
	Title   string
	Entries []ChangeEntry
}

// sectionOrder fixes the order sections appear in, which is the order a
// reader actually cares about: breaking changes and features before
// fixes, fixes before everything else, and purely internal types
// (chore/ci/build/style) last because they have no user-facing effect.
var sectionOrder = []struct {
	title string
	types []string
}{
	{"Features", []string{"feat"}},
	{"Fixes", []string{"fix"}},
	{"Performance", []string{"perf"}},
	{"Reverts", []string{"revert"}},
	{"Documentation", []string{"docs"}},
	{"Refactoring", []string{"refactor"}},
	{"Tests", []string{"test"}},
	{"Chores", []string{"style", "build", "ci", "chore"}},
}

// BuildChangelog groups commits into sections suitable for a CHANGELOG
// entry.
//
// Breaking changes get their own leading section built from entries
// found anywhere else, not a competing category -- a breaking feat is
// still a feature, and duplicating it would double-count the same
// commit. Non-conventional commits are kept, in their own section, rather
// than silently dropped: a changelog that omits commits look complete
// when it is not, and the omission is discovered only when someone goes
// looking for a change that is missing.
func BuildChangelog(entries []ChangeEntry) []ChangelogSection {
	var sections []ChangelogSection

	var breaking []ChangeEntry
	for _, e := range entries {
		if e.Breaking {
			breaking = append(breaking, e)
		}
	}
	if len(breaking) > 0 {
		sections = append(sections, ChangelogSection{Title: "BREAKING CHANGES", Entries: breaking})
	}

	for _, group := range sectionOrder {
		var matched []ChangeEntry
		for _, e := range entries {
			for _, t := range group.types {
				if e.Type == t {
					matched = append(matched, e)
					break
				}
			}
		}
		if len(matched) > 0 {
			sections = append(sections, ChangelogSection{Title: group.title, Entries: matched})
		}
	}

	var other []ChangeEntry
	for _, e := range entries {
		if e.Type == "" {
			other = append(other, e)
		}
	}
	if len(other) > 0 {
		sections = append(sections, ChangelogSection{Title: "Other", Entries: other})
	}

	return sections
}

// ScopeCounts tallies how many entries touched each named scope, which is
// what shows whether a release concentrated in one area or spread across
// many.
func ScopeCounts(entries []ChangeEntry) map[string]int {
	counts := map[string]int{}
	for _, e := range entries {
		if e.Scope != "" {
			counts[e.Scope]++
		}
	}
	return counts
}

// SortedScopes returns scope names ordered by count, most first.
func SortedScopes(counts map[string]int) []string {
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if counts[names[i]] != counts[names[j]] {
			return counts[names[i]] > counts[names[j]]
		}
		return names[i] < names[j]
	})
	return names
}

// ChangelogRange builds a changelog from every commit reachable in
// revRange (e.g. "v1.2.0..HEAD"), excluding merge commits -- a merge
// commit carries no content of its own, only what its non-merge commits
// already contribute.
func ChangelogRange(ctx context.Context, dir, revRange string) ([]ChangeEntry, error) {
	commits, err := Log(ctx, dir, LogOptions{
		Ref:      revRange,
		Limit:    1000,
		NoMerges: true,
	})
	if err != nil {
		return nil, err
	}
	entries := make([]ChangeEntry, 0, len(commits))
	for _, c := range commits {
		entries = append(entries, ParseConventionalCommit(c))
	}
	return entries, nil
}
