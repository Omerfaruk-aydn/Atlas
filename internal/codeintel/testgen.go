package codeintel

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
)

// TestSkeleton is a generated table-driven test for one function.
type TestSkeleton struct {
	FuncName string
	Skeleton string
	// Imports lists the packages the skeleton references, so the caller
	// knows what to add alongside it -- this is a snippet to paste into
	// an existing test file, not a standalone one, so it deliberately
	// does not print its own import block.
	Imports []string
	File    string
	Line    int
}

// GenerateTestSkeleton finds symbol -- a package-level function, not a
// method -- under root and generates a table-driven test skeleton shaped
// from its parameters and return values.
//
// Methods are not supported: constructing a receiver generically isn't
// possible from syntax alone, and a wrong guess (a zero value, say) would
// produce a skeleton that compiles but tests nothing meaningful. A plain
// function's parameters can all become table fields instead.
func GenerateTestSkeleton(root, symbol string) (TestSkeleton, error) {
	files, err := collectGoFiles(root, false)
	if err != nil {
		return TestSkeleton{}, err
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

		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Name == nil || fd.Name.Name != symbol {
				continue
			}
			if fd.Recv != nil {
				return TestSkeleton{}, fmt.Errorf("%s is a method; test generation only supports package-level functions", symbol)
			}
			return TestSkeleton{
				FuncName: symbol,
				Skeleton: buildTestSkeleton(fd),
				Imports:  requiredImports(fd),
				File:     path,
				Line:     fset.Position(fd.Name.Pos()).Line,
			}, nil
		}
	}
	return TestSkeleton{}, fmt.Errorf("no package-level function named %q found", symbol)
}

func requiredImports(fd *ast.FuncDecl) []string {
	imports := []string{"testing"}
	results := resultShape(fd.Type.Results)
	if results.count > 0 && results.count <= 2 {
		imports = append(imports, "github.com/stretchr/testify/require")
	}
	for _, p := range allParams(fd.Type.Params) {
		if p.typ == "context.Context" {
			imports = append(imports, "context")
			break
		}
	}
	return imports
}

func buildTestSkeleton(fd *ast.FuncDecl) string {
	params := allParams(fd.Type.Params)
	results := resultShape(fd.Type.Results)

	var b strings.Builder
	fmt.Fprintf(&b, "func Test%s(t *testing.T) {\n", exportedTestName(fd.Name.Name))
	b.WriteString("\ttests := []struct {\n\t\tname string\n")
	for _, p := range params {
		if p.typ == "context.Context" {
			continue // Passed as context.Background() in the call, not a table field.
		}
		fmt.Fprintf(&b, "\t\t%s %s\n", p.name, p.typ)
	}
	if results.value != "" {
		fmt.Fprintf(&b, "\t\twant %s\n", results.value)
	}
	if results.hasError {
		b.WriteString("\t\twantErr bool\n")
	}
	b.WriteString("\t}{\n\t\t{name: \"TODO\"},\n\t}\n\n")

	b.WriteString("\tfor _, tt := range tests {\n\t\tt.Run(tt.name, func(t *testing.T) {\n")
	writeCall(&b, fd.Name.Name, params, results)
	b.WriteString("\t\t})\n\t}\n}\n")
	return b.String()
}

func writeCall(b *strings.Builder, funcName string, params []namedParam, results resultInfo) {
	args := make([]string, len(params))
	for i, p := range params {
		if p.typ == "context.Context" {
			args[i] = "context.Background()"
			continue
		}
		args[i] = "tt." + p.name
	}
	call := fmt.Sprintf("%s(%s)", funcName, strings.Join(args, ", "))

	switch {
	case results.count == 0:
		fmt.Fprintf(b, "\t\t\t%s\n", call)
	case results.count == 1 && results.hasError:
		fmt.Fprintf(b, "\t\t\terr := %s\n", call)
		b.WriteString("\t\t\trequire.Equal(t, tt.wantErr, err != nil)\n")
	case results.count == 1:
		fmt.Fprintf(b, "\t\t\tgot := %s\n", call)
		b.WriteString("\t\t\trequire.Equal(t, tt.want, got)\n")
	case results.count == 2 && results.hasError:
		fmt.Fprintf(b, "\t\t\tgot, err := %s\n", call)
		b.WriteString("\t\t\trequire.Equal(t, tt.wantErr, err != nil)\n")
		b.WriteString("\t\t\tif !tt.wantErr {\n\t\t\t\trequire.Equal(t, tt.want, got)\n\t\t\t}\n")
	default:
		fmt.Fprintf(b, "\t\t\t%s // TODO: capture and assert %d return values\n", call, results.count)
	}
}

// exportedTestName gives an unexported function's test a name that is
// still exported (Go requires TestXxx to start with an uppercase letter
// to be discovered as a test at all).
func exportedTestName(name string) string {
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

type resultInfo struct {
	count    int
	value    string // Non-error result's type, when there is exactly one.
	hasError bool
}

func resultShape(fl *ast.FieldList) resultInfo {
	if fl == nil {
		return resultInfo{}
	}
	info := resultInfo{count: fieldCount(fl)}
	for _, field := range fl.List {
		typ := exprTypeName(field.Type)
		n := len(field.Names)
		if n == 0 {
			n = 1
		}
		for range n {
			if typ == "error" {
				info.hasError = true
			} else if info.value == "" {
				info.value = typ
			}
		}
	}
	return info
}

func fieldCount(fl *ast.FieldList) int {
	n := 0
	for _, field := range fl.List {
		if len(field.Names) == 0 {
			n++
		} else {
			n += len(field.Names)
		}
	}
	return n
}

// allParams lists every parameter by name, generating "argN" for an
// unnamed one -- unlike describableParams, this keeps a lone
// context.Context, because a generated call still has to pass something
// for it.
func allParams(fl *ast.FieldList) []namedParam {
	if fl == nil {
		return nil
	}
	var out []namedParam
	argN := 0
	nextName := func() string {
		argN++
		return fmt.Sprintf("arg%d", argN)
	}
	for _, field := range fl.List {
		typ := exprTypeName(field.Type)
		if len(field.Names) == 0 {
			out = append(out, namedParam{name: nextName(), typ: typ})
			continue
		}
		for _, n := range field.Names {
			name := n.Name
			if name == "_" || name == "" {
				name = nextName()
			}
			out = append(out, namedParam{name: name, typ: typ})
		}
	}
	return out
}
