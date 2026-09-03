package codeintel

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// PackageNode is one directory of Go source, with the imports its files
// declare.
type PackageNode struct {
	// ImportPath is the module-qualified path, e.g.
	// "example.com/mod/internal/agent". It falls back to the directory
	// path relative to the root when no module can be determined.
	ImportPath string
	Dir        string
	Name       string
	// Internal holds imports that resolve to another package in this
	// same module, already normalised to import paths.
	Internal []string
	// External holds third-party and standard-library imports.
	External []string
	Files    int
}

// ImportGraphResult is the outcome of an import-graph scan.
type ImportGraphResult struct {
	Module   string
	Packages []PackageNode
	// Cycles holds each import cycle found among internal packages, as an
	// ordered list of import paths where the last element imports the
	// first.
	Cycles [][]string
}

// ImportGraph builds the internal package dependency graph rooted at
// root and reports any import cycles in it.
//
// Package identity is the directory, which is how Go itself defines it.
// Import paths are resolved through the module path in go.mod when one is
// found at or above root; without a go.mod, packages are identified by
// their path relative to root and only same-tree edges are recognised.
//
// Only static import declarations are read. An import injected by build
// tags in a file excluded from the scan, or a dependency expressed
// through a plugin or reflection, is not visible -- so the graph can be
// missing an edge, and a cycle it does not report may still exist.
func ImportGraph(root string, includeTests bool) (ImportGraphResult, error) {
	files, err := collectGoFiles(root, includeTests)
	if err != nil {
		return ImportGraphResult{}, err
	}

	modulePath, moduleRoot := findModule(root)

	byDir := map[string]*PackageNode{}
	for _, path := range files {
		fset := token.NewFileSet()
		src, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		// ImportsOnly stops at the import block: the rest of the file is
		// irrelevant here, and skipping it makes a whole-repository scan
		// cheap.
		file, err := parser.ParseFile(fset, path, src, parser.ImportsOnly|parser.SkipObjectResolution)
		if err != nil {
			continue
		}

		dir := filepath.Dir(path)
		node, ok := byDir[dir]
		if !ok {
			node = &PackageNode{
				Dir:        dir,
				Name:       file.Name.Name,
				ImportPath: importPathFor(dir, modulePath, moduleRoot, root),
			}
			byDir[dir] = node
		}
		node.Files++

		for _, imp := range file.Imports {
			if imp.Path == nil {
				continue
			}
			target, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				continue
			}
			node.External = append(node.External, target)
		}
	}

	// Split each package's imports into internal and external now that
	// every package's import path is known.
	known := make(map[string]bool, len(byDir))
	for _, node := range byDir {
		known[node.ImportPath] = true
	}
	for _, node := range byDir {
		var internal, external []string
		for _, target := range node.External {
			if known[target] && target != node.ImportPath {
				internal = append(internal, target)
			} else if !known[target] {
				external = append(external, target)
			}
		}
		node.Internal = dedupeSorted(internal)
		node.External = dedupeSorted(external)
	}

	packages := make([]PackageNode, 0, len(byDir))
	for _, node := range byDir {
		packages = append(packages, *node)
	}
	sort.Slice(packages, func(i, j int) bool {
		return packages[i].ImportPath < packages[j].ImportPath
	})

	return ImportGraphResult{
		Module:   modulePath,
		Packages: packages,
		Cycles:   findCycles(packages),
	}, nil
}

// findCycles returns every distinct import cycle among the given
// packages, using an iterative depth-first search with an explicit stack
// so a pathological graph cannot blow the goroutine stack.
func findCycles(packages []PackageNode) [][]string {
	edges := make(map[string][]string, len(packages))
	for _, p := range packages {
		edges[p.ImportPath] = p.Internal
	}

	const (
		white = 0 // unvisited
		grey  = 1 // on the current path
		black = 2 // fully explored
	)
	color := make(map[string]int, len(edges))

	var (
		cycles []([]string)
		seen   = map[string]bool{}
		path   []string
		onPath = map[string]int{} // node -> index in path
	)

	// Explicit frame stack: node plus how many of its edges are done.
	type frame struct {
		node string
		next int
	}

	roots := make([]string, 0, len(edges))
	for node := range edges {
		roots = append(roots, node)
	}
	sort.Strings(roots)

	for _, root := range roots {
		if color[root] != white {
			continue
		}
		stack := []frame{{node: root}}
		color[root] = grey
		onPath[root] = len(path)
		path = append(path, root)

		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			if top.next >= len(edges[top.node]) {
				color[top.node] = black
				delete(onPath, top.node)
				path = path[:len(path)-1]
				stack = stack[:len(stack)-1]
				continue
			}
			target := edges[top.node][top.next]
			top.next++

			switch color[target] {
			case grey:
				// Back edge: the cycle is the path from target onward.
				start := onPath[target]
				cycle := append([]string(nil), path[start:]...)
				if key := cycleKey(cycle); !seen[key] {
					seen[key] = true
					cycles = append(cycles, cycle)
				}
			case white:
				color[target] = grey
				onPath[target] = len(path)
				path = append(path, target)
				stack = append(stack, frame{node: target})
			}
		}
	}

	sort.Slice(cycles, func(i, j int) bool {
		if len(cycles[i]) != len(cycles[j]) {
			return len(cycles[i]) < len(cycles[j])
		}
		return strings.Join(cycles[i], ",") < strings.Join(cycles[j], ",")
	})
	return cycles
}

// cycleKey normalises a cycle so the same loop discovered from different
// entry points is reported once. Rotating to the smallest element is
// enough: direction is fixed by the edges.
func cycleKey(cycle []string) string {
	if len(cycle) == 0 {
		return ""
	}
	minIdx := 0
	for i, node := range cycle {
		if node < cycle[minIdx] {
			minIdx = i
		}
	}
	rotated := make([]string, 0, len(cycle))
	rotated = append(rotated, cycle[minIdx:]...)
	rotated = append(rotated, cycle[:minIdx]...)
	return strings.Join(rotated, "\x00")
}

// findModule looks for a go.mod at or above dir and returns its module
// path together with the directory holding it.
func findModule(dir string) (string, string) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", ""
	}
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	for {
		data, err := os.ReadFile(filepath.Join(abs, "go.mod"))
		if err == nil {
			if path := parseModulePath(string(data)); path != "" {
				return path, abs
			}
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", ""
		}
		abs = parent
	}
}

func parseModulePath(goMod string) string {
	for line := range strings.SplitSeq(goMod, "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "module "); ok {
			return strings.Trim(strings.TrimSpace(after), `"`)
		}
	}
	return ""
}

// importPathFor turns a directory into the import path other packages
// would use to reach it.
func importPathFor(dir, modulePath, moduleRoot, root string) string {
	base, anchor := moduleRoot, modulePath
	if base == "" {
		// No module: fall back to root-relative paths. Edges still
		// resolve, they are just not real import paths.
		base, anchor = root, ""
	}
	rel, err := filepath.Rel(base, dir)
	if err != nil {
		return filepath.ToSlash(dir)
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		if anchor != "" {
			return anchor
		}
		return "."
	}
	if anchor == "" {
		return rel
	}
	return anchor + "/" + rel
}

func dedupeSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	sort.Strings(in)
	out := in[:1]
	for _, v := range in[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}
