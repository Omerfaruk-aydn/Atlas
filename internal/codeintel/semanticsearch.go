package codeintel

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// CodeSymbol is one top-level declaration -- exported or not, unlike
// APISurface, because the target of a search across an unfamiliar
// codebase is just as often an unexported helper as a public entry
// point.
type CodeSymbol struct {
	Name      string
	Kind      string // "func", "method", "type", "const", "var"
	Recv      string
	Signature string
	// Doc is the full doc comment text, not just its first line -- a
	// search wants every word available to match against.
	Doc  string
	File string
	Line int
}

// SymbolMatch is one search result.
type SymbolMatch struct {
	CodeSymbol
	Score int
	// MatchedTerms lists which query words contributed to the score, so
	// a result can be explained rather than trusted blindly.
	MatchedTerms []string
}

// SearchOptions narrows a search.
type SearchOptions struct {
	IncludeTests bool
	// Limit caps how many matches are returned. Zero means 10.
	Limit int
}

// SearchResult is the outcome of a search.
type SearchResult struct {
	Matches      []SymbolMatch
	FilesScanned int
}

// SemanticSearch indexes every top-level declaration under root and
// ranks them against query by matching words in the declaration's name
// and doc comment -- not an embedding, just tokenised keyword overlap,
// weighted so a name match counts for more than a doc match. That is a
// real limitation: it finds "which symbol's name or comment mentions
// these words", not "which symbol does what you mean" -- a function with
// no doc comment and a name that doesn't share vocabulary with the query
// will not be found even if it's exactly the right one.
func SemanticSearch(root, query string, opts SearchOptions) (SearchResult, error) {
	files, err := collectGoFiles(root, opts.IncludeTests)
	if err != nil {
		return SearchResult{}, err
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}

	terms := tokenize(query)
	result := SearchResult{}
	var matches []SymbolMatch

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

		for _, sym := range collectAllSymbols(fset, file, path) {
			score, matched := scoreSymbol(sym, terms)
			if score > 0 {
				matches = append(matches, SymbolMatch{CodeSymbol: sym, Score: score, MatchedTerms: matched})
			}
		}
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].Name < matches[j].Name
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	result.Matches = matches
	return result, nil
}

var nonWordRun = regexp.MustCompile(`[^a-z0-9]+`)

func tokenize(s string) []string {
	lower := strings.ToLower(s)
	fields := nonWordRun.Split(lower, -1)
	var out []string
	for _, f := range fields {
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// splitIdentifier breaks a Go identifier into lowercase words along
// camelCase and snake_case boundaries, so a query for "generate docstring"
// can find a symbol named "GenerateDocstrings" or "generate_docstrings".
func splitIdentifier(name string) []string {
	var b strings.Builder
	runes := []rune(name)
	for i, r := range runes {
		if r == '_' {
			b.WriteByte(' ')
			continue
		}
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && nextLower) {
				b.WriteByte(' ')
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return strings.Fields(b.String())
}

func scoreSymbol(sym CodeSymbol, terms []string) (int, []string) {
	if len(terms) == 0 {
		return 0, nil
	}
	nameWords := splitIdentifier(sym.Name)
	nameWordSet := map[string]bool{}
	for _, w := range nameWords {
		nameWordSet[w] = true
	}
	nameLower := strings.ToLower(sym.Name)
	docLower := strings.ToLower(sym.Doc)

	score := 0
	var matched []string
	for _, term := range terms {
		switch {
		case nameWordSet[term]:
			score += 3
			matched = append(matched, term)
		case strings.Contains(nameLower, term):
			score += 2
			matched = append(matched, term)
		case docLower != "" && strings.Contains(docLower, term):
			score += 1
			matched = append(matched, term)
		}
	}
	return score, matched
}

// collectAllSymbols lists every func, method, type, const, and var
// declared at package level in file -- exported or not.
func collectAllSymbols(fset *token.FileSet, file *ast.File, path string) []CodeSymbol {
	var out []CodeSymbol
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name == nil {
				continue
			}
			sym := CodeSymbol{
				Name:      d.Name.Name,
				Kind:      "func",
				Signature: "func " + d.Name.Name + renderFuncSig(fset, d.Type),
				Doc:       docText(d.Doc),
				File:      path,
				Line:      fset.Position(d.Name.Pos()).Line,
			}
			if d.Recv != nil && len(d.Recv.List) > 0 {
				recv, ptr := receiverName(d.Recv.List[0].Type)
				sym.Kind = "method"
				sym.Recv = recv
				star := ""
				if ptr {
					star = "*"
				}
				sym.Signature = "func (" + star + recv + ") " + d.Name.Name + renderFuncSig(fset, d.Type)
			}
			out = append(out, sym)

		case *ast.GenDecl:
			out = append(out, symbolsFromGenDecl(fset, d, path)...)
		}
	}
	return out
}

func symbolsFromGenDecl(fset *token.FileSet, d *ast.GenDecl, path string) []CodeSymbol {
	var out []CodeSymbol
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			doc := docText(s.Doc)
			if doc == "" {
				doc = docText(d.Doc)
			}
			out = append(out, CodeSymbol{
				Name:      s.Name.Name,
				Kind:      "type",
				Signature: "type " + s.Name.Name + " " + typeKeyword(s.Type),
				Doc:       doc,
				File:      path,
				Line:      fset.Position(s.Name.Pos()).Line,
			})

		case *ast.ValueSpec:
			kind := "var"
			if d.Tok == token.CONST {
				kind = "const"
			}
			doc := docText(s.Doc)
			if doc == "" {
				doc = docText(d.Doc)
			}
			for _, name := range s.Names {
				if name.Name == "_" {
					continue
				}
				sig := kind + " " + name.Name
				if s.Type != nil {
					sig += " " + renderNode(fset, s.Type)
				}
				out = append(out, CodeSymbol{
					Name:      name.Name,
					Kind:      kind,
					Signature: sig,
					Doc:       doc,
					File:      path,
					Line:      fset.Position(name.Pos()).Line,
				})
			}
		}
	}
	return out
}

func docText(doc *ast.CommentGroup) string {
	if doc == nil {
		return ""
	}
	return strings.TrimSpace(doc.Text())
}
