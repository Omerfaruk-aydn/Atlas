package codeintel

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Todo is one marker found in a comment.
type Todo struct {
	// Kind is the normalised marker: TODO, FIXME, HACK, XXX, BUG,
	// NOTE, OPTIMIZE, DEPRECATED.
	Kind string
	// Owner is the name in "TODO(alice):" when there is one. An owned
	// marker is one somebody can be asked about; an unowned one usually
	// belongs to nobody.
	Owner string
	// Ticket is an issue reference found in the text, e.g. "#1234" or
	// "PROJ-42".
	Ticket  string
	File    string
	Line    int
	Text    string
	Context string
}

// TodoResult is one scan.
type TodoResult struct {
	Todos        []Todo
	FilesScanned int
	// ByKind counts each marker type.
	ByKind map[string]int
	// Truncated reports that the scan stopped at its limit.
	Truncated bool
}

// todoPattern matches a marker in a comment.
//
// Two constraints, both of which a real scan of a real repository proved
// necessary:
//
// The marker must follow an actual comment opener. Allowing it merely to
// start a line matches `todo := Todo{`, because the identifier sits at a
// word boundary after leading whitespace.
//
// And it must then look like a marker rather than a word: either
// ALL-CAPS (`// TODO fix this`) or immediately followed by a colon or an
// owner in parentheses (`// todo: fix`, `// Todo(alice): fix`). Without
// that, every Go doc comment on a type named Todo or a function named
// Bug is reported -- which on this repository was most of the findings.
var todoPattern = regexp.MustCompile(
	`(?:` +
		// ALL-CAPS marker after a comment opener, colon optional.
		`(?://|/\*|#|--|<!--|\*)+\s*\b(TODO|FIXME|HACK|XXX|BUG|OPTIMISE|OPTIMIZE|DEPRECATED)\b\s*(?:\(([^)]{1,40})\))?\s*:?\s*(.*)$` +
		`|` +
		// Any case, but then the colon or owner is required.
		`(?i:(?:(?://|/\*|#|--|<!--|\*)+\s*)\b(TODO|FIXME|HACK|XXX|BUG|OPTIMI[SZ]E|DEPRECATED)\b\s*(?:\(([^)]{1,40})\))?\s*:\s*(.*)$)` +
		`)`)

// ticketPattern matches common issue references.
var ticketPattern = regexp.MustCompile(`(#\d+|[A-Z][A-Z0-9]+-\d+)`)

// TodoOptions narrows a scan.
type TodoOptions struct {
	// Kinds, when non-empty, restricts to these markers (case
	// insensitive).
	Kinds []string
	// MaxResults caps findings. Zero means 500.
	MaxResults int
	// IncludeTests scans _test.go and similar files too.
	IncludeTests bool
	// Extensions, when non-empty, restricts to these file extensions
	// (with the leading dot).
	Extensions []string
}

const defaultTodoMax = 500

// scannableExts are text file types worth scanning. An allowlist rather
// than a denylist: a repository contains far more kinds of binary and
// generated file than can be enumerated, and reading them produces
// nothing but noise and wasted time.
var scannableExts = map[string]bool{
	".go": true, ".js": true, ".jsx": true, ".ts": true, ".tsx": true,
	".py": true, ".rb": true, ".rs": true, ".java": true, ".kt": true,
	".c": true, ".h": true, ".cc": true, ".cpp": true, ".hpp": true,
	".cs": true, ".swift": true, ".php": true, ".scala": true, ".sh": true,
	".bash": true, ".zsh": true, ".sql": true, ".yaml": true, ".yml": true,
	".toml": true, ".tf": true, ".proto": true, ".md": true, ".css": true,
	".scss": true, ".html": true, ".vue": true, ".svelte": true, ".lua": true,
	".ex": true, ".exs": true, ".erl": true, ".dart": true, ".m": true,
	".mm": true, ".zig": true, ".nim": true, ".pl": true, ".r": true,
}

// FindTodos scans root for markers left in comments.
func FindTodos(root string, opts TodoOptions) (TodoResult, error) {
	if opts.MaxResults <= 0 {
		opts.MaxResults = defaultTodoMax
	}

	wanted := map[string]bool{}
	for _, k := range opts.Kinds {
		wanted[strings.ToUpper(strings.TrimSpace(k))] = true
	}
	extFilter := map[string]bool{}
	for _, e := range opts.Extensions {
		e = strings.ToLower(strings.TrimSpace(e))
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		extFilter[e] = true
	}

	info, err := os.Stat(root)
	if err != nil {
		return TodoResult{}, fmt.Errorf("cannot scan %s: %w", root, err)
	}

	result := TodoResult{ByKind: map[string]int{}}

	scanOne := func(path string) {
		todos, err := scanFileForTodos(path, wanted, opts.MaxResults-len(result.Todos))
		if err != nil {
			return
		}
		result.FilesScanned++
		result.Todos = append(result.Todos, todos...)
	}

	if !info.IsDir() {
		scanOne(root)
	} else {
		err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				if path != root && (skipTodoDirs[name] || strings.HasPrefix(name, ".")) {
					return filepath.SkipDir
				}
				return nil
			}
			if len(result.Todos) >= opts.MaxResults {
				result.Truncated = true
				return filepath.SkipAll
			}
			ext := strings.ToLower(filepath.Ext(path))
			if len(extFilter) > 0 {
				if !extFilter[ext] {
					return nil
				}
			} else if !scannableExts[ext] {
				return nil
			}
			if !opts.IncludeTests && isTestPath(path) {
				return nil
			}
			scanOne(path)
			return nil
		})
		if err != nil {
			return result, err
		}
	}

	for _, t := range result.Todos {
		result.ByKind[t.Kind]++
	}

	sort.SliceStable(result.Todos, func(i, j int) bool {
		if result.Todos[i].File != result.Todos[j].File {
			return result.Todos[i].File < result.Todos[j].File
		}
		return result.Todos[i].Line < result.Todos[j].Line
	})
	return result, nil
}

var skipTodoDirs = map[string]bool{
	"node_modules": true, "vendor": true, "dist": true, "build": true,
	"target": true, ".venv": true, "venv": true, "__pycache__": true,
	"testdata": true, "coverage": true,
}

func isTestPath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return strings.HasSuffix(base, "_test.go") ||
		strings.HasPrefix(base, "test_") ||
		strings.Contains(base, ".test.") ||
		strings.Contains(base, ".spec.")
}

func scanFileForTodos(path string, wanted map[string]bool, budget int) ([]Todo, error) {
	if budget <= 0 {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var todos []Todo
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		// A very long line is a minified bundle or generated data, and
		// running the pattern over it finds nothing worth having.
		if len(line) > 2000 {
			continue
		}

		m := todoPattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		// The pattern has two alternatives; whichever matched leaves the
		// other's groups empty.
		marker, owner, text := m[1], m[2], m[3]
		if marker == "" {
			marker, owner, text = m[4], m[5], m[6]
		}

		kind := strings.ToUpper(marker)
		// OPTIMISE and OPTIMIZE are the same marker; reporting them as
		// two kinds splits one count in half.
		if strings.HasPrefix(kind, "OPTIMI") {
			kind = "OPTIMIZE"
		}
		if len(wanted) > 0 && !wanted[kind] {
			continue
		}

		text = strings.TrimSpace(text)
		text = strings.TrimSuffix(text, "*/")
		text = strings.TrimSuffix(text, "-->")
		text = strings.TrimSpace(text)

		todo := Todo{
			Kind:    kind,
			Owner:   strings.TrimSpace(owner),
			File:    path,
			Line:    lineNo,
			Text:    text,
			Context: strings.TrimSpace(line),
		}
		if ticket := ticketPattern.FindString(line); ticket != "" {
			todo.Ticket = ticket
		}

		todos = append(todos, todo)
		if len(todos) >= budget {
			break
		}
	}
	return todos, nil
}
