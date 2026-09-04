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

// SecurityFinding is one syntactic security smell found in a Go source file.
type SecurityFinding struct {
	// Kind is one of "hardcoded-credential", "weak-crypto", "insecure-tls",
	// "sql-injection-risk", "command-injection-risk".
	Kind    string
	File    string
	Line    int
	Func    string
	Message string
	Snippet string
}

// SecurityScanOptions narrows a scan.
type SecurityScanOptions struct {
	// IncludeTests scans _test.go files too. Off by default: fixtures and
	// mock secrets in tests are normal and not worth flagging.
	IncludeTests bool
}

// SecurityScanResult is the outcome of a scan.
type SecurityScanResult struct {
	Findings     []SecurityFinding
	FilesScanned int
	ByKind       map[string]int
}

var weakCryptoPackages = map[string]string{
	"md5":  "crypto/md5",
	"sha1": "crypto/sha1",
	"des":  "crypto/des",
	"rc4":  "crypto/rc4",
}

var sqlExecMethods = map[string]bool{
	"Query": true, "QueryContext": true, "QueryRow": true, "QueryRowContext": true,
	"Exec": true, "ExecContext": true,
}

var credentialNameHints = []string{"password", "passwd", "secret", "apikey", "api_key", "token", "credential", "privatekey", "private_key"}

func looksLikeCredentialName(name string) bool {
	lower := strings.ToLower(name)
	for _, hint := range credentialNameHints {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}

// isPlaceholderLiteral reports whether a string literal is too short or too
// generic to be a real secret -- an empty default, "TODO", "changeme", and
// the like show up constantly in legitimate struct field defaults.
func isPlaceholderLiteral(v string) bool {
	trimmed := strings.Trim(v, `"`+"`")
	if len(trimmed) < 8 {
		return true
	}
	lower := strings.ToLower(trimmed)
	for _, placeholder := range []string{"todo", "changeme", "xxx", "example", "placeholder", "your-", "<", "$"} {
		if strings.Contains(lower, placeholder) {
			return true
		}
	}
	return false
}

// SecurityScan walks root's Go source and flags syntactic security smells:
// hardcoded credentials, use of broken cryptographic primitives, disabled
// TLS verification, and string-built SQL or shell commands.
func SecurityScan(root string, opts SecurityScanOptions) (SecurityScanResult, error) {
	files, err := collectGoFiles(root, opts.IncludeTests)
	if err != nil {
		return SecurityScanResult{}, err
	}

	result := SecurityScanResult{ByKind: map[string]int{}}
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

		imported := importedWeakCrypto(file)
		for pkg, importPath := range imported {
			pos := fset.Position(file.Pos())
			result.Findings = append(result.Findings, SecurityFinding{
				Kind:    "weak-crypto",
				File:    path,
				Line:    pos.Line,
				Message: fmt.Sprintf("imports %s, a broken or deprecated cryptographic primitive", importPath),
				Snippet: fmt.Sprintf("import %q", importPath),
			})
			_ = pkg
		}

		scanFileBody(file, path, fset, &result.Findings)
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

func importedWeakCrypto(file *ast.File) map[string]string {
	found := map[string]string{}
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		for name, weakPath := range weakCryptoPackages {
			if path == weakPath {
				found[name] = weakPath
			}
		}
	}
	return found
}

func scanFileBody(file *ast.File, path string, fset *token.FileSet, findings *[]SecurityFinding) {
	var currentFunc string
	ast.Inspect(file, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.FuncDecl:
			if v.Name != nil {
				currentFunc = v.Name.Name
			}
		case *ast.AssignStmt:
			checkHardcodedCredentialAssign(v, path, fset, currentFunc, findings)
		case *ast.ValueSpec:
			checkHardcodedCredentialSpec(v, path, fset, currentFunc, findings)
		case *ast.KeyValueExpr:
			checkInsecureTLS(v, path, fset, currentFunc, findings)
		case *ast.CallExpr:
			checkSQLInjectionRisk(v, path, fset, currentFunc, findings)
			checkCommandInjectionRisk(v, path, fset, currentFunc, findings)
		}
		return true
	})
}

func checkHardcodedCredentialAssign(v *ast.AssignStmt, path string, fset *token.FileSet, fn string, findings *[]SecurityFinding) {
	if len(v.Lhs) != len(v.Rhs) {
		return
	}
	for i, lhs := range v.Lhs {
		id, ok := lhs.(*ast.Ident)
		if !ok || !looksLikeCredentialName(id.Name) {
			continue
		}
		lit, ok := v.Rhs[i].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING || isPlaceholderLiteral(lit.Value) {
			continue
		}
		*findings = append(*findings, SecurityFinding{
			Kind:    "hardcoded-credential",
			File:    path,
			Line:    fset.Position(lit.Pos()).Line,
			Func:    fn,
			Message: fmt.Sprintf("%s is assigned a literal string that looks like a real credential", id.Name),
			Snippet: fmt.Sprintf("%s = %s", id.Name, redactCredential(lit.Value)),
		})
	}
}

func checkHardcodedCredentialSpec(v *ast.ValueSpec, path string, fset *token.FileSet, fn string, findings *[]SecurityFinding) {
	if len(v.Names) != len(v.Values) {
		return
	}
	for i, name := range v.Names {
		if !looksLikeCredentialName(name.Name) {
			continue
		}
		lit, ok := v.Values[i].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING || isPlaceholderLiteral(lit.Value) {
			continue
		}
		*findings = append(*findings, SecurityFinding{
			Kind:    "hardcoded-credential",
			File:    path,
			Line:    fset.Position(lit.Pos()).Line,
			Func:    fn,
			Message: fmt.Sprintf("%s is declared with a literal string that looks like a real credential", name.Name),
			Snippet: fmt.Sprintf("%s = %s", name.Name, redactCredential(lit.Value)),
		})
	}
}

func checkInsecureTLS(v *ast.KeyValueExpr, path string, fset *token.FileSet, fn string, findings *[]SecurityFinding) {
	key, ok := v.Key.(*ast.Ident)
	if !ok || key.Name != "InsecureSkipVerify" {
		return
	}
	val, ok := v.Value.(*ast.Ident)
	if !ok || val.Name != "true" {
		return
	}
	*findings = append(*findings, SecurityFinding{
		Kind:    "insecure-tls",
		File:    path,
		Line:    fset.Position(v.Pos()).Line,
		Func:    fn,
		Message: "InsecureSkipVerify: true disables TLS certificate verification",
		Snippet: "InsecureSkipVerify: true",
	})
}

func checkSQLInjectionRisk(v *ast.CallExpr, path string, fset *token.FileSet, fn string, findings *[]SecurityFinding) {
	sel, ok := v.Fun.(*ast.SelectorExpr)
	if !ok || !sqlExecMethods[sel.Sel.Name] {
		return
	}
	if len(v.Args) == 0 {
		return
	}
	if isSprintfCall(v.Args[0]) {
		*findings = append(*findings, SecurityFinding{
			Kind:    "sql-injection-risk",
			File:    path,
			Line:    fset.Position(v.Pos()).Line,
			Func:    fn,
			Message: fmt.Sprintf("%s query is built with fmt.Sprintf instead of a parameterized placeholder", sel.Sel.Name),
			Snippet: fmt.Sprintf("%s(fmt.Sprintf(...))", sel.Sel.Name),
		})
	}
}

func checkCommandInjectionRisk(v *ast.CallExpr, path string, fset *token.FileSet, fn string, findings *[]SecurityFinding) {
	sel, ok := v.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Command" {
		return
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "exec" {
		return
	}
	for _, arg := range v.Args {
		if isSprintfCall(arg) {
			*findings = append(*findings, SecurityFinding{
				Kind:    "command-injection-risk",
				File:    path,
				Line:    fset.Position(v.Pos()).Line,
				Func:    fn,
				Message: "exec.Command argument is built with fmt.Sprintf instead of passed as a separate argument",
				Snippet: "exec.Command(..., fmt.Sprintf(...))",
			})
			return
		}
	}
}

func isSprintfCall(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Sprintf" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "fmt"
}

// redactCredential mirrors scan_secrets' own rule: a finding names the
// shape of a problem, never the value, so a value that turns out to be a
// live credential is never one field-read away from leaking into a
// transcript or a log.
func redactCredential(litValue string) string {
	v := strings.Trim(litValue, "`\"")
	if len(v) <= 8 {
		return `"` + strings.Repeat("*", len(v)) + `"`
	}
	return `"` + v[:4] + strings.Repeat("*", len(v)-8) + v[len(v)-4:] + `"`
}
