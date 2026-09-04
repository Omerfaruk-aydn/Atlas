// Package docindex builds a table of contents across a tree's Markdown
// files.
//
// It reads ATX headings ("# Title", "## Section", ...) line by line. That
// covers the overwhelming majority of real-world Markdown and keeps the
// parser a handful of string operations instead of a full CommonMark
// implementation; Setext headings (a line underlined with "===" or
// "---") are not recognised, which the tool's own output says plainly
// rather than silently under-reporting a file's structure.
package docindex

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Heading is one ATX heading found in a Markdown file.
type Heading struct {
	Level int
	Text  string
	Line  int
}

// DocFile is one Markdown file's table of contents.
type DocFile struct {
	Path string
	// Title is the file's first H1, or its filename (without extension,
	// with separators turned into spaces) when it has none.
	Title    string
	Headings []Heading
}

// Options narrows a scan.
type Options struct {
	// Query restricts the result to files whose title or any heading
	// contains this text, case-insensitively. Empty returns every file.
	Query string
	// MaxDepth caps which heading levels are kept. Zero means all six.
	MaxDepth int
}

// Result is the outcome of a scan.
type Result struct {
	Files        []DocFile
	FilesScanned int
}

// Build walks root and indexes every Markdown file's headings.
func Build(root string, opts Options) (Result, error) {
	files, err := collectMarkdownFiles(root)
	if err != nil {
		return Result{}, err
	}

	maxDepth := opts.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 6
	}
	query := strings.ToLower(strings.TrimSpace(opts.Query))

	result := Result{}
	for _, path := range files {
		doc, err := indexFile(path, maxDepth)
		if err != nil {
			continue
		}
		result.FilesScanned++
		if query != "" && !docMatches(doc, query) {
			continue
		}
		result.Files = append(result.Files, doc)
	}

	sort.Slice(result.Files, func(i, j int) bool {
		return result.Files[i].Path < result.Files[j].Path
	})
	return result, nil
}

func docMatches(doc DocFile, query string) bool {
	if strings.Contains(strings.ToLower(doc.Title), query) {
		return true
	}
	for _, h := range doc.Headings {
		if strings.Contains(strings.ToLower(h.Text), query) {
			return true
		}
	}
	return false
}

func indexFile(path string, maxDepth int) (DocFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return DocFile{}, err
	}
	defer f.Close()

	doc := DocFile{Path: path}
	inFence := false
	lineNo := 0

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// A "#" inside a fenced code block is source, not structure.
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}

		level, text, ok := parseATXHeading(line)
		if !ok || level > maxDepth {
			continue
		}
		if level == 1 && doc.Title == "" {
			doc.Title = text
		}
		doc.Headings = append(doc.Headings, Heading{Level: level, Text: text, Line: lineNo})
	}
	if err := scanner.Err(); err != nil {
		return DocFile{}, err
	}

	if doc.Title == "" {
		doc.Title = titleFromFilename(path)
	}
	return doc, nil
}

// parseATXHeading reads "#"*1-6 followed by a space and text. A run of
// "#" with no following space is not a heading -- it's usually a
// hashtag-like comment or, in a shell fence that slipped past the code
// detection, a literal "#" character.
func parseATXHeading(line string) (level int, text string, ok bool) {
	trimmed := strings.TrimLeft(line, " \t")
	i := 0
	for i < len(trimmed) && trimmed[i] == '#' {
		i++
	}
	if i == 0 || i > 6 || i >= len(trimmed) || trimmed[i] != ' ' {
		return 0, "", false
	}
	text = strings.TrimSpace(trimmed[i:])
	text = strings.TrimRight(text, "#")
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, "", false
	}
	return i, text, true
}

func titleFromFilename(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	base = strings.ReplaceAll(base, "_", " ")
	base = strings.ReplaceAll(base, "-", " ")
	return base
}

func collectMarkdownFiles(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{root}, nil
	}

	var files []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			if path != root && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		lower := strings.ToLower(d.Name())
		if strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".mdx") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}
