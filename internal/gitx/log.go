package gitx

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Commit is one entry from the history.
type Commit struct {
	Hash    string
	Short   string
	Author  string
	Email   string
	Date    time.Time
	Subject string
	Body    string
	// Parents holds parent hashes; more than one means a merge.
	Parents []string
	// Files lists paths touched, populated only when requested -- it
	// costs an extra diff per commit.
	Files []string
	// Insertions and Deletions are populated alongside Files.
	Insertions int
	Deletions  int
}

// Merge reports whether the commit has more than one parent.
func (c Commit) Merge() bool { return len(c.Parents) > 1 }

// LogOptions narrows a history query.
type LogOptions struct {
	// Limit caps how many commits are returned. Zero means 20.
	Limit int
	// Path restricts history to commits touching this path.
	Path string
	// Author filters by author name or email substring.
	Author string
	// Grep filters by commit message substring.
	Grep string
	// Since and Until accept anything git's --since/--until accept,
	// including "2 weeks ago".
	Since string
	Until string
	// Ref names a branch, tag or revision range. Empty means HEAD.
	Ref string
	// WithStats populates Files, Insertions and Deletions.
	WithStats bool
	// NoMerges drops merge commits, which usually carry no content of
	// their own and dominate a busy history.
	NoMerges bool
}

// logFormat uses record and field separators that cannot occur in commit
// metadata. Splitting on newlines would break on any multi-line commit
// body, which is most of them in a repository with real messages.
const (
	fieldSep  = "\x1f" // ASCII unit separator
	recordSep = "\x1e" // ASCII record separator
)

var logFormat = strings.Join([]string{
	"%H", "%h", "%an", "%ae", "%aI", "%P", "%s", "%b",
}, fieldSep) + recordSep

// Log reads commit history.
func Log(ctx context.Context, dir string, opts LogOptions) ([]Commit, error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}

	args := []string{"log", "--format=" + logFormat, "-n", strconv.Itoa(opts.Limit)}
	if opts.NoMerges {
		args = append(args, "--no-merges")
	}
	if opts.Author != "" {
		args = append(args, "--author="+opts.Author)
	}
	if opts.Grep != "" {
		// Fixed-string matching: a user asking for "fix(auth)" means that
		// literal text, not a regexp whose parentheses are groups.
		args = append(args, "--fixed-strings", "--grep="+opts.Grep)
	}
	if opts.Since != "" {
		args = append(args, "--since="+opts.Since)
	}
	if opts.Until != "" {
		args = append(args, "--until="+opts.Until)
	}
	if opts.Ref != "" {
		args = append(args, opts.Ref)
	}
	if opts.Path != "" {
		// The -- separator is what stops git reading a path that happens
		// to match a branch name as a revision.
		args = append(args, "--", opts.Path)
	}

	out, err := Run(ctx, dir, args...)
	if err != nil {
		return nil, err
	}

	commits := parseLog(out)

	if opts.WithStats {
		for i := range commits {
			files, ins, del, err := commitStats(ctx, dir, commits[i].Hash)
			if err != nil {
				continue // A stat failure must not lose the commit.
			}
			commits[i].Files = files
			commits[i].Insertions = ins
			commits[i].Deletions = del
		}
	}
	return commits, nil
}

func parseLog(out string) []Commit {
	var commits []Commit
	for _, record := range strings.Split(out, recordSep) {
		record = strings.TrimLeft(record, "\n")
		if strings.TrimSpace(record) == "" {
			continue
		}
		fields := strings.Split(record, fieldSep)
		if len(fields) < 8 {
			continue
		}
		c := Commit{
			Hash:    fields[0],
			Short:   fields[1],
			Author:  fields[2],
			Email:   fields[3],
			Subject: fields[6],
			Body:    strings.TrimSpace(fields[7]),
		}
		if t, err := time.Parse(time.RFC3339, fields[4]); err == nil {
			c.Date = t
		}
		if p := strings.TrimSpace(fields[5]); p != "" {
			c.Parents = strings.Fields(p)
		}
		commits = append(commits, c)
	}
	return commits
}

// commitStats reads the per-commit file list and line counts.
//
// A merge commit has no meaningful single diff, so it reports nothing
// rather than the arbitrary first-parent diff git would otherwise give.
func commitStats(ctx context.Context, dir, hash string) ([]string, int, int, error) {
	out, err := Run(ctx, dir, "show", "--numstat", "--format=", "--no-renames", hash)
	if err != nil {
		return nil, 0, 0, err
	}
	var (
		files      []string
		insertions int
		deletions  int
	)
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		// A binary file reports "-" instead of a count.
		if n, err := strconv.Atoi(parts[0]); err == nil {
			insertions += n
		}
		if n, err := strconv.Atoi(parts[1]); err == nil {
			deletions += n
		}
		files = append(files, parts[2])
	}
	return files, insertions, deletions, nil
}

// Show returns one commit with its file statistics.
func Show(ctx context.Context, dir, ref string) (Commit, error) {
	if ref == "" {
		ref = "HEAD"
	}
	commits, err := Log(ctx, dir, LogOptions{Limit: 1, Ref: ref, WithStats: true})
	if err != nil {
		return Commit{}, err
	}
	if len(commits) == 0 {
		return Commit{}, fmt.Errorf("no commit found at %q", ref)
	}
	return commits[0], nil
}
