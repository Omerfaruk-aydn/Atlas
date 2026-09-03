// Package codeintel holds the static-analysis engines behind Atlas's
// code-intelligence tools.
//
// Everything here parses Go source with go/ast and go/parser only -- no
// type checking, no build step, no module download. That makes the
// analyses fast and usable on a tree that does not currently compile,
// which is exactly the state an agent most often finds a repository in.
// The trade-off is that results are heuristics rather than proofs, and
// each analysis documents where it can be wrong.
package codeintel

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DeadSymbol is one declaration that no other file in the scanned tree
// appears to reference.
type DeadSymbol struct {
	Name     string
	Kind     string // "func", "method", "type", "const", "var"
	File     string
	Line     int
	Exported bool
}

// DeadCodeResult is the outcome of a dead-code scan.
type DeadCodeResult struct {
	Symbols      []DeadSymbol
	FilesScanned int
	// SkippedTests reports whether _test.go files were excluded. When they
	// are, a symbol used only by tests still counts as dead, which is
	// usually what a cleanup pass wants but is worth stating.
	SkippedTests bool
}

// deadCodeFile is one parsed file plus the identifiers it mentions.
type deadCodeFile struct {
	path  string
	fset  *token.FileSet
	decls []DeadSymbol
	// uses holds every identifier referenced anywhere in the file,
	// including in its own declarations. Self-reference inside the
	// declaring file is handled by the caller.
	uses map[string]int
}

// FindDeadCode reports declarations in root that nothing else references.
//
// The analysis is deliberately syntactic: it collects every top-level
// declaration, then counts identifier occurrences across the whole tree.
// A declaration whose name is never mentioned outside its own declaration
// site is reported.
//
// It is a heuristic, and it is conservative in one direction and not the
// other:
//
//   - It never misses a genuine use through a plain call or reference,
//     because any mention of the name counts.
//   - It CAN report a false positive for a symbol reached only through
//     reflection, a struct tag, code generation, a build-tagged file that
//     was skipped, or an interface satisfied implicitly (a method that
//     exists only to satisfy an interface is never named at the call
//     site).
//   - It CAN also report a false positive for an exported symbol consumed
//     by a downstream module, since only the given tree is scanned.
//
// Callers must present the output as candidates to review, not as a list
// safe to delete blindly.
func FindDeadCode(root string, includeTests bool, includeUnexported bool) (DeadCodeResult, error) {
	files, err := collectGoFiles(root, includeTests)
	if err != nil {
		return DeadCodeResult{}, err
	}

	parsed := make([]*deadCodeFile, 0, len(files))
	for _, path := range files {
		pf, err := parseForDeadCode(path)
		if err != nil {
			// A file that does not parse is skipped rather than failing
			// the whole scan: a half-edited tree is the normal case.
			continue
		}
		parsed = append(parsed, pf)
	}

	// Total mentions of every identifier across the tree.
	total := map[string]int{}
	for _, pf := range parsed {
		for name, n := range pf.uses {
			total[name] += n
		}
	}

	var dead []DeadSymbol
	for _, pf := range parsed {
		for _, d := range pf.decls {
			if !d.Exported && !includeUnexported {
				continue
			}
			// A declaration mentions its own name once, at the
			// declaration site. Anything above that is a real use.
			if total[d.Name] <= 1 {
				dead = append(dead, d)
			}
		}
	}

	sort.Slice(dead, func(i, j int) bool {
		if dead[i].File != dead[j].File {
			return dead[i].File < dead[j].File
		}
		return dead[i].Line < dead[j].Line
	})

	return DeadCodeResult{
		Symbols:      dead,
		FilesScanned: len(parsed),
		SkippedTests: !includeTests,
	}, nil
}

func parseForDeadCode(path string) (*deadCodeFile, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}

	pf := &deadCodeFile{path: path, fset: fset, uses: map[string]int{}}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name == nil {
				continue
			}
			// init and main are entry points; TestXxx/BenchmarkXxx are
			// called by the testing framework, never by name.
			if isEntryPoint(d.Name.Name) {
				continue
			}
			kind := "func"
			if d.Recv != nil {
				kind = "method"
			}
			pf.decls = append(pf.decls, DeadSymbol{
				Name:     d.Name.Name,
				Kind:     kind,
				File:     path,
				Line:     fset.Position(d.Name.Pos()).Line,
				Exported: d.Name.IsExported(),
			})
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					pf.decls = append(pf.decls, DeadSymbol{
						Name:     s.Name.Name,
						Kind:     "type",
						File:     path,
						Line:     fset.Position(s.Name.Pos()).Line,
						Exported: s.Name.IsExported(),
					})
				case *ast.ValueSpec:
					kind := "var"
					if d.Tok == token.CONST {
						kind = "const"
					}
					for _, name := range s.Names {
						if name.Name == "_" {
							continue
						}
						pf.decls = append(pf.decls, DeadSymbol{
							Name:     name.Name,
							Kind:     kind,
							File:     path,
							Line:     fset.Position(name.Pos()).Line,
							Exported: name.IsExported(),
						})
					}
				}
			}
		}
	}

	ast.Inspect(file, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			pf.uses[id.Name]++
		}
		return true
	})

	return pf, nil
}

// isEntryPoint reports whether a function is invoked by the toolchain
// rather than by name, so its absence of callers means nothing.
func isEntryPoint(name string) bool {
	switch name {
	case "init", "main":
		return true
	}
	for _, prefix := range []string{"Test", "Benchmark", "Fuzz", "Example"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// collectGoFiles walks root and returns every .go file, skipping vendor
// directories, hidden directories, and testdata -- none of which a
// cleanup pass should be told to edit.
func collectGoFiles(root string, includeTests bool) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("cannot scan %s: %w", root, err)
	}
	if !info.IsDir() {
		if strings.HasSuffix(root, ".go") {
			return []string{root}, nil
		}
		return nil, fmt.Errorf("%s is not a directory or a .go file", root)
	}

	var files []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Unreadable subtree: skip, do not abort the scan.
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == "testdata" || name == "node_modules" {
				return filepath.SkipDir
			}
			// Skip .git, .idea and friends, but never the root itself
			// even if it happens to start with a dot.
			if path != root && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if !includeTests && strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}
