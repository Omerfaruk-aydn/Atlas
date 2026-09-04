package gitx

import (
	"context"
	"path/filepath"
	"strings"
)

// CommitTypeSuggestion is a Conventional Commits type and scope inferred
// from the files in a diff, not from anything claiming to know intent.
type CommitTypeSuggestion struct {
	// Type is a Conventional Commits type (feat, fix, docs, test, ci,
	// build, chore, refactor). Empty when the files gave no signal at
	// all worth acting on.
	Type string
	// Scope is the shared directory's last path segment, e.g. "gitx" for
	// changes confined to internal/gitx. Empty when the changed files
	// don't share one.
	Scope string
	// Confidence is "high", "medium", or "low". Only a diff where every
	// file falls into one obvious category earns "high" -- anything
	// mixed is a guess.
	Confidence string
	Rationale  string
	ByCategory map[string]int
	FilesCount int
}

var dependencyFiles = map[string]bool{
	"go.mod": true, "go.sum": true,
	"package.json": true, "package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true,
	"requirements.txt": true, "pipfile": true, "pipfile.lock": true,
	"gemfile": true, "gemfile.lock": true,
	"cargo.toml": true, "cargo.lock": true,
}

// SuggestCommitType looks at the files staged for commit and proposes a
// Conventional Commits type and scope for the message header.
//
// This reads file paths and diff stats only, never the content of a
// change -- it can tell that every touched file is a test, but it cannot
// tell "fix" from "feat" from "refactor" when the change touches ordinary
// source, because that distinction lives in what the change is for, not
// in what it touches. In that case the suggestion is deliberately
// low-confidence: a starting point to correct, not an answer to accept.
func SuggestCommitType(ctx context.Context, dir string) (CommitTypeSuggestion, error) {
	if !IsRepository(ctx, dir) {
		return CommitTypeSuggestion{}, ErrNotARepository
	}

	diff, err := GetDiff(ctx, dir, DiffOptions{Staged: true})
	if err != nil {
		return CommitTypeSuggestion{}, err
	}
	if len(diff.Files) == 0 {
		return CommitTypeSuggestion{}, nil
	}

	statuses, err := stagedFileStatuses(ctx, dir)
	if err != nil {
		statuses = nil // Degrade to a status-blind suggestion rather than failing outright.
	}

	byCategory := map[string]int{}
	var paths []string
	for _, f := range diff.Files {
		paths = append(paths, f.Path)
		byCategory[categorise(f.Path)]++
	}

	suggestion := CommitTypeSuggestion{
		ByCategory: byCategory,
		FilesCount: len(diff.Files),
		Scope:      commonScope(paths),
	}
	suggestion.Type, suggestion.Confidence, suggestion.Rationale = inferType(byCategory, len(diff.Files), statuses)
	return suggestion, nil
}

// categorise buckets one changed path by what kind of file it is, not by
// what changed inside it.
func categorise(path string) string {
	lower := strings.ToLower(path)
	base := strings.ToLower(filepath.Base(path))

	switch {
	case strings.Contains(lower, ".github/workflows/"), strings.Contains(lower, ".circleci/"), base == ".gitlab-ci.yml":
		return "ci"
	case dependencyFiles[base]:
		return "dependency"
	case strings.HasSuffix(base, "_test.go"), strings.Contains(lower, "/test/"), strings.Contains(lower, "/tests/"),
		strings.HasSuffix(base, ".test.js"), strings.HasSuffix(base, ".test.ts"), strings.HasSuffix(base, ".spec.js"), strings.HasSuffix(base, ".spec.ts"):
		return "test"
	case strings.HasSuffix(base, ".md"), strings.HasSuffix(base, ".mdx"), strings.HasPrefix(lower, "docs/"), strings.Contains(lower, "/docs/"):
		return "docs"
	case strings.HasSuffix(base, ".yml"), strings.HasSuffix(base, ".yaml"), strings.HasSuffix(base, ".toml"),
		strings.HasPrefix(base, "."):
		return "config"
	default:
		return "source"
	}
}

// inferType turns the category breakdown into a type suggestion. A diff
// that falls entirely into one non-source category is unambiguous; a
// diff that touches source code cannot be classified past "something
// changed in source" without knowing why, so that case is always
// reported at low confidence.
func inferType(byCategory map[string]int, total int, statuses map[string]byte) (typ, confidence, rationale string) {
	for category, kind := range map[string]string{
		"ci": "ci", "docs": "docs", "test": "test", "dependency": "build",
	} {
		if byCategory[category] == total {
			return kind, "high", "every changed file is " + category
		}
	}

	if byCategory["source"] == 0 && byCategory["config"] == total {
		return "chore", "medium", "every changed file is configuration, none of it source or docs"
	}

	if byCategory["source"] == 0 {
		return "chore", "low", "changed files are a mix of non-source categories"
	}

	added, deleted := 0, 0
	for _, status := range statuses {
		switch status {
		case 'A':
			added++
		case 'D':
			deleted++
		}
	}
	switch {
	case added > 0 && deleted == 0:
		return "feat", "medium", "new file(s) added alongside source changes"
	case deleted > 0 && added == 0:
		return "chore", "medium", "file(s) removed; confirm this is cleanup rather than a fix or feature"
	default:
		return "fix", "low", "source files modified with no new or removed files -- could equally be feat, fix, or refactor"
	}
}

// commonScope finds the deepest directory shared by every changed path
// and returns its last segment, mirroring the "scope" this project's own
// commits already use (feat(gitx): ..., feat(tools): ...).
func commonScope(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	dirs := make([][]string, len(paths))
	for i, p := range paths {
		dirs[i] = strings.Split(filepath.ToSlash(filepath.Dir(p)), "/")
	}

	common := dirs[0]
	for _, d := range dirs[1:] {
		common = commonPrefix(common, d)
		if len(common) == 0 {
			return ""
		}
	}
	// A one-segment common ancestor ("internal", "src") is too generic to
	// be a useful scope -- it says almost nothing about what changed.
	if len(common) < 2 {
		return ""
	}
	return common[len(common)-1]
}

func commonPrefix(a, b []string) []string {
	n := min(len(b), len(a))
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return a[:i]
}

// stagedFileStatuses maps each staged path to its one-letter git status
// (A, M, D, ...), which --numstat alone does not report.
func stagedFileStatuses(ctx context.Context, dir string) (map[string]byte, error) {
	out, err := Run(ctx, dir, "diff", "--cached", "--name-status", "--no-color")
	if err != nil {
		return nil, err
	}
	statuses := map[string]byte{}
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 || len(parts[0]) == 0 {
			continue
		}
		statuses[parts[len(parts)-1]] = parts[0][0]
	}
	return statuses, nil
}
