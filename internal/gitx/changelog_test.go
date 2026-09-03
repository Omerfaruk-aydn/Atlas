package gitx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseConventionalCommitReadsTypeScopeAndDescription(t *testing.T) {
	e := ParseConventionalCommit(Commit{Subject: "feat(auth): add SSO login"})
	require.Equal(t, "feat", e.Type)
	require.Equal(t, "auth", e.Scope)
	require.Equal(t, "add SSO login", e.Description)
	require.False(t, e.Breaking)
}

func TestParseConventionalCommitHandlesNoScope(t *testing.T) {
	e := ParseConventionalCommit(Commit{Subject: "fix: correct off-by-one"})
	require.Equal(t, "fix", e.Type)
	require.Empty(t, e.Scope)
	require.Equal(t, "correct off-by-one", e.Description)
}

// Both forms mark a breaking change under Conventional Commits; treating
// only one as authoritative would silently miss real breaks.
func TestParseConventionalCommitDetectsBreakingViaBang(t *testing.T) {
	e := ParseConventionalCommit(Commit{Subject: "feat(api)!: remove legacy endpoint"})
	require.True(t, e.Breaking)
	require.Equal(t, "remove legacy endpoint", e.Description)
}

func TestParseConventionalCommitDetectsBreakingViaFooter(t *testing.T) {
	e := ParseConventionalCommit(Commit{
		Subject: "refactor: rework the client",
		Body:    "Internal cleanup.\n\nBREAKING CHANGE: Client.New now requires a context.",
	})
	require.True(t, e.Breaking)
}

// A colon in an ordinary sentence must not be read as a Conventional
// Commits type -- "invented" categories would litter every changelog.
func TestParseConventionalCommitIgnoresUnknownTypes(t *testing.T) {
	e := ParseConventionalCommit(Commit{Subject: "Merge note: nothing to see here"})
	require.Empty(t, e.Type)
	require.Equal(t, "Merge note: nothing to see here", e.Description)
}

func TestParseConventionalCommitLowercasesTheType(t *testing.T) {
	e := ParseConventionalCommit(Commit{Subject: "Feat: uppercase type"})
	require.Equal(t, "feat", e.Type)
}

func TestParseConventionalCommitHandlesAPlainSubject(t *testing.T) {
	e := ParseConventionalCommit(Commit{Subject: "just a plain message"})
	require.Empty(t, e.Type)
	require.Equal(t, "just a plain message", e.Description)
}

func sectionTitles(sections []ChangelogSection) []string {
	out := make([]string, 0, len(sections))
	for _, s := range sections {
		out = append(out, s.Title)
	}
	return out
}

func TestBuildChangelogOrdersSectionsByReaderPriority(t *testing.T) {
	entries := []ChangeEntry{
		{Type: "chore", Description: "bump deps"},
		{Type: "fix", Description: "a bug"},
		{Type: "feat", Description: "a feature"},
	}
	sections := BuildChangelog(entries)
	require.Equal(t, []string{"Features", "Fixes", "Chores"}, sectionTitles(sections))
}

// A breaking feat is still a feature; putting it only in "BREAKING
// CHANGES" and not in "Features" would make it invisible to someone
// reading the features list, and putting it in both would double-count
// on any total.
func TestBuildChangelogListsBreakingChangesAsTheirOwnSectionAndKeepsTheType(t *testing.T) {
	entries := []ChangeEntry{
		{Type: "feat", Description: "new thing", Breaking: true},
	}
	sections := BuildChangelog(entries)
	require.Equal(t, []string{"BREAKING CHANGES", "Features"}, sectionTitles(sections))
	require.Equal(t, "new thing", sections[0].Entries[0].Description)
	require.Equal(t, "new thing", sections[1].Entries[0].Description)
}

// Silently dropping non-conventional commits makes a changelog look
// complete when it is not.
func TestBuildChangelogKeepsNonConventionalCommitsInTheirOwnSection(t *testing.T) {
	entries := []ChangeEntry{
		{Description: "quick hotfix, no convention followed"},
		{Type: "feat", Description: "a feature"},
	}
	sections := BuildChangelog(entries)
	require.Contains(t, sectionTitles(sections), "Other")
}

func TestBuildChangelogOmitsEmptySections(t *testing.T) {
	sections := BuildChangelog([]ChangeEntry{{Type: "feat", Description: "x"}})
	require.Equal(t, []string{"Features"}, sectionTitles(sections))
}

func TestBuildChangelogGroupsInternalTypesUnderChores(t *testing.T) {
	entries := []ChangeEntry{
		{Type: "style", Description: "a"},
		{Type: "build", Description: "b"},
		{Type: "ci", Description: "c"},
	}
	sections := BuildChangelog(entries)
	require.Len(t, sections, 1)
	require.Equal(t, "Chores", sections[0].Title)
	require.Len(t, sections[0].Entries, 3)
}

func TestScopeCountsTalliesByScope(t *testing.T) {
	entries := []ChangeEntry{
		{Scope: "auth"}, {Scope: "auth"}, {Scope: "ui"}, {},
	}
	counts := ScopeCounts(entries)
	require.Equal(t, 2, counts["auth"])
	require.Equal(t, 1, counts["ui"])
	require.NotContains(t, counts, "")
}

func TestSortedScopesOrdersByCountThenName(t *testing.T) {
	got := SortedScopes(map[string]int{"b": 1, "a": 1, "c": 3})
	require.Equal(t, []string{"c", "a", "b"}, got)
}

func TestChangelogRangeBuildsFromRealCommits(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "chore: init")
	writeFile(t, dir, "b.txt", "two\n")
	commit(t, dir, "feat(api): add endpoint")
	writeFile(t, dir, "c.txt", "three\n")
	commit(t, dir, "fix: correct response code")

	entries, err := ChangelogRange(context.Background(), dir, "HEAD")
	require.NoError(t, err)
	require.Len(t, entries, 3)

	sections := BuildChangelog(entries)
	require.Contains(t, sectionTitles(sections), "Features")
	require.Contains(t, sectionTitles(sections), "Fixes")
}

// Merge commits carry no content of their own beyond what their
// non-merge commits already contribute, so including them would produce
// a duplicate, contentless line.
func TestChangelogRangeExcludesMergeCommits(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "base\n")
	commit(t, dir, "chore: init")
	git(t, dir, "checkout", "-b", "feature")
	writeFile(t, dir, "b.txt", "x\n")
	commit(t, dir, "feat: add thing")
	git(t, dir, "checkout", "main")
	git(t, dir, "merge", "--no-ff", "feature", "-m", "Merge branch 'feature'")

	entries, err := ChangelogRange(context.Background(), dir, "HEAD")
	require.NoError(t, err)
	for _, e := range entries {
		require.NotContains(t, e.Subject, "Merge branch")
	}
}

func TestChangelogRangeHonoursARevisionRange(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "chore: init")
	git(t, dir, "tag", "v1.0.0")
	writeFile(t, dir, "b.txt", "two\n")
	commit(t, dir, "feat: new since tag")

	entries, err := ChangelogRange(context.Background(), dir, "v1.0.0..HEAD")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "feat", entries[0].Type)
}
