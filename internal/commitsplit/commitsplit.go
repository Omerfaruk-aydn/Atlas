// Package commitsplit proposes how to split a pile of uncommitted
// changes into several smaller commits, ordered so that a package a
// change depends on is committed before the package that depends on it.
//
// It never runs git add or git commit itself -- see Result. Turning the
// proposal into real commits, and writing each one's message, is left to
// the caller (pair this with git_conventional_commit, run once per
// group after staging just that group's files).
package commitsplit

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/codeintel"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/gitx"
)

// CommitGroup is one proposed commit's worth of files.
type CommitGroup struct {
	// Label identifies the group: a Go import path for a package group,
	// or a directory path for a non-Go group.
	Label      string
	Files      []string
	Insertions int
	Deletions  int
}

// Result is a proposed split, in commit order.
type Result struct {
	Groups []CommitGroup
	// Cycles lists import cycles found among the *changed* packages --
	// packages inside a cycle cannot be strictly ordered relative to
	// each other (each depends on the other, directly or transitively),
	// so they are placed together as one group instead of two that
	// would each be individually unbuildable in isolation.
	Cycles [][]string
}

// Split reads every file changed in dir's working tree -- staged,
// unstaged, and untracked -- and groups them into a dependency-ordered
// commit plan. A conflicted (unmerged) path is left out entirely: it
// cannot be committed until it is resolved, so including it in a plan
// meant to guide commits would be misleading.
func Split(ctx context.Context, dir string) (Result, error) {
	status, err := gitx.GetStatus(ctx, dir)
	if err != nil {
		return Result{}, err
	}
	if len(status.Files) == 0 {
		return Result{}, nil
	}

	conflicted := map[string]bool{}
	for _, p := range status.Conflicts {
		conflicted[p] = true
	}

	// git diff HEAD never reports untracked files, so it only supplies
	// insertion/deletion counts for paths that already existed at HEAD;
	// a brand new file's line count is read directly, below.
	diff, err := gitx.GetDiff(ctx, dir, gitx.DiffOptions{Ref: "HEAD"})
	if err != nil {
		return Result{}, err
	}
	statsByPath := map[string]gitx.FileDiff{}
	for _, f := range diff.Files {
		statsByPath[f.Path] = f
	}

	graph, err := codeintel.ImportGraph(dir, false)
	if err != nil {
		return Result{}, err
	}

	dirByImportPath := map[string]string{}
	pkgByDir := map[string]codeintel.PackageNode{}
	for _, pkg := range graph.Packages {
		dirByImportPath[pkg.ImportPath] = pkg.Dir
		pkgByDir[pkg.Dir] = pkg
	}

	statByDir := map[string]*CommitGroup{}
	var order []string
	for _, f := range status.Files {
		if conflicted[f.Path] {
			continue
		}
		for _, path := range expandPath(dir, f.Path) {
			addChangedFile(statByDir, &order, dir, path, statsByPath, pkgByDir)
		}
	}

	goDirs, otherDirs := splitGoAndOther(order, pkgByDir)
	sorted, cycles := topoSortPackages(goDirs, pkgByDir, dirByImportPath)
	sort.Strings(otherDirs)

	result := Result{Cycles: cycles}
	for _, d := range sorted {
		result.Groups = append(result.Groups, finalizeGroup(*statByDir[d]))
	}
	for _, d := range otherDirs {
		result.Groups = append(result.Groups, finalizeGroup(*statByDir[d]))
	}
	return result, nil
}

// expandPath handles the one shape git status's default mode reports
// that isn't a plain file: a wholly untracked directory is reported as
// a single entry ending in "/", not as each file inside it. Splitting
// by directory needs the individual files, so such an entry is walked
// and expanded here; every other path is returned as-is.
func expandPath(repoDir, path string) []string {
	if !strings.HasSuffix(path, "/") {
		return []string{path}
	}

	var out []string
	_ = filepath.WalkDir(filepath.Join(repoDir, path), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(repoDir, p)
		if err != nil {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	return out
}

func addChangedFile(statByDir map[string]*CommitGroup, order *[]string, repoDir, path string, statsByPath map[string]gitx.FileDiff, pkgByDir map[string]codeintel.PackageNode) {
	absDir := filepath.Dir(filepath.Join(repoDir, path))
	group, ok := statByDir[absDir]
	if !ok {
		label := absDir
		if pkg, isPkg := pkgByDir[absDir]; isPkg {
			label = pkg.ImportPath
		}
		group = &CommitGroup{Label: label}
		statByDir[absDir] = group
		*order = append(*order, absDir)
	}
	group.Files = append(group.Files, path)
	if fd, ok := statsByPath[path]; ok {
		group.Insertions += fd.Insertions
		group.Deletions += fd.Deletions
	} else {
		group.Insertions += countLines(filepath.Join(repoDir, path))
	}
}

// countLines gives an untracked file's insertion count, since git diff
// never reports one. A file that can't be read (permission, a broken
// symlink) contributes zero rather than failing the whole plan over one
// path's stats.
func countLines(path string) int {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return 0
	}
	return bytes.Count(data, []byte("\n")) + 1
}

func finalizeGroup(g CommitGroup) CommitGroup {
	sort.Strings(g.Files)
	return g
}

func splitGoAndOther(dirs []string, pkgByDir map[string]codeintel.PackageNode) (goDirs, otherDirs []string) {
	for _, d := range dirs {
		if _, ok := pkgByDir[d]; ok {
			goDirs = append(goDirs, d)
		} else {
			otherDirs = append(otherDirs, d)
		}
	}
	return goDirs, otherDirs
}

// topoSortPackages orders changed Go package directories so that a
// package another changed package imports comes first, using Kahn's
// algorithm restricted to edges between two changed packages -- an
// import of a package that didn't change carries no ordering
// constraint, since that package isn't being committed here at all.
//
// A package left over once no more dependency-free nodes remain is part
// of an import cycle: it and everything else still unresolved are
// appended together, alphabetically, as one final unordered batch, and
// reported in Cycles so the caller knows why.
func topoSortPackages(dirs []string, pkgByDir map[string]codeintel.PackageNode, dirByImportPath map[string]string) ([]string, [][]string) {
	changed := map[string]bool{}
	for _, d := range dirs {
		changed[d] = true
	}

	// dependsOn[d] = other changed dirs that d imports.
	dependsOn := map[string][]string{}
	for _, d := range dirs {
		for _, imp := range pkgByDir[d].Internal {
			depDir, ok := dirByImportPath[imp]
			if !ok || !changed[depDir] || depDir == d {
				continue
			}
			dependsOn[d] = append(dependsOn[d], depDir)
		}
	}

	remaining := map[string]int{}
	for _, d := range dirs {
		remaining[d] = len(dependsOn[d])
	}

	var sorted []string
	ready := func() []string {
		var r []string
		for _, d := range dirs {
			if remaining[d] == 0 {
				r = append(r, d)
			}
		}
		sort.Strings(r)
		return r
	}
	done := map[string]bool{}

	for {
		batch := ready()
		var pending []string
		for _, d := range batch {
			if done[d] {
				continue
			}
			pending = append(pending, d)
		}
		if len(pending) == 0 {
			break
		}
		for _, d := range pending {
			done[d] = true
			sorted = append(sorted, d)
			remaining[d] = -1 // Removed from further consideration.
		}
		for _, d := range dirs {
			if done[d] {
				continue
			}
			n := 0
			for _, dep := range dependsOn[d] {
				if !done[dep] {
					n++
				}
			}
			remaining[d] = n
		}
	}

	var leftover []string
	for _, d := range dirs {
		if !done[d] {
			leftover = append(leftover, d)
		}
	}
	if len(leftover) == 0 {
		return sorted, nil
	}

	sort.Strings(leftover)
	cycle := make([]string, len(leftover))
	for i, d := range leftover {
		cycle[i] = pkgByDir[d].ImportPath
	}
	sorted = append(sorted, leftover...)
	return sorted, [][]string{cycle}
}
