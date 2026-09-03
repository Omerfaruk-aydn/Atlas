package gitx

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// Branch is one local or remote branch.
type Branch struct {
	Name    string
	Remote  bool
	Current bool
	// Upstream is the tracked branch, empty when there is none.
	Upstream string
	Ahead    int
	Behind   int
	// LastCommit describes the tip.
	LastCommit  string
	LastAuthor  string
	LastDate    time.Time
	LastSubject string
	// MergedInto records the base this branch has already been merged
	// into, when it has been. A merged branch is safe to delete; an
	// unmerged one carries work that would be lost.
	MergedInto string
}

// Age returns how long since the branch's last commit.
func (b Branch) Age() time.Duration {
	if b.LastDate.IsZero() {
		return 0
	}
	return time.Since(b.LastDate)
}

// BranchOptions narrows a listing.
type BranchOptions struct {
	// IncludeRemote lists remote-tracking branches too.
	IncludeRemote bool
	// MergedBase, when set, marks branches already merged into it.
	// Usually "main" or "master".
	MergedBase string
}

// branchFormat uses a separator that cannot appear in a ref name or a
// commit subject.
const branchFieldSep = "\x1f"

var branchFormat = strings.Join([]string{
	"%(refname:short)",
	"%(HEAD)",
	"%(upstream:short)",
	"%(upstream:track)",
	"%(objectname:short)",
	"%(authorname)",
	"%(authordate:iso-strict)",
	"%(contents:subject)",
}, branchFieldSep)

// Branches lists branches with their divergence and tip information.
//
// It uses for-each-ref rather than `git branch -vv`, whose output is
// formatted for humans and changes shape with terminal width, branch name
// length, and whether a branch is checked out.
func Branches(ctx context.Context, dir string, opts BranchOptions) ([]Branch, error) {
	// Locals and remotes are read in separate passes so each ref's
	// namespace is known for certain. Reading both at once and guessing
	// from the name cannot work: a local branch called "feat/x" and a
	// remote-tracking "origin/x" are indistinguishable once refname:short
	// has stripped the namespace.
	out, err := Run(ctx, dir, "for-each-ref", "--format="+branchFormat, "refs/heads")
	if err != nil {
		return nil, err
	}
	branches := parseBranches(out, false)

	if opts.IncludeRemote {
		out, err := Run(ctx, dir, "for-each-ref", "--format="+branchFormat, "refs/remotes")
		if err != nil {
			return nil, err
		}
		branches = append(branches, parseBranches(out, true)...)
	}

	if opts.MergedBase != "" {
		merged, err := mergedBranches(ctx, dir, opts.MergedBase)
		if err == nil {
			for i := range branches {
				if merged[branches[i].Name] {
					branches[i].MergedInto = opts.MergedBase
				}
			}
		}
	}
	return branches, nil
}

func parseBranches(out string, remote bool) []Branch {
	var branches []Branch
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, branchFieldSep)
		if len(fields) < 8 {
			continue
		}

		b := Branch{
			Name:        fields[0],
			Current:     strings.TrimSpace(fields[1]) == "*",
			Upstream:    fields[2],
			LastCommit:  fields[4],
			LastAuthor:  fields[5],
			LastSubject: fields[7],
		}
		b.Remote = remote

		b.Ahead, b.Behind = parseTrack(fields[3])
		if t, err := time.Parse(time.RFC3339, fields[6]); err == nil {
			b.LastDate = t
		}
		branches = append(branches, b)
	}
	return branches
}

// parseTrack reads for-each-ref's "[ahead 3, behind 1]" track field.
func parseTrack(track string) (ahead, behind int) {
	track = strings.Trim(strings.TrimSpace(track), "[]")
	if track == "" {
		return 0, 0
	}
	for part := range strings.SplitSeq(track, ",") {
		part = strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(part, "ahead "):
			ahead, _ = strconv.Atoi(strings.TrimPrefix(part, "ahead "))
		case strings.HasPrefix(part, "behind "):
			behind, _ = strconv.Atoi(strings.TrimPrefix(part, "behind "))
		}
	}
	return ahead, behind
}

// mergedBranches returns the set of branches already merged into base.
func mergedBranches(ctx context.Context, dir, base string) (map[string]bool, error) {
	out, err := Run(ctx, dir, "branch", "--merged", base, "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}
	merged := map[string]bool{}
	for line := range strings.SplitSeq(out, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			merged[name] = true
		}
	}
	return merged, nil
}

// CurrentBranch returns the checked-out branch name, or the short commit
// when HEAD is detached.
func CurrentBranch(ctx context.Context, dir string) (string, error) {
	out, err := Run(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(out)
	if name != "HEAD" {
		return name, nil
	}
	out, err = Run(ctx, dir, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// DefaultBranch guesses the repository's base branch.
//
// It asks the remote's HEAD first, since that is the authoritative
// answer when a remote exists, then falls back to whichever conventional
// name is actually present. A wrong guess here would mark unmerged work
// as merged, so it never invents a name that does not exist.
func DefaultBranch(ctx context.Context, dir string) string {
	if out, err := Run(ctx, dir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		if name := strings.TrimSpace(out); name != "" {
			return strings.TrimPrefix(name, "origin/")
		}
	}
	for _, candidate := range []string{"main", "master", "trunk", "develop"} {
		if _, err := Run(ctx, dir, "rev-parse", "--verify", "--quiet", "refs/heads/"+candidate); err == nil {
			return candidate
		}
	}
	return ""
}
