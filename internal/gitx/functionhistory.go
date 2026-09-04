package gitx

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// FunctionHistoryEntry is one commit that touched a specific function,
// carrying only the diff hunk for that function -- not the whole
// commit's patch.
type FunctionHistoryEntry struct {
	Hash    string
	Short   string
	Author  string
	Date    string // Git's own default date format; kept as-is rather than reparsed.
	Subject string
	// Diff is the unified diff for this function alone, as git's -L mode
	// produces it.
	Diff string
}

// ErrFunctionNotFound is returned when git's line-history search cannot
// locate the given function in the file.
var ErrFunctionNotFound = errors.New("function not found in file")

// FunctionHistory walks a file's history through the lens of one
// function, using git's own `-L :function:file` line-history mode. Each
// entry is a commit that changed lines within that function, most recent
// first, with the diff scoped to just that region -- the same view `git
// log -L` gives at the command line, structured for a caller that isn't
// a terminal.
func FunctionHistory(ctx context.Context, dir, path, symbol string, limit int) ([]FunctionHistoryEntry, error) {
	args := []string{"log", "--no-color", "-L", fmt.Sprintf(":%s:%s", symbol, path)}
	if limit > 0 {
		args = append(args, "-n", strconv.Itoa(limit))
	}

	out, err := Run(ctx, dir, args...)
	if err != nil {
		if strings.Contains(err.Error(), "-L parameter") && strings.Contains(err.Error(), "no match") {
			return nil, fmt.Errorf("%w: %q in %s", ErrFunctionNotFound, symbol, path)
		}
		return nil, err
	}
	return parseFunctionHistory(out), nil
}

func parseFunctionHistory(out string) []FunctionHistoryEntry {
	var entries []FunctionHistoryEntry
	var cur *FunctionHistoryEntry
	var diff strings.Builder
	inDiff := false

	flush := func() {
		if cur == nil {
			return
		}
		cur.Diff = strings.TrimRight(diff.String(), "\n")
		entries = append(entries, *cur)
	}

	for line := range strings.SplitSeq(out, "\n") {
		if hash, ok := strings.CutPrefix(line, "commit "); ok {
			flush()
			diff.Reset()
			hash = strings.TrimSpace(hash)
			cur = &FunctionHistoryEntry{Hash: hash, Short: shortenHash(hash)}
			inDiff = false
			continue
		}
		if cur == nil {
			continue
		}

		switch {
		case strings.HasPrefix(line, "Author:"):
			cur.Author = strings.TrimSpace(strings.TrimPrefix(line, "Author:"))
		case strings.HasPrefix(line, "Date:"):
			cur.Date = strings.TrimSpace(strings.TrimPrefix(line, "Date:"))
		case strings.HasPrefix(line, "diff --git"), strings.HasPrefix(line, "@@"), inDiff:
			inDiff = true
			diff.WriteString(line)
			diff.WriteString("\n")
		case cur.Subject == "" && strings.TrimSpace(line) != "":
			cur.Subject = strings.TrimSpace(line)
		}
	}
	flush()
	return entries
}

func shortenHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}
