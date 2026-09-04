package codeintel

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
)

// AntiPatternFinding is one syntactic smell found in a Go source file.
type AntiPatternFinding struct {
	// Kind is one of "swallowed-error", "context-not-first",
	// "panic-in-library".
	Kind    string
	File    string
	Line    int
	Func    string
	Message string
	Snippet string
}

// AntiPatternOptions narrows a scan.
type AntiPatternOptions struct {
	// IncludeTests scans _test.go files too. Off by default: a test
	// deliberately panicking on a bad fixture, or discarding an error
	// from a throwaway helper, is normal and not worth flagging.
	IncludeTests bool
}

// AntiPatternResult is the outcome of a scan.
type AntiPatternResult struct {
	Findings     []AntiPatternFinding
	FilesScanned int
	ByKind       map[string]int
}

// errLikeName reports whether an identifier looks like it holds an error,
// by name rather than by type -- this package never type-checks.
func errLikeName(name string) bool {
	return strings.Contains(strings.ToLower(name), "err")
}

// ScanAntiPatterns walks root's Go source for three syntactic smells:
//
//   - swallowed-error: an `if err != nil { }` whose body does nothing, or
//     an error-like value explicitly discarded with `_ = err`. Both
//     compile cleanly and both can hide a real failure.
//   - context-not-first: a function with a context.Context parameter that
//     isn't the first one, which breaks the convention every caller and
//     linter expects.
//   - panic-in-library: a panic outside package main, a test file, or a
//     function named MustXxx (where panicking is the documented contract).
//     A library that panics instead of returning an error takes the
//     decision to crash away from its caller.
//
// This is name- and shape-based, not type-checked: it can flag a `_ =`
// discard on a value that only looks like an error, and it cannot see
// through an error wrapped in a different variable name. Findings are
// candidates for review, not a list of confirmed bugs.
func ScanAntiPatterns(root string, opts AntiPatternOptions) (AntiPatternResult, error) {
	files, err := collectGoFiles(root, opts.IncludeTests)
	if err != nil {
		return AntiPatternResult{}, err
	}

	result := AntiPatternResult{ByKind: map[string]int{}}
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
		if err != nil {
			// A half-edited file is the normal case, not a scan failure.
			continue
		}
		result.FilesScanned++

		isMain := file.Name != nil && file.Name.Name == "main"
		isTest := strings.HasSuffix(path, "_test.go")

		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Name == nil {
				continue
			}
			if f := checkContextParamOrder(fd, path, fset); f != nil {
				result.Findings = append(result.Findings, *f)
			}
			if fd.Body != nil {
				inspectFuncBody(fd, path, fset, isMain, isTest, &result.Findings)
			}
		}
	}

	sort.SliceStable(result.Findings, func(i, j int) bool {
		if result.Findings[i].File != result.Findings[j].File {
			return result.Findings[i].File < result.Findings[j].File
		}
		return result.Findings[i].Line < result.Findings[j].Line
	})
	for _, f := range result.Findings {
		result.ByKind[f.Kind]++
	}
	return result, nil
}

// checkContextParamOrder flags a context.Context parameter that is not
// the function's first parameter.
func checkContextParamOrder(fd *ast.FuncDecl, path string, fset *token.FileSet) *AntiPatternFinding {
	if fd.Type.Params == nil {
		return nil
	}

	idx := 0
	for _, field := range fd.Type.Params.List {
		n := len(field.Names)
		if n == 0 {
			n = 1
		}
		isCtx := isContextType(field.Type)
		for range n {
			if isCtx && idx != 0 {
				return &AntiPatternFinding{
					Kind:    "context-not-first",
					File:    path,
					Line:    fset.Position(field.Pos()).Line,
					Func:    fd.Name.Name,
					Message: fmt.Sprintf("%s takes a context.Context that isn't its first parameter", fd.Name.Name),
					Snippet: "func " + fd.Name.Name + "(...)",
				}
			}
			idx++
		}
	}
	return nil
}

func isContextType(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "context" && sel.Sel.Name == "Context"
}

// inspectFuncBody walks one function body for swallowed errors and
// library panics.
func inspectFuncBody(fd *ast.FuncDecl, path string, fset *token.FileSet, isMain, isTest bool, findings *[]AntiPatternFinding) {
	mustFn := strings.HasPrefix(fd.Name.Name, "Must") || strings.HasPrefix(fd.Name.Name, "must")
	skipPanic := isMain || isTest || mustFn

	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.IfStmt:
			if name, ok := swallowedErrCheck(v); ok {
				*findings = append(*findings, AntiPatternFinding{
					Kind:    "swallowed-error",
					File:    path,
					Line:    fset.Position(v.Pos()).Line,
					Func:    fd.Name.Name,
					Message: fmt.Sprintf("%s is checked but the empty block does nothing about it", name),
					Snippet: fmt.Sprintf("if %s != nil {}", name),
				})
			}
		case *ast.AssignStmt:
			if name, ok := blankErrDiscard(v); ok {
				*findings = append(*findings, AntiPatternFinding{
					Kind:    "swallowed-error",
					File:    path,
					Line:    fset.Position(v.Pos()).Line,
					Func:    fd.Name.Name,
					Message: fmt.Sprintf("%s is explicitly discarded with `_ =`", name),
					Snippet: fmt.Sprintf("_ = %s", name),
				})
			}
		case *ast.CallExpr:
			if !skipPanic && isPanicCall(v) {
				*findings = append(*findings, AntiPatternFinding{
					Kind:    "panic-in-library",
					File:    path,
					Line:    fset.Position(v.Pos()).Line,
					Func:    fd.Name.Name,
					Message: fmt.Sprintf("%s panics instead of returning an error", fd.Name.Name),
					Snippet: "panic(...)",
				})
			}
		}
		return true
	})
}

// swallowedErrCheck reports an `if err != nil { }` (or `nil != err`) whose
// body has no statements at all.
func swallowedErrCheck(v *ast.IfStmt) (string, bool) {
	if len(v.Body.List) != 0 {
		return "", false
	}
	bin, ok := v.Cond.(*ast.BinaryExpr)
	if !ok || bin.Op != token.NEQ {
		return "", false
	}

	if id, ok := bin.X.(*ast.Ident); ok && errLikeName(id.Name) && isNilIdent(bin.Y) {
		return id.Name, true
	}
	if id, ok := bin.Y.(*ast.Ident); ok && errLikeName(id.Name) && isNilIdent(bin.X) {
		return id.Name, true
	}
	return "", false
}

func isNilIdent(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "nil"
}

// blankErrDiscard reports a `_ = x` assignment where x looks like an
// error by name.
func blankErrDiscard(v *ast.AssignStmt) (string, bool) {
	if v.Tok != token.ASSIGN || len(v.Lhs) != 1 || len(v.Rhs) != 1 {
		return "", false
	}
	lhs, ok := v.Lhs[0].(*ast.Ident)
	if !ok || lhs.Name != "_" {
		return "", false
	}
	rhs, ok := v.Rhs[0].(*ast.Ident)
	if !ok || !errLikeName(rhs.Name) {
		return "", false
	}
	return rhs.Name, true
}

// isPanicCall reports a call to the builtin panic.
func isPanicCall(v *ast.CallExpr) bool {
	id, ok := v.Fun.(*ast.Ident)
	return ok && id.Name == "panic"
}
