package gitx

import (
	"context"
	"strconv"
	"strings"
)

// FileStatus is one path's state in the working tree.
type FileStatus struct {
	Path string
	// OrigPath is set for a rename or copy: where the content came from.
	OrigPath string
	// Staged and Unstaged are git's two-letter status codes expanded into
	// words: "modified", "added", "deleted", "renamed", "copied",
	// "untracked", "unmerged", or "" when that side is clean.
	Staged   string
	Unstaged string
}

// Status is the parsed state of a working tree.
type Status struct {
	Branch string
	// Upstream is the tracked remote branch, empty when there is none.
	Upstream string
	Ahead    int
	Behind   int
	// Detached reports a detached HEAD, where Branch holds the commit.
	Detached bool
	Files    []FileStatus
	// Conflicts holds paths in an unmerged state. They are also in Files;
	// they are pulled out because nothing else can proceed until they are
	// resolved.
	Conflicts []string
}

// Clean reports whether the working tree has no changes at all.
func (s Status) Clean() bool { return len(s.Files) == 0 }

// StagedCount and UnstagedCount count files with changes on each side. A
// file modified both ways counts on both.
func (s Status) StagedCount() int   { return countSide(s.Files, true) }
func (s Status) UnstagedCount() int { return countSide(s.Files, false) }

func countSide(files []FileStatus, staged bool) int {
	n := 0
	for _, f := range files {
		if staged && f.Staged != "" {
			n++
		}
		if !staged && f.Unstaged != "" {
			n++
		}
	}
	return n
}

// GetStatus reads the working tree state.
//
// It uses `status --porcelain=v2 -z`, which is the only status format git
// documents as stable for machine consumption. The v1 format loses
// information on renames and paths with unusual characters, and the
// human-readable output is explicitly not a contract.
func GetStatus(ctx context.Context, dir string) (Status, error) {
	out, err := Run(ctx, dir, "status", "--porcelain=v2", "--branch",
		"--untracked-files=normal", "-z")
	if err != nil {
		return Status{}, err
	}
	return parseStatusV2(out), nil
}

// parseStatusV2 reads porcelain v2 records.
//
// Records are NUL-terminated, but a rename record ("2 ") carries two
// paths separated by a NUL inside a single record -- so the parser has to
// consume an extra field for those rather than treating every NUL as a
// record boundary.
func parseStatusV2(out string) Status {
	var status Status
	fields := splitNul(out)

	// git emits branch.oid BEFORE branch.head, so the commit has to be
	// held until the whole header is read: at the point oid arrives,
	// whether HEAD is detached is not yet known.
	var headOID string

	for i := 0; i < len(fields); i++ {
		line := fields[i]
		if line == "" {
			continue
		}

		switch {
		case strings.HasPrefix(line, "# branch.head "):
			head := strings.TrimPrefix(line, "# branch.head ")
			if head == "(detached)" {
				status.Detached = true
			}
			status.Branch = head

		case strings.HasPrefix(line, "# branch.oid "):
			headOID = strings.TrimPrefix(line, "# branch.oid ")

		case strings.HasPrefix(line, "# branch.upstream "):
			status.Upstream = strings.TrimPrefix(line, "# branch.upstream ")

		case strings.HasPrefix(line, "# branch.ab "):
			ahead, behind := parseAheadBehind(strings.TrimPrefix(line, "# branch.ab "))
			status.Ahead, status.Behind = ahead, behind

		case strings.HasPrefix(line, "1 "):
			// Ordinary change: "1 XY sub mH mI mW hH hI path"
			if f, ok := parseOrdinary(line); ok {
				status.Files = append(status.Files, f)
			}

		case strings.HasPrefix(line, "2 "):
			// Rename or copy. The origin path is the NEXT field, inside
			// this record rather than after it.
			f, ok := parseRename(line)
			if ok && i+1 < len(fields) {
				f.OrigPath = fields[i+1]
				i++
				status.Files = append(status.Files, f)
			}

		case strings.HasPrefix(line, "u "):
			// Unmerged: "u XY sub m1 m2 m3 mW h1 h2 h3 path"
			if path, ok := lastField(line, 10); ok {
				status.Files = append(status.Files, FileStatus{
					Path:     path,
					Staged:   "unmerged",
					Unstaged: "unmerged",
				})
				status.Conflicts = append(status.Conflicts, path)
			}

		case strings.HasPrefix(line, "? "):
			status.Files = append(status.Files, FileStatus{
				Path:     strings.TrimPrefix(line, "? "),
				Unstaged: "untracked",
			})
		}
	}

	// A detached HEAD has no branch name, so report the short commit
	// instead. Leaving the literal "(detached)" there tells the reader
	// nothing about where they are.
	if status.Detached && headOID != "" {
		status.Branch = headOID
		if len(status.Branch) > 8 {
			status.Branch = status.Branch[:8]
		}
	}
	return status
}

func parseOrdinary(line string) (FileStatus, bool) {
	parts := strings.SplitN(line, " ", 9)
	if len(parts) < 9 {
		return FileStatus{}, false
	}
	staged, unstaged := expandXY(parts[1])
	return FileStatus{Path: parts[8], Staged: staged, Unstaged: unstaged}, true
}

func parseRename(line string) (FileStatus, bool) {
	// "2 XY sub mH mI mW hH hI score path"
	parts := strings.SplitN(line, " ", 10)
	if len(parts) < 10 {
		return FileStatus{}, false
	}
	staged, unstaged := expandXY(parts[1])
	return FileStatus{Path: parts[9], Staged: staged, Unstaged: unstaged}, true
}

// lastField returns the tail of a line after n space-separated fields,
// which is how git formats a path that may itself contain spaces.
func lastField(line string, n int) (string, bool) {
	parts := strings.SplitN(line, " ", n+1)
	if len(parts) <= n {
		return "", false
	}
	return parts[n], true
}

// expandXY turns git's two-letter code into words. X is the index state,
// Y the working-tree state; a dot means clean on that side.
func expandXY(xy string) (staged, unstaged string) {
	if len(xy) != 2 {
		return "", ""
	}
	return expandCode(xy[0]), expandCode(xy[1])
}

func expandCode(c byte) string {
	switch c {
	case 'M':
		return "modified"
	case 'A':
		return "added"
	case 'D':
		return "deleted"
	case 'R':
		return "renamed"
	case 'C':
		return "copied"
	case 'T':
		return "typechange"
	case 'U':
		return "unmerged"
	case '?':
		return "untracked"
	case '!':
		return "ignored"
	default: // '.' and anything unrecognised mean clean on this side.
		return ""
	}
}

// parseAheadBehind reads git's "+N -M" divergence field.
func parseAheadBehind(s string) (int, int) {
	var ahead, behind int
	for _, part := range strings.Fields(s) {
		if len(part) < 2 {
			continue
		}
		n, err := strconv.Atoi(part[1:])
		if err != nil {
			continue
		}
		switch part[0] {
		case '+':
			ahead = n
		case '-':
			behind = n
		}
	}
	return ahead, behind
}
