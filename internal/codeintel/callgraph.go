package codeintel

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
)

// FuncRef locates one function or method declaration.
type FuncRef struct {
	Name string
	// Recv is the receiver type name for a method, empty for a plain
	// function.
	Recv    string
	Package string
	File    string
	Line    int
}

// Key is the name the call graph is indexed by. Methods are indexed by
// bare method name rather than receiver.method, because a call site
// written as x.Close() carries no type information without a type
// checker -- so any Close() is a possible target. Over-approximating here
// is the safe direction for an impact question: it can name callers that
// are not really affected, never miss one that is.
func (f FuncRef) Key() string { return f.Name }

// Caller is one function that calls the symbol under analysis, together
// with how many hops away it is.
type Caller struct {
	Func  FuncRef
	Depth int
	// Via names the function it calls that leads to the target, so a
	// multi-hop result reads as a chain instead of a flat list.
	Via string
}

// ImpactResult is the outcome of a reverse-call-graph walk.
type ImpactResult struct {
	Target FuncRef
	// Ambiguous holds the other declarations sharing the target's name.
	// Their presence means callers may belong to any of them.
	Ambiguous []FuncRef
	Callers   []Caller
	// Reached counts distinct functions in the transitive caller set.
	Reached      int
	MaxDepth     int
	FilesScanned int
	// Truncated reports that the walk stopped at the depth limit and the
	// real caller set is larger.
	Truncated bool
}

// callSite records that one function body mentions another name.
type callGraph struct {
	// callers maps a callee name to every function whose body calls it.
	callers map[string][]FuncRef
	// decls maps a function name to its declarations.
	decls map[string][]FuncRef
	files int
}

// ImpactAnalysis reports which functions transitively call symbol, out to
// maxDepth hops, so the blast radius of changing it is visible before the
// change is made.
//
// Call sites are matched on the identifier alone. Without a type checker
// there is no way to tell x.Close() on one type from x.Close() on
// another, so every declaration named Close is a candidate and every
// Close() call site is an edge. This over-approximates: the result may
// name a caller that is not truly affected. That is deliberate -- for
// "what breaks if I change this", a false alarm costs a moment's reading
// and a miss costs a broken build. Ambiguous names are reported alongside
// the result so the caller knows when to distrust the breadth.
func ImpactAnalysis(root, symbol string, maxDepth int, includeTests bool) (ImpactResult, error) {
	if symbol == "" {
		return ImpactResult{}, fmt.Errorf("symbol is required")
	}
	if maxDepth < 1 {
		maxDepth = 1
	}

	graph, err := buildCallGraph(root, includeTests)
	if err != nil {
		return ImpactResult{}, err
	}

	decls := graph.decls[symbol]
	if len(decls) == 0 {
		return ImpactResult{
			Target:       FuncRef{Name: symbol},
			FilesScanned: graph.files,
		}, nil
	}

	result := ImpactResult{
		Target:       decls[0],
		FilesScanned: graph.files,
		MaxDepth:     maxDepth,
	}
	if len(decls) > 1 {
		result.Ambiguous = decls[1:]
	}

	// Breadth-first over the reverse edges, so Depth is the shortest
	// distance from the target and the nearest callers come first.
	seen := map[string]bool{symbol: true}
	frontier := []string{symbol}

	for depth := 1; depth <= maxDepth && len(frontier) > 0; depth++ {
		var next []string
		for _, callee := range frontier {
			for _, caller := range graph.callers[callee] {
				if seen[caller.Key()] {
					continue
				}
				seen[caller.Key()] = true
				result.Callers = append(result.Callers, Caller{
					Func:  caller,
					Depth: depth,
					Via:   callee,
				})
				next = append(next, caller.Key())
			}
		}
		frontier = next
	}
	result.Truncated = len(frontier) > 0
	result.Reached = len(result.Callers)

	sort.SliceStable(result.Callers, func(i, j int) bool {
		if result.Callers[i].Depth != result.Callers[j].Depth {
			return result.Callers[i].Depth < result.Callers[j].Depth
		}
		if result.Callers[i].Func.File != result.Callers[j].Func.File {
			return result.Callers[i].Func.File < result.Callers[j].Func.File
		}
		return result.Callers[i].Func.Line < result.Callers[j].Func.Line
	})

	return result, nil
}

func buildCallGraph(root string, includeTests bool) (*callGraph, error) {
	files, err := collectGoFiles(root, includeTests)
	if err != nil {
		return nil, err
	}

	g := &callGraph{
		callers: map[string][]FuncRef{},
		decls:   map[string][]FuncRef{},
	}

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
		g.files++
		pkg := file.Name.Name

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name == nil {
				continue
			}
			ref := FuncRef{
				Name:    fn.Name.Name,
				Package: pkg,
				File:    path,
				Line:    fset.Position(fn.Name.Pos()).Line,
			}
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				ref.Recv, _ = receiverName(fn.Recv.List[0].Type)
			}
			g.decls[ref.Name] = append(g.decls[ref.Name], ref)

			if fn.Body == nil {
				continue
			}
			// One edge per distinct callee, not per call site: repeating
			// a caller five times because it calls the target in a loop
			// body adds nothing to an impact report.
			for callee := range calleesIn(fn.Body) {
				g.callers[callee] = append(g.callers[callee], ref)
			}
		}
	}

	for name := range g.decls {
		sort.Slice(g.decls[name], func(i, j int) bool {
			if g.decls[name][i].File != g.decls[name][j].File {
				return g.decls[name][i].File < g.decls[name][j].File
			}
			return g.decls[name][i].Line < g.decls[name][j].Line
		})
	}
	return g, nil
}

// calleesIn returns the set of function names a body calls. Both f() and
// x.f() reduce to "f": the receiver is unknowable without type checking,
// and dropping it is what makes the analysis over-approximate rather than
// silently miss method calls entirely.
func calleesIn(body *ast.BlockStmt) map[string]struct{} {
	out := map[string]struct{}{}
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			out[fn.Name] = struct{}{}
		case *ast.SelectorExpr:
			if fn.Sel != nil {
				out[fn.Sel.Name] = struct{}{}
			}
		case *ast.IndexExpr: // generic instantiation: f[T]()
			if id, ok := fn.X.(*ast.Ident); ok {
				out[id.Name] = struct{}{}
			} else if sel, ok := fn.X.(*ast.SelectorExpr); ok && sel.Sel != nil {
				out[sel.Sel.Name] = struct{}{}
			}
		}
		return true
	})
	return out
}
