package codeintel

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// EnvVarUsage is one os.Getenv/LookupEnv/Setenv call site.
type EnvVarUsage struct {
	// Name is the literal environment variable name, or "(dynamic)" when
	// the call built its key from something other than a string
	// literal -- a format string, a variable, string concatenation.
	Name string
	// Kind is "read" (Getenv, LookupEnv) or "write" (Setenv).
	Kind string
	File string
	Line int
}

// EnvAuditResult is the outcome of a scan.
type EnvAuditResult struct {
	Usages []EnvVarUsage
	// Undocumented lists the distinct literal names read but not found
	// in an example env file, sorted. Empty when no such file exists to
	// compare against -- see EnvFileFound.
	Undocumented []string
	// EnvFileFound reports whether a .env.example / .env.sample was
	// found to compare against at all. When false, Undocumented is not
	// meaningful -- there was nothing to check documentation against.
	EnvFileFound bool
	EnvFilePath  string
	FilesScanned int
}

var envMethods = map[string]string{
	"Getenv": "read", "LookupEnv": "read", "Setenv": "write",
}

// envExampleNames is tried in order; the first one found is used.
var envExampleNames = []string{".env.example", ".env.sample", ".env.dist"}

// AuditEnvVars walks root for Go source and lists every os.Getenv,
// os.LookupEnv, and os.Setenv call, then cross-checks the variables read
// against an example env file (.env.example, .env.sample, or .env.dist,
// whichever is found first at root) if one exists.
func AuditEnvVars(root string, includeTests bool) (EnvAuditResult, error) {
	files, err := collectGoFiles(root, includeTests)
	if err != nil {
		return EnvAuditResult{}, err
	}

	result := EnvAuditResult{}
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
		result.Usages = append(result.Usages, findEnvUsages(fset, file, path)...)
	}

	sort.SliceStable(result.Usages, func(i, j int) bool {
		if result.Usages[i].File != result.Usages[j].File {
			return result.Usages[i].File < result.Usages[j].File
		}
		return result.Usages[i].Line < result.Usages[j].Line
	})

	documented, envPath, found := loadEnvExample(root)
	result.EnvFileFound = found
	result.EnvFilePath = envPath
	if found {
		seen := map[string]bool{}
		for _, u := range result.Usages {
			if u.Kind != "read" || u.Name == "(dynamic)" || documented[u.Name] || seen[u.Name] {
				continue
			}
			seen[u.Name] = true
			result.Undocumented = append(result.Undocumented, u.Name)
		}
		sort.Strings(result.Undocumented)
	}
	return result, nil
}

func findEnvUsages(fset *token.FileSet, file *ast.File, path string) []EnvVarUsage {
	var out []EnvVarUsage
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "os" {
			return true
		}
		kind, ok := envMethods[sel.Sel.Name]
		if !ok || len(call.Args) == 0 {
			return true
		}

		name := "(dynamic)"
		if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if v, err := strconv.Unquote(lit.Value); err == nil {
				name = v
			}
		}
		out = append(out, EnvVarUsage{
			Name: name, Kind: kind, File: path, Line: fset.Position(call.Pos()).Line,
		})
		return true
	})
	return out
}

// loadEnvExample reads the first example env file found directly in
// root and returns the set of keys it declares.
func loadEnvExample(root string) (map[string]bool, string, bool) {
	dir := root
	if info, err := os.Stat(root); err == nil && !info.IsDir() {
		dir = filepath.Dir(root)
	}

	for _, name := range envExampleNames {
		path := filepath.Join(dir, name)
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		defer f.Close()

		keys := map[string]bool{}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, _, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			keys[strings.TrimSpace(key)] = true
		}
		return keys, path, true
	}
	return nil, "", false
}
