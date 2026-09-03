package gitx

import (
	"context"
	"strconv"
	"strings"
)

// FileDiff summarises the change to one path.
type FileDiff struct {
	Path string
	// OrigPath is set on a rename.
	OrigPath   string
	Insertions int
	Deletions  int
	// Binary reports a file git could not diff as text.
	Binary bool
	// Patch holds the unified diff, populated only when requested.
	Patch string
}

// Diff is a set of file changes.
type Diff struct {
	Files      []FileDiff
	Insertions int
	Deletions  int
	// Truncated reports that patch text was dropped to stay within
	// budget. The summary lines are always complete.
	Truncated bool
}

// DiffOptions selects what to compare.
type DiffOptions struct {
	// Staged compares the index against HEAD instead of the working tree
	// against the index. These are different questions and conflating
	// them is how a commit ends up containing the wrong thing.
	Staged bool
	// Ref compares against a revision or range instead. "main" compares
	// the working tree to main; "main..HEAD" compares two points.
	Ref string
	// Path narrows to one file or directory.
	Path string
	// WithPatch includes the unified diff text, not just the counts.
	WithPatch bool
	// ContextLines sets how many unchanged lines surround each hunk.
	// Zero means git's default of three.
	ContextLines int
	// MaxPatchBytes bounds total patch text. Zero means unlimited.
	MaxPatchBytes int
}

// GetDiff compares two states of the tree.
//
// The summary always comes from --numstat, which is exact and cheap, and
// the patch text is fetched separately only when asked for. A tool that
// always returns full patches makes a one-line question about a large
// refactor unaffordable.
func GetDiff(ctx context.Context, dir string, opts DiffOptions) (Diff, error) {
	args := []string{"diff", "--numstat", "--no-color"}
	if opts.Staged {
		args = append(args, "--cached")
	}
	if opts.Ref != "" {
		args = append(args, opts.Ref)
	}
	if opts.Path != "" {
		args = append(args, "--", opts.Path)
	}

	out, err := Run(ctx, dir, args...)
	if err != nil {
		return Diff{}, err
	}

	diff := parseNumstat(out)
	if !opts.WithPatch || len(diff.Files) == 0 {
		return diff, nil
	}

	patchArgs := []string{"diff", "--no-color"}
	if opts.ContextLines > 0 {
		patchArgs = append(patchArgs, "-U"+strconv.Itoa(opts.ContextLines))
	}
	if opts.Staged {
		patchArgs = append(patchArgs, "--cached")
	}
	if opts.Ref != "" {
		patchArgs = append(patchArgs, opts.Ref)
	}
	if opts.Path != "" {
		patchArgs = append(patchArgs, "--", opts.Path)
	}

	patch, err := Run(ctx, dir, patchArgs...)
	if err != nil {
		// The summary is still a useful answer without the patch.
		return diff, nil
	}

	assignPatches(&diff, patch, opts.MaxPatchBytes)
	return diff, nil
}

// parseNumstat reads "insertions<TAB>deletions<TAB>path" records. A
// binary file reports "-" for both counts.
func parseNumstat(out string) Diff {
	var diff Diff
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}

		f := FileDiff{Path: parts[2]}
		ins, insErr := strconv.Atoi(parts[0])
		del, delErr := strconv.Atoi(parts[1])
		if insErr != nil || delErr != nil {
			f.Binary = true
		} else {
			f.Insertions, f.Deletions = ins, del
			diff.Insertions += ins
			diff.Deletions += del
		}

		// A rename is reported as "old => new", sometimes with a shared
		// prefix or suffix factored out as "dir/{old => new}/file".
		if orig, path, ok := splitRename(f.Path); ok {
			f.OrigPath, f.Path = orig, path
		}

		diff.Files = append(diff.Files, f)
	}
	return diff
}

// splitRename expands git's compact rename notation into the two full
// paths. Without this a rename shows up as a path that does not exist,
// which then fails every subsequent read.
func splitRename(spec string) (orig, path string, ok bool) {
	open := strings.Index(spec, "{")
	arrow := strings.Index(spec, " => ")
	if arrow < 0 {
		return "", "", false
	}

	if open < 0 {
		return spec[:arrow], spec[arrow+4:], true
	}
	close := strings.Index(spec[open:], "}")
	if close < 0 || arrow < open {
		return spec[:arrow], spec[arrow+4:], true
	}
	close += open

	prefix := spec[:open]
	suffix := spec[close+1:]
	inner := spec[open+1 : close]
	innerOld, innerNew, found := strings.Cut(inner, " => ")
	if !found {
		return "", "", false
	}
	return cleanPath(prefix + innerOld + suffix), cleanPath(prefix + innerNew + suffix), true
}

// cleanPath collapses the doubled slash left behind when a rename's
// braces sat around an empty segment ("a/{ => b}/c" -> "a//c").
func cleanPath(p string) string {
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	return p
}

// assignPatches splits unified diff output per file and attaches it,
// stopping once the byte budget is spent.
func assignPatches(diff *Diff, patch string, maxBytes int) {
	sections := splitPatchByFile(patch)
	used := 0
	for i := range diff.Files {
		text, ok := sections[diff.Files[i].Path]
		if !ok {
			continue
		}
		if maxBytes > 0 && used+len(text) > maxBytes {
			diff.Truncated = true
			continue
		}
		diff.Files[i].Patch = text
		used += len(text)
	}
}

// splitPatchByFile keys each file's patch by its post-image path, which
// is the path the summary carries.
func splitPatchByFile(patch string) map[string]string {
	out := map[string]string{}
	var (
		current string
		buf     strings.Builder
	)

	flush := func() {
		if current != "" && buf.Len() > 0 {
			out[current] = buf.String()
		}
		buf.Reset()
	}

	for line := range strings.SplitSeq(patch, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			flush()
			current = postImagePath(line)
			continue
		}
		if current == "" {
			continue
		}
		// The +++ line names the post-image path authoritatively, which
		// matters when a path contains a space and the "diff --git"
		// header is therefore ambiguous.
		if after, ok := strings.CutPrefix(line, "+++ b/"); ok {
			current = after
			continue
		}
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") ||
			strings.HasPrefix(line, "index ") || strings.HasPrefix(line, "new file mode") ||
			strings.HasPrefix(line, "deleted file mode") || strings.HasPrefix(line, "similarity index") ||
			strings.HasPrefix(line, "rename from") || strings.HasPrefix(line, "rename to") ||
			strings.HasPrefix(line, "old mode") || strings.HasPrefix(line, "new mode") {
			continue
		}
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	flush()
	return out
}

// postImagePath pulls the "b/..." path out of a "diff --git a/x b/x"
// header. It assumes the two halves are equal length, which holds for
// everything but a rename -- and renames also carry a +++ line, which
// overrides this.
func postImagePath(header string) string {
	rest := strings.TrimPrefix(header, "diff --git ")
	half := len(rest) / 2
	b := strings.TrimSpace(rest[half:])
	return strings.TrimPrefix(b, "b/")
}
