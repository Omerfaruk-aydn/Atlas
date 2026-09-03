package codeintel

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
)

// FuncMetrics is the measured shape of one function.
type FuncMetrics struct {
	Name string
	Recv string
	File string
	Line int
	// Complexity is cyclomatic complexity: the number of linearly
	// independent paths through the body. One plus a branch point count.
	Complexity int
	// Lines is the span of the declaration in source lines, braces
	// included.
	Lines int
	// Params and Results count the signature's arity. A long parameter
	// list is a design smell the complexity number does not capture.
	Params  int
	Results int
	// Nesting is the deepest block nesting inside the body. Two functions
	// can share a complexity score while one is flat and the other is a
	// staircase, and the staircase is the harder one to read.
	Nesting int
	// Returns counts return statements, which is what makes a function
	// hard to reason about when combined with deep nesting.
	Returns int
}

// FileMetrics aggregates one file.
type FileMetrics struct {
	Path      string
	Lines     int
	Functions int
}

// MetricsResult is the outcome of a metrics scan.
type MetricsResult struct {
	Functions    []FuncMetrics
	Files        []FileMetrics
	FilesScanned int
	TotalLines   int
}

// Metrics measures every function under root.
//
// Cyclomatic complexity is counted the standard way: one, plus one for
// each if, for, range, case, branching comm clause, and each && or ||.
// That matches gocyclo and golangci-lint's cyclop, so a number here is
// comparable to a number from those.
//
// The measurements are exact -- unlike the rest of this package there is
// no approximation, because counting syntax needs no type information.
// What is a judgement call is what the numbers mean, and that belongs to
// the caller: a 30-branch switch over an enum is fine, a 30-branch nest
// of conditionals is not, and no threshold tells them apart.
func Metrics(root string, includeTests bool) (MetricsResult, error) {
	files, err := collectGoFiles(root, includeTests)
	if err != nil {
		return MetricsResult{}, err
	}

	var result MetricsResult
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
		if err != nil {
			continue
		}
		result.FilesScanned++

		fileLines := fset.File(file.Pos()).LineCount()
		result.TotalLines += fileLines
		fm := FileMetrics{Path: path, Lines: fileLines}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name == nil {
				continue
			}
			fm.Functions++

			m := FuncMetrics{
				Name:       fn.Name.Name,
				File:       path,
				Line:       fset.Position(fn.Name.Pos()).Line,
				Complexity: 1,
				Params:     countFields(fn.Type.Params),
				Results:    countFields(fn.Type.Results),
			}
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				m.Recv, _ = receiverName(fn.Recv.List[0].Type)
			}
			if fn.Body != nil {
				m.Lines = fset.Position(fn.Body.Rbrace).Line - fset.Position(fn.Pos()).Line + 1
				m.Complexity += branchPoints(fn.Body)
				m.Nesting = maxNesting(fn.Body)
				m.Returns = countReturns(fn.Body)
			}
			result.Functions = append(result.Functions, m)
		}
		result.Files = append(result.Files, fm)
	}

	sort.Slice(result.Functions, func(i, j int) bool {
		if result.Functions[i].Complexity != result.Functions[j].Complexity {
			return result.Functions[i].Complexity > result.Functions[j].Complexity
		}
		if result.Functions[i].Lines != result.Functions[j].Lines {
			return result.Functions[i].Lines > result.Functions[j].Lines
		}
		return result.Functions[i].Name < result.Functions[j].Name
	})
	sort.Slice(result.Files, func(i, j int) bool {
		return result.Files[i].Lines > result.Files[j].Lines
	})
	return result, nil
}

// branchPoints counts the decision points that add a path through a body.
func branchPoints(body ast.Node) int {
	n := 0
	ast.Inspect(body, func(node ast.Node) bool {
		switch stmt := node.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
			n++
		case *ast.CaseClause:
			// The default clause is not a decision: it is where control
			// lands when none of the others matched.
			if len(stmt.List) > 0 {
				n++
			}
		case *ast.CommClause:
			if stmt.Comm != nil {
				n++
			}
		case *ast.BinaryExpr:
			if stmt.Op == token.LAND || stmt.Op == token.LOR {
				n++
			}
		}
		return true
	})
	return n
}

// maxNesting reports the deepest block nesting in a body. Only constructs
// that actually indent count: a bare block statement is rare enough not
// to matter, and counting every BlockStmt would double-count each if.
func maxNesting(body *ast.BlockStmt) int {
	var walk func(n ast.Node, depth int) int
	walk = func(n ast.Node, depth int) int {
		deepest := depth
		ast.Inspect(n, func(child ast.Node) bool {
			if child == n {
				return true
			}
			switch child.(type) {
			case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt,
				*ast.TypeSwitchStmt, *ast.SelectStmt, *ast.FuncLit:
				if d := walk(child, depth+1); d > deepest {
					deepest = d
				}
				return false // Already descended.
			}
			return true
		})
		return deepest
	}
	return walk(body, 0)
}

func countReturns(body *ast.BlockStmt) int {
	n := 0
	ast.Inspect(body, func(node ast.Node) bool {
		if _, ok := node.(*ast.ReturnStmt); ok {
			n++
		}
		return true
	})
	return n
}

// countFields counts declared values, not field groups: "a, b int" is two
// parameters, not one.
func countFields(fl *ast.FieldList) int {
	if fl == nil {
		return 0
	}
	n := 0
	for _, field := range fl.List {
		n += max(len(field.Names), 1)
	}
	return n
}
