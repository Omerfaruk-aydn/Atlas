package codeintel

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"sort"
	"strings"
)

// Method is one method in an interface's method set or on a concrete
// type, reduced to the shape that decides assignability.
type Method struct {
	Name string
	// Sig is the rendered parameter and result list, e.g.
	// "(ctx context.Context) (int, error)". It is compared textually,
	// which is why the analysis is a heuristic -- see TypeHierarchy.
	Sig string
}

// Interface is one interface declaration found in the scanned tree.
type Interface struct {
	Name    string
	Package string
	File    string
	Line    int
	Methods []Method
	// Embeds names interfaces this one embeds. They are resolved when
	// the embedded interface is also in the scanned tree, and left here
	// for reporting either way.
	Embeds []string
}

// ConcreteType is a named type together with the methods declared on it.
type ConcreteType struct {
	Name    string
	Package string
	File    string
	Line    int
	Methods []Method
	// PointerOnly is true when at least one method has a pointer
	// receiver, meaning only *T satisfies an interface, not T.
	PointerOnly bool
}

// Implementation pairs a concrete type with an interface it satisfies.
type Implementation struct {
	Type      ConcreteType
	Interface Interface
	// ViaPointer records that the satisfying type is *T rather than T.
	ViaPointer bool
}

// HierarchyResult is the outcome of a type-hierarchy scan.
type HierarchyResult struct {
	Interfaces      []Interface
	Types           []ConcreteType
	Implementations []Implementation
	FilesScanned    int
}

// TypeHierarchy reports which named types in root satisfy which
// interfaces declared in the same tree.
//
// Like the rest of this package the analysis is syntactic: method sets
// are matched by name and by the printed text of the signature. That is
// exact for the common case and wrong in three specific ways, all of
// which callers must surface rather than hide:
//
//   - Two identical types spelled differently (an alias, a dot-import, a
//     package qualifier present in one file and absent in the other) will
//     not match, so a real implementation can be missed.
//   - Interfaces declared outside the scanned tree -- io.Reader, or
//     anything from a dependency -- are not known, so satisfying them is
//     invisible here.
//   - Generic type parameters are compared as written, so two
//     instantiations that are identical after substitution may not match.
//
// Embedded interfaces are flattened when the embedded interface is also
// in the tree; when it is not, its name is kept in Embeds and its methods
// are simply unknown, which can only cause a missed match, never a false
// one.
func TypeHierarchy(root string, includeTests bool) (HierarchyResult, error) {
	files, err := collectGoFiles(root, includeTests)
	if err != nil {
		return HierarchyResult{}, err
	}

	var (
		interfaces []Interface
		byRecv     = map[string]*ConcreteType{}
		scanned    int
	)

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
		scanned++
		pkg := file.Name.Name

		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					pos := fset.Position(ts.Name.Pos())
					if iface, ok := ts.Type.(*ast.InterfaceType); ok {
						methods, embeds := interfaceMembers(fset, iface)
						interfaces = append(interfaces, Interface{
							Name:    ts.Name.Name,
							Package: pkg,
							File:    path,
							Line:    pos.Line,
							Methods: methods,
							Embeds:  embeds,
						})
						continue
					}
					// A named non-interface type. Record it even with no
					// methods yet; methods are attached below.
					ct := ensureConcrete(byRecv, ts.Name.Name)
					ct.Package = pkg
					ct.File = path
					ct.Line = pos.Line
				}
			case *ast.FuncDecl:
				if d.Recv == nil || len(d.Recv.List) == 0 || d.Name == nil {
					continue
				}
				recv, isPtr := receiverName(d.Recv.List[0].Type)
				if recv == "" {
					continue
				}
				ct := ensureConcrete(byRecv, recv)
				if ct.File == "" {
					// Methods can be declared in a different file from
					// the type; keep the first position seen as a
					// fallback so the report always has a location.
					ct.Package = pkg
					ct.File = path
					ct.Line = fset.Position(d.Name.Pos()).Line
				}
				ct.Methods = append(ct.Methods, Method{
					Name: d.Name.Name,
					Sig:  renderFuncSig(fset, d.Type),
				})
				if isPtr {
					ct.PointerOnly = true
				}
			}
		}
	}

	flattenEmbeds(interfaces)

	types := make([]ConcreteType, 0, len(byRecv))
	for _, ct := range byRecv {
		sortMethods(ct.Methods)
		types = append(types, *ct)
	}
	sort.Slice(types, func(i, j int) bool { return types[i].Name < types[j].Name })
	sort.Slice(interfaces, func(i, j int) bool { return interfaces[i].Name < interfaces[j].Name })

	var impls []Implementation
	for _, iface := range interfaces {
		// An interface with no methods is satisfied by everything, which
		// is true and useless. Skip it rather than emit N results.
		if len(iface.Methods) == 0 {
			continue
		}
		for _, ct := range types {
			if ct.Name == iface.Name {
				continue
			}
			if satisfies(ct, iface) {
				impls = append(impls, Implementation{
					Type:       ct,
					Interface:  iface,
					ViaPointer: ct.PointerOnly,
				})
			}
		}
	}

	return HierarchyResult{
		Interfaces:      interfaces,
		Types:           types,
		Implementations: impls,
		FilesScanned:    scanned,
	}, nil
}

func ensureConcrete(m map[string]*ConcreteType, name string) *ConcreteType {
	if ct, ok := m[name]; ok {
		return ct
	}
	ct := &ConcreteType{Name: name}
	m[name] = ct
	return ct
}

// satisfies reports whether ct's method set covers every method the
// interface requires, matching on name and printed signature.
func satisfies(ct ConcreteType, iface Interface) bool {
	if len(ct.Methods) < len(iface.Methods) {
		return false
	}
	have := make(map[string]string, len(ct.Methods))
	for _, m := range ct.Methods {
		have[m.Name] = m.Sig
	}
	for _, want := range iface.Methods {
		got, ok := have[want.Name]
		if !ok || got != want.Sig {
			return false
		}
	}
	return true
}

// interfaceMembers splits an interface body into its own methods and the
// names of the interfaces it embeds.
func interfaceMembers(fset *token.FileSet, iface *ast.InterfaceType) ([]Method, []string) {
	var (
		methods []Method
		embeds  []string
	)
	if iface.Methods == nil {
		return nil, nil
	}
	for _, field := range iface.Methods.List {
		if len(field.Names) == 0 {
			// An embedded interface, or a type-set constraint element.
			// Constraints have no method set, so recording the rendered
			// name is harmless: nothing will resolve it.
			embeds = append(embeds, renderNode(fset, field.Type))
			continue
		}
		ft, ok := field.Type.(*ast.FuncType)
		if !ok {
			continue
		}
		sig := renderFuncSig(fset, ft)
		for _, name := range field.Names {
			methods = append(methods, Method{Name: name.Name, Sig: sig})
		}
	}
	sortMethods(methods)
	return methods, embeds
}

// flattenEmbeds folds the method set of each embedded interface into the
// embedding one, when the embedded interface is present in the tree.
// Unresolvable embeds are left alone, which can only under-report.
func flattenEmbeds(interfaces []Interface) {
	byName := make(map[string]int, len(interfaces))
	for i, iface := range interfaces {
		byName[iface.Name] = i
	}

	// Bounded passes rather than recursion: an embedding chain deeper
	// than this is vanishingly rare, and a cycle -- which does not
	// compile anyway -- must not hang the scan.
	const maxDepth = 8
	for range maxDepth {
		changed := false
		for i := range interfaces {
			for _, embed := range interfaces[i].Embeds {
				// Strip any package qualifier: only same-tree names can
				// resolve, and those are usually written unqualified.
				name := embed
				if idx := strings.LastIndex(name, "."); idx >= 0 {
					name = name[idx+1:]
				}
				j, ok := byName[name]
				if !ok || j == i {
					continue
				}
				for _, m := range interfaces[j].Methods {
					if !hasMethod(interfaces[i].Methods, m) {
						interfaces[i].Methods = append(interfaces[i].Methods, m)
						changed = true
					}
				}
			}
		}
		if !changed {
			break
		}
	}
	for i := range interfaces {
		sortMethods(interfaces[i].Methods)
	}
}

func hasMethod(set []Method, m Method) bool {
	for _, have := range set {
		if have.Name == m.Name {
			return true
		}
	}
	return false
}

func sortMethods(ms []Method) {
	sort.Slice(ms, func(i, j int) bool { return ms[i].Name < ms[j].Name })
}

// receiverName extracts the base type name from a receiver expression,
// reporting whether the receiver is a pointer. Generic receivers such as
// Foo[T] reduce to Foo.
func receiverName(expr ast.Expr) (string, bool) {
	switch t := expr.(type) {
	case *ast.StarExpr:
		name, _ := receiverName(t.X)
		return name, true
	case *ast.Ident:
		return t.Name, false
	case *ast.IndexExpr: // Foo[T]
		name, ptr := receiverName(t.X)
		return name, ptr
	case *ast.IndexListExpr: // Foo[T, U]
		name, ptr := receiverName(t.X)
		return name, ptr
	}
	return "", false
}

// renderFuncSig prints a function's parameters and results, without the
// name, so an interface method and a concrete method can be compared.
func renderFuncSig(fset *token.FileSet, ft *ast.FuncType) string {
	var b strings.Builder
	b.WriteString(renderFieldList(fset, ft.Params, true))
	if ft.Results != nil && len(ft.Results.List) > 0 {
		b.WriteString(" ")
		b.WriteString(renderFieldList(fset, ft.Results, len(ft.Results.List) > 1 || len(ft.Results.List[0].Names) > 0))
	}
	return b.String()
}

// renderFieldList prints a parameter or result list using only the types,
// so that a parameter's name -- which has no bearing on assignability --
// cannot make two identical signatures compare unequal.
func renderFieldList(fset *token.FileSet, fl *ast.FieldList, parens bool) string {
	var parts []string
	if fl != nil {
		for _, field := range fl.List {
			typ := renderNode(fset, field.Type)
			n := max(len(field.Names), 1)
			for range n {
				parts = append(parts, typ)
			}
		}
	}
	joined := strings.Join(parts, ", ")
	if parens {
		return "(" + joined + ")"
	}
	return joined
}

func renderNode(fset *token.FileSet, node ast.Node) string {
	var b strings.Builder
	if err := printer.Fprint(&b, fset, node); err != nil {
		return fmt.Sprintf("<unprintable:%T>", node)
	}
	return b.String()
}
