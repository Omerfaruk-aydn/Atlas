package codeintel

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// APISymbol is one exported declaration.
type APISymbol struct {
	Name string
	// Kind is "func", "method", "type", "const", "var", or "field".
	Kind string
	// Recv is the receiver type for a method, or the owning struct for a
	// field.
	Recv string
	// Signature is the rendered declaration, e.g.
	// "func New(cfg Config) (*Client, error)".
	Signature string
	// Doc is the first line of the doc comment, which is where Go
	// convention puts the summary.
	Doc string
	// Deprecated reports a "Deprecated:" paragraph in the doc comment.
	Deprecated bool
	File       string
	Line       int
}

// APIPackage is one package's exported surface.
type APIPackage struct {
	Name    string
	Dir     string
	Symbols []APISymbol
	// Undocumented counts exported symbols with no doc comment.
	Undocumented int
}

// APIResult is one scan.
type APIResult struct {
	Packages     []APIPackage
	FilesScanned int
}

// APISurface reports the exported declarations of every package under
// root.
//
// This is what a consumer of the package can actually reach, which is a
// different and usually much smaller thing than what the package
// contains. It is also the part that cannot be changed without breaking
// somebody, so it is the right thing to look at before a rename, a
// signature change, or a release.
//
// Struct fields are included because an exported field on an exported
// struct is part of the contract just as much as a method -- changing its
// type breaks callers exactly the same way.
func APISurface(root string, includeTests bool) (APIResult, error) {
	files, err := collectGoFiles(root, includeTests)
	if err != nil {
		return APIResult{}, err
	}

	byDir := map[string]*APIPackage{}
	scanned := 0

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
		scanned++

		dir := filepath.Dir(path)
		pkg, ok := byDir[dir]
		if !ok {
			pkg = &APIPackage{Name: file.Name.Name, Dir: dir}
			byDir[dir] = pkg
		}

		pkg.Symbols = append(pkg.Symbols, exportedIn(fset, file, path)...)
	}

	result := APIResult{FilesScanned: scanned}
	for _, pkg := range byDir {
		sort.Slice(pkg.Symbols, func(i, j int) bool {
			// Types first, then their methods, then everything else --
			// so a type reads together with its API rather than being
			// scattered alphabetically.
			ai, aj := apiSortKey(pkg.Symbols[i]), apiSortKey(pkg.Symbols[j])
			if ai != aj {
				return ai < aj
			}
			return pkg.Symbols[i].Name < pkg.Symbols[j].Name
		})
		for _, s := range pkg.Symbols {
			if s.Doc == "" {
				pkg.Undocumented++
			}
		}
		result.Packages = append(result.Packages, *pkg)
	}
	sort.Slice(result.Packages, func(i, j int) bool {
		return result.Packages[i].Dir < result.Packages[j].Dir
	})
	return result, nil
}

// apiSortKey groups a type with the methods and fields that belong to it.
func apiSortKey(s APISymbol) string {
	switch s.Kind {
	case "type":
		return "1" + s.Name + "0"
	case "field":
		return "1" + s.Recv + "1" + s.Name
	case "method":
		return "1" + s.Recv + "2" + s.Name
	case "const":
		return "0" + s.Name
	default:
		return "2" + s.Name
	}
}

func exportedIn(fset *token.FileSet, file *ast.File, path string) []APISymbol {
	var out []APISymbol

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name == nil || !d.Name.IsExported() {
				continue
			}
			sym := APISymbol{
				Name:      d.Name.Name,
				Kind:      "func",
				Signature: "func " + d.Name.Name + renderFuncSig(fset, d.Type),
				File:      path,
				Line:      fset.Position(d.Name.Pos()).Line,
			}
			if d.Recv != nil && len(d.Recv.List) > 0 {
				recv, ptr := receiverName(d.Recv.List[0].Type)
				// An unexported receiver means the method is not
				// reachable from outside even though its own name is
				// capitalised.
				if recv == "" || !ast.IsExported(recv) {
					continue
				}
				sym.Kind = "method"
				sym.Recv = recv
				star := ""
				if ptr {
					star = "*"
				}
				sym.Signature = "func (" + star + recv + ") " + d.Name.Name + renderFuncSig(fset, d.Type)
			}
			sym.Doc, sym.Deprecated = summariseDoc(d.Doc)
			out = append(out, sym)

		case *ast.GenDecl:
			out = append(out, exportedFromGenDecl(fset, d, path)...)
		}
	}
	return out
}

func exportedFromGenDecl(fset *token.FileSet, d *ast.GenDecl, path string) []APISymbol {
	var out []APISymbol

	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			if !s.Name.IsExported() {
				continue
			}
			doc, deprecated := summariseDoc(s.Doc)
			if doc == "" {
				// A grouped declaration puts the comment on the group,
				// not the spec.
				doc, deprecated = summariseDoc(d.Doc)
			}
			out = append(out, APISymbol{
				Name:       s.Name.Name,
				Kind:       "type",
				Signature:  "type " + s.Name.Name + " " + typeKeyword(s.Type),
				Doc:        doc,
				Deprecated: deprecated,
				File:       path,
				Line:       fset.Position(s.Name.Pos()).Line,
			})
			out = append(out, exportedFields(fset, s, path)...)

		case *ast.ValueSpec:
			kind := "var"
			if d.Tok == token.CONST {
				kind = "const"
			}
			for _, name := range s.Names {
				if !name.IsExported() {
					continue
				}
				doc, deprecated := summariseDoc(s.Doc)
				if doc == "" {
					doc, deprecated = summariseDoc(d.Doc)
				}
				sig := kind + " " + name.Name
				if s.Type != nil {
					sig += " " + renderNode(fset, s.Type)
				}
				out = append(out, APISymbol{
					Name:       name.Name,
					Kind:       kind,
					Signature:  sig,
					Doc:        doc,
					Deprecated: deprecated,
					File:       path,
					Line:       fset.Position(name.Pos()).Line,
				})
			}
		}
	}
	return out
}

// exportedFields lists the exported fields of an exported struct. They
// are part of the contract: changing a field's type breaks a caller as
// surely as changing a method's signature does.
func exportedFields(fset *token.FileSet, ts *ast.TypeSpec, path string) []APISymbol {
	st, ok := ts.Type.(*ast.StructType)
	if !ok || st.Fields == nil {
		return nil
	}
	var out []APISymbol
	for _, field := range st.Fields.List {
		typ := renderNode(fset, field.Type)
		if len(field.Names) == 0 {
			// An embedded field promotes its whole method set into this
			// type's API, so it belongs in the surface.
			name := typ
			if i := strings.LastIndexAny(name, ".*"); i >= 0 {
				name = name[i+1:]
			}
			if !ast.IsExported(name) {
				continue
			}
			doc, deprecated := summariseDoc(field.Doc)
			out = append(out, APISymbol{
				Name: name, Kind: "field", Recv: ts.Name.Name,
				Signature: typ + " (embedded)", Doc: doc, Deprecated: deprecated,
				File: path, Line: fset.Position(field.Pos()).Line,
			})
			continue
		}
		for _, name := range field.Names {
			if !name.IsExported() {
				continue
			}
			doc, deprecated := summariseDoc(field.Doc)
			out = append(out, APISymbol{
				Name: name.Name, Kind: "field", Recv: ts.Name.Name,
				Signature: name.Name + " " + typ, Doc: doc, Deprecated: deprecated,
				File: path, Line: fset.Position(name.Pos()).Line,
			})
		}
	}
	return out
}

// typeKeyword names a type's underlying shape without printing the whole
// body, which for a large struct would be the entire file.
func typeKeyword(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StructType:
		return "struct"
	case *ast.InterfaceType:
		return "interface"
	case *ast.FuncType:
		return "func"
	case *ast.MapType:
		return "map"
	case *ast.ArrayType:
		if t.Len == nil {
			return "slice"
		}
		return "array"
	case *ast.ChanType:
		return "chan"
	case *ast.StarExpr:
		return "pointer"
	case *ast.Ident:
		return t.Name
	}
	return "type"
}

// summariseDoc returns the first sentence of a doc comment and whether it
// carries a Deprecated marker.
//
// Go convention puts the summary in the first line, so that is what a
// surface listing needs -- reprinting whole doc comments would make the
// output longer than reading the source.
func summariseDoc(doc *ast.CommentGroup) (string, bool) {
	if doc == nil {
		return "", false
	}
	text := doc.Text()
	deprecated := strings.Contains(text, "Deprecated:")

	first := text
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	first = strings.TrimSpace(first)
	if first == "" {
		// A doc comment whose first line is blank still has content
		// worth summarising.
		for line := range strings.SplitSeq(text, "\n") {
			if s := strings.TrimSpace(line); s != "" {
				first = s
				break
			}
		}
	}
	return first, deprecated
}
