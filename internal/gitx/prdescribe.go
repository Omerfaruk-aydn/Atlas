package gitx

import (
	"context"
	"path"
	"regexp"
	"sort"
	"strings"
)

// PRSummary is the material a pull-request description can be built
// from, gathered from the commits and diff of a branch against a base.
type PRSummary struct {
	Base    string
	Head    string
	Commits []ChangeEntry
	Diff    Diff
	// TopLevelDirs counts changed files per top-level directory, which is
	// what shows whether a change is localised or sprawling.
	TopLevelDirs map[string]int
	// HasTests reports whether any changed file looks like a test.
	HasTests bool
	// TicketRefs collects issue references found in commit subjects and
	// bodies, deduplicated.
	TicketRefs []string
}

var prTicketPattern = regexp.MustCompile(`(#\d+|[A-Z][A-Z0-9]+-\d+)`)

// SummarisePR gathers everything needed to draft a PR description for
// the range base..head.
func SummarisePR(ctx context.Context, dir, base, head string) (PRSummary, error) {
	commits, err := Log(ctx, dir, LogOptions{
		Ref:      base + ".." + head,
		Limit:    500,
		NoMerges: true,
	})
	if err != nil {
		return PRSummary{}, err
	}

	entries := make([]ChangeEntry, 0, len(commits))
	tickets := map[string]bool{}
	for _, c := range commits {
		entries = append(entries, ParseConventionalCommit(c))
		for _, m := range prTicketPattern.FindAllString(c.Subject+" "+c.Body, -1) {
			tickets[m] = true
		}
	}

	diff, err := GetDiff(ctx, dir, DiffOptions{Ref: base + ".." + head})
	if err != nil {
		return PRSummary{}, err
	}

	dirs := map[string]int{}
	hasTests := false
	for _, f := range diff.Files {
		top := topLevelDir(f.Path)
		dirs[top]++
		if looksLikeTest(f.Path) {
			hasTests = true
		}
	}

	ticketList := make([]string, 0, len(tickets))
	for t := range tickets {
		ticketList = append(ticketList, t)
	}
	sort.Strings(ticketList)

	return PRSummary{
		Base:         base,
		Head:         head,
		Commits:      entries,
		Diff:         diff,
		TopLevelDirs: dirs,
		HasTests:     hasTests,
		TicketRefs:   ticketList,
	}, nil
}

func topLevelDir(p string) string {
	p = path.Clean(p)
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return "." // A root-level file.
}

func looksLikeTest(p string) bool {
	base := strings.ToLower(path.Base(p))
	return strings.HasSuffix(base, "_test.go") ||
		strings.Contains(base, ".test.") ||
		strings.Contains(base, ".spec.") ||
		strings.HasPrefix(base, "test_")
}

// SortedDirs returns directory names ordered by how many files changed,
// most first.
func SortedDirs(counts map[string]int) []string {
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
