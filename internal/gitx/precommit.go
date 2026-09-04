package gitx

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// PreCommitFinding is one issue spotted in the staged changes.
type PreCommitFinding struct {
	// Kind is one of "merge-conflict-marker", "debug-statement",
	// "large-file".
	Kind    string
	File    string
	Line    int // Zero when the finding is about the whole file, not a line.
	Message string
}

// PreCommitOptions narrows a check.
type PreCommitOptions struct {
	// MaxFileBytes flags a staged file larger than this. Zero uses a
	// 1 MiB default -- big enough that ordinary source and config files
	// never trip it, small enough to catch an accidentally staged
	// binary, dump, or dependency directory.
	MaxFileBytes int64
}

// PreCommitResult is the outcome of a check.
type PreCommitResult struct {
	Findings    []PreCommitFinding
	FilesStaged int
}

const defaultMaxFileBytes = 1 << 20 // 1 MiB

// debugStatementPattern matches the handful of statements that are
// legitimate during development and never meant to survive a commit,
// across the languages this agent is likely to touch. It intentionally
// does not try to catch every logging call -- a real log statement is not
// a defect, and only the small set of forms below are used exclusively
// for throwaway debugging.
var debugStatementPattern = regexp.MustCompile(`\b(console\.log|debugger|dbg!|binding\.pry|pdb\.set_trace|fmt\.Println)\s*\(|^\s*debugger\s*;`)

var mergeConflictMarker = regexp.MustCompile(`^(<{7}|={7}|>{7})(\s|$)`)

// PreCommitCheck inspects the currently staged changes for the kind of
// mistake that is obvious in hindsight and easy to miss under review:
// leftover merge-conflict markers, a debug print left in on purpose to
// see one value and never taken back out, or a large file staged by
// accident.
//
// It only reads `git diff --cached`; it never touches the index or the
// working tree.
func PreCommitCheck(ctx context.Context, dir string, opts PreCommitOptions) (PreCommitResult, error) {
	maxBytes := opts.MaxFileBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxFileBytes
	}

	// git diff --cached falls back to its no-index mode outside a
	// repository and reports a plain usage error rather than "not a git
	// repository", so that check has to happen up front here instead of
	// relying on Run's usual error classification.
	if !IsRepository(ctx, dir) {
		return PreCommitResult{}, ErrNotARepository
	}

	diff, err := GetDiff(ctx, dir, DiffOptions{Staged: true, WithPatch: true})
	if err != nil {
		return PreCommitResult{}, err
	}

	result := PreCommitResult{FilesStaged: len(diff.Files)}
	for _, f := range diff.Files {
		if f.Binary {
			result.Findings = append(result.Findings, checkLargeFile(dir, f.Path, maxBytes)...)
			continue
		}
		result.Findings = append(result.Findings, scanAddedLines(f.Path, f.Patch)...)
		result.Findings = append(result.Findings, checkLargeFile(dir, f.Path, maxBytes)...)
	}

	sort.SliceStable(result.Findings, func(i, j int) bool {
		if result.Findings[i].File != result.Findings[j].File {
			return result.Findings[i].File < result.Findings[j].File
		}
		return result.Findings[i].Line < result.Findings[j].Line
	})
	return result, nil
}

// scanAddedLines walks a unified diff's added lines only -- a marker or a
// debug print that was already there and merely moved is not something
// this commit introduced.
func scanAddedLines(path, patch string) []PreCommitFinding {
	if patch == "" {
		return nil
	}
	var findings []PreCommitFinding
	lineNo := 0
	for _, raw := range strings.Split(patch, "\n") {
		if strings.HasPrefix(raw, "@@") {
			lineNo = hunkStartLine(raw)
			continue
		}
		if strings.HasPrefix(raw, "+") && !strings.HasPrefix(raw, "+++") {
			content := raw[1:]
			if mergeConflictMarker.MatchString(content) {
				findings = append(findings, PreCommitFinding{
					Kind: "merge-conflict-marker", File: path, Line: lineNo,
					Message: "an unresolved merge-conflict marker is staged",
				})
			}
			if debugStatementPattern.MatchString(content) {
				findings = append(findings, PreCommitFinding{
					Kind: "debug-statement", File: path, Line: lineNo,
					Message: "a debug print or breakpoint looks like it was left in",
				})
			}
			lineNo++
		} else if !strings.HasPrefix(raw, "-") {
			lineNo++
		}
	}
	return findings
}

// hunkStartLine reads the post-image starting line number out of a
// "@@ -a,b +c,d @@" hunk header.
func hunkStartLine(header string) int {
	_, after, ok := strings.Cut(header, "+")
	if !ok {
		return 0
	}
	numPart, _, _ := strings.Cut(after, ",")
	numPart, _, _ = strings.Cut(numPart, " ")
	n := 0
	for _, r := range numPart {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func checkLargeFile(dir, path string, maxBytes int64) []PreCommitFinding {
	info, err := os.Stat(filepath.Join(dir, path))
	if err != nil || info.IsDir() || info.Size() <= maxBytes {
		return nil
	}
	return []PreCommitFinding{{
		Kind:    "large-file",
		File:    path,
		Message: fmt.Sprintf("staged file is %.1f MiB, over the %.1f MiB threshold", float64(info.Size())/(1<<20), float64(maxBytes)/(1<<20)),
	}}
}
