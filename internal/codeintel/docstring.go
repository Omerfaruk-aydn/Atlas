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

// DocstringSuggestion is a ready-to-paste doc comment for one exported
// declaration that currently has none.
type DocstringSuggestion struct {
	Name string
	// Kind is "func", "method", or "type".
	Kind string
	// Recv is the receiver type for a method.
	Recv      string
	Signature string
	// Stub is the suggested comment block, following Go's convention of
	// starting the comment with the declared name. The prose is a
	// placeholder -- describing what the name already says would add
	// nothing -- but the shape (summary line, parameter list, return
	// note) is filled in from the signature so only the wording is left
	// to do.
	Stub string
	File string
	Line int
}

// DocstringOptions narrows a scan.
type DocstringOptions struct {
	// IncludeTests scans _test.go files too. Off by default: exported
	// names in test files are rarely part of anything's public surface.
	IncludeTests bool
	// Symbol restricts suggestions to a single exported name. Empty
	// scans every undocumented exported declaration.
	Symbol string
}

// DocstringResult is the outcome of a scan.
type DocstringResult struct {
	Suggestions  []DocstringSuggestion
	FilesScanned int
}

// GenerateDocstrings finds exported declarations under root that have no
// doc comment and proposes a stub for each, shaped from the declaration's
// own signature: a parameter is listed by name and type, and a non-error
// return is called out, but the description text is left as a TODO for a
// human or a follow-up model call to fill in -- this tool reads syntax,
// it does not know what the code is for.
func GenerateDocstrings(root string, opts DocstringOptions) (DocstringResult, error) {
	files, err := collectGoFiles(root, opts.IncludeTests)
	if err != nil {
		return DocstringResult{}, err
	}

	result := DocstringResult{}
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, src, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			continue
		}
		result.FilesScanned++
		result.Suggestions = append(result.Suggestions, undocumentedIn(fset, file, path, opts.Symbol)...)
	}

	sort.SliceStable(result.Suggestions, func(i, j int) bool {
		if result.Suggestions[i].File != result.Suggestions[j].File {
			return result.Suggestions[i].File < result.Suggestions[j].File
		}
		return result.Suggestions[i].Line < result.Suggestions[j].Line
	})
	return result, nil
}

func undocumentedIn(fset *token.FileSet, file *ast.File, path, symbol string) []DocstringSuggestion {
	var out []DocstringSuggestion

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name == nil || !d.Name.IsExported() || d.Doc != nil {
				continue
			}
			if symbol != "" && d.Name.Name != symbol {
				continue
			}

			kind := "func"
			recv := ""
			if d.Recv != nil && len(d.Recv.List) > 0 {
				name, _ := receiverName(d.Recv.List[0].Type)
				if name == "" || !ast.IsExported(name) {
					continue // Unexported receiver: not part of the public surface.
				}
				kind, recv = "method", name
			}

			out = append(out, DocstringSuggestion{
				Name:      d.Name.Name,
				Kind:      kind,
				Recv:      recv,
				Signature: "func " + d.Name.Name + renderFuncSig(fset, d.Type),
				Stub:      funcDocStub(d.Name.Name, d.Type),
				File:      path,
				Line:      fset.Position(d.Name.Pos()).Line,
			})

		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || !ts.Name.IsExported() {
					continue
				}
				if ts.Doc != nil || d.Doc != nil {
					continue
				}
				if symbol != "" && ts.Name.Name != symbol {
					continue
				}
				out = append(out, DocstringSuggestion{
					Name:      ts.Name.Name,
					Kind:      "type",
					Signature: "type " + ts.Name.Name + " " + typeKeyword(ts.Type),
					Stub:      fmt.Sprintf("// %s TODO: describe what %s represents.\n", ts.Name.Name, ts.Name.Name),
					File:      path,
					Line:      fset.Position(ts.Name.Pos()).Line,
				})
			}
		}
	}
	return out
}

// funcDocStub builds a comment block shaped from a function's own
// parameter and result list, in the style Go convention expects: the
// comment starts with the declared name, followed by a one-line summary,
// then a parameter list when there is more than a context.Context to
// describe, then a note about what is returned.
func funcDocStub(name string, ft *ast.FuncType) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// %s TODO: describe what %s does.\n", name, name)

	params := describableParams(ft.Params)
	if len(params) > 0 {
		b.WriteString("//\n// Parameters:\n")
		for _, p := range params {
			fmt.Fprintf(&b, "//   - %s: TODO (%s).\n", p.name, p.typ)
		}
	}

	if ft.Results != nil && len(ft.Results.List) > 0 {
		b.WriteString("//\n")
		if returnsError(ft.Results) {
			b.WriteString("// Returns TODO, or an error if TODO.\n")
		} else {
			b.WriteString("// Returns TODO.\n")
		}
	}

	return b.String()
}

type namedParam struct {
	name string
	typ  string
}

// describableParams lists a function's parameters by name, skipping a
// lone context.Context -- its purpose is idiomatic enough across the
// codebase that spelling it out in every stub would be noise, not help.
func describableParams(fl *ast.FieldList) []namedParam {
	if fl == nil {
		return nil
	}
	var out []namedParam
	for _, field := range fl.List {
		typ := exprTypeName(field.Type)
		if len(field.Names) == 0 {
			if typ == "context.Context" {
				continue
			}
			out = append(out, namedParam{name: "_", typ: typ})
			continue
		}
		for _, n := range field.Names {
			if typ == "context.Context" && n.Name == "ctx" {
				continue
			}
			out = append(out, namedParam{name: n.Name, typ: typ})
		}
	}
	return out
}

func exprTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.SelectorExpr:
		if pkg, ok := t.X.(*ast.Ident); ok {
			return pkg.Name + "." + t.Sel.Name
		}
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprTypeName(t.X)
	case *ast.ArrayType:
		return "[]" + exprTypeName(t.Elt)
	case *ast.Ellipsis:
		return "..." + exprTypeName(t.Elt)
	}
	return "value"
}

func returnsError(fl *ast.FieldList) bool {
	for _, field := range fl.List {
		if id, ok := field.Type.(*ast.Ident); ok && id.Name == "error" {
			return true
		}
	}
	return false
}
