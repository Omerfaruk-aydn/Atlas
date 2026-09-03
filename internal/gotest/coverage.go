package gotest

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
)

// Block is one coverage-profile record: a contiguous span of source with
// a single execution count.
type Block struct {
	File      string
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
	// Statements is how many statements the block contains. Coverage is
	// measured in statements, not lines, which is why a 40-line function
	// can be one block.
	Statements int
	Count      int
}

// Covered reports whether the block executed at least once.
func (b Block) Covered() bool { return b.Count > 0 }

// FileCoverage aggregates one file.
type FileCoverage struct {
	File       string
	Statements int
	Covered    int
	// UncoveredBlocks lists the spans that never executed, in source
	// order -- the actual answer to "what is not tested".
	UncoveredBlocks []Block
}

// Percent returns the covered fraction, or 100 for a file with no
// statements at all: a file with nothing to cover is not 0% covered, and
// reporting it that way drags an average down for no reason.
func (f FileCoverage) Percent() float64 {
	if f.Statements == 0 {
		return 100
	}
	return float64(f.Covered) * 100 / float64(f.Statements)
}

// PackageCoverage aggregates one package.
type PackageCoverage struct {
	Package    string
	Statements int
	Covered    int
}

// Percent returns the covered fraction for the package.
func (p PackageCoverage) Percent() float64 {
	if p.Statements == 0 {
		return 100
	}
	return float64(p.Covered) * 100 / float64(p.Statements)
}

// Coverage is a parsed profile.
type Coverage struct {
	Mode       string
	Files      []FileCoverage
	Packages   []PackageCoverage
	Statements int
	Covered    int
}

// Percent returns overall statement coverage.
func (c Coverage) Percent() float64 {
	if c.Statements == 0 {
		return 0
	}
	return float64(c.Covered) * 100 / float64(c.Statements)
}

// ParseCoverageFile reads a profile written by `go test -coverprofile`.
//
// The format is a mode line followed by one record per block:
//
//	name.go:startLine.startCol,endLine.endCol numStatements count
//
// Records for the same block can appear more than once when several test
// binaries contributed, so counts are accumulated rather than replaced.
// Overwriting instead would report a block as uncovered whenever the
// last binary to mention it happened not to reach it.
func ParseCoverageFile(profilePath string) (Coverage, error) {
	f, err := os.Open(profilePath)
	if err != nil {
		return Coverage{}, fmt.Errorf("cannot read coverage profile: %w", err)
	}
	defer f.Close()

	var (
		cov     Coverage
		blocks  = map[string]*Block{}
		order   []string
		scanner = bufio.NewScanner(f)
	)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if after, ok := strings.CutPrefix(line, "mode:"); ok {
			cov.Mode = strings.TrimSpace(after)
			continue
		}

		block, err := parseBlockLine(line)
		if err != nil {
			continue // A malformed record is skipped, not fatal.
		}

		id := fmt.Sprintf("%s:%d.%d,%d.%d", block.File,
			block.StartLine, block.StartCol, block.EndLine, block.EndCol)
		if existing, ok := blocks[id]; ok {
			existing.Count += block.Count
			continue
		}
		b := block
		blocks[id] = &b
		order = append(order, id)
	}
	if err := scanner.Err(); err != nil {
		return cov, fmt.Errorf("cannot read coverage profile: %w", err)
	}

	byFile := map[string]*FileCoverage{}
	byPackage := map[string]*PackageCoverage{}
	for _, id := range order {
		b := blocks[id]

		fc, ok := byFile[b.File]
		if !ok {
			fc = &FileCoverage{File: b.File}
			byFile[b.File] = fc
		}
		fc.Statements += b.Statements
		if b.Covered() {
			fc.Covered += b.Statements
		} else {
			fc.UncoveredBlocks = append(fc.UncoveredBlocks, *b)
		}

		pkgName := path.Dir(b.File)
		pc, ok := byPackage[pkgName]
		if !ok {
			pc = &PackageCoverage{Package: pkgName}
			byPackage[pkgName] = pc
		}
		pc.Statements += b.Statements
		if b.Covered() {
			pc.Covered += b.Statements
		}

		cov.Statements += b.Statements
		if b.Covered() {
			cov.Covered += b.Statements
		}
	}

	for _, fc := range byFile {
		sort.Slice(fc.UncoveredBlocks, func(i, j int) bool {
			return fc.UncoveredBlocks[i].StartLine < fc.UncoveredBlocks[j].StartLine
		})
		cov.Files = append(cov.Files, *fc)
	}
	for _, pc := range byPackage {
		cov.Packages = append(cov.Packages, *pc)
	}

	// Least covered first: the whole point of the report is finding what
	// is untested, and a list sorted by name buries it.
	sort.Slice(cov.Files, func(i, j int) bool {
		if cov.Files[i].Percent() != cov.Files[j].Percent() {
			return cov.Files[i].Percent() < cov.Files[j].Percent()
		}
		return cov.Files[i].File < cov.Files[j].File
	})
	sort.Slice(cov.Packages, func(i, j int) bool {
		if cov.Packages[i].Percent() != cov.Packages[j].Percent() {
			return cov.Packages[i].Percent() < cov.Packages[j].Percent()
		}
		return cov.Packages[i].Package < cov.Packages[j].Package
	})

	return cov, nil
}

// parseBlockLine reads one profile record.
func parseBlockLine(line string) (Block, error) {
	// Split on the LAST colon before the position span: a Windows path
	// or a module path with a port-like segment can contain colons, and
	// splitting on the first would cut the filename in half.
	colon := strings.LastIndex(line, ":")
	if colon < 0 {
		return Block{}, fmt.Errorf("no position separator")
	}
	file := line[:colon]
	rest := line[colon+1:]

	fields := strings.Fields(rest)
	if len(fields) != 3 {
		return Block{}, fmt.Errorf("expected 3 fields, got %d", len(fields))
	}

	startPos, endPos, ok := strings.Cut(fields[0], ",")
	if !ok {
		return Block{}, fmt.Errorf("malformed span")
	}
	startLine, startCol, err := parsePos(startPos)
	if err != nil {
		return Block{}, err
	}
	endLine, endCol, err := parsePos(endPos)
	if err != nil {
		return Block{}, err
	}

	statements, err := strconv.Atoi(fields[1])
	if err != nil {
		return Block{}, err
	}
	count, err := strconv.Atoi(fields[2])
	if err != nil {
		return Block{}, err
	}

	return Block{
		File:       file,
		StartLine:  startLine,
		StartCol:   startCol,
		EndLine:    endLine,
		EndCol:     endCol,
		Statements: statements,
		Count:      count,
	}, nil
}

func parsePos(s string) (line, col int, err error) {
	lineStr, colStr, ok := strings.Cut(s, ".")
	if !ok {
		return 0, 0, fmt.Errorf("malformed position %q", s)
	}
	if line, err = strconv.Atoi(lineStr); err != nil {
		return 0, 0, err
	}
	if col, err = strconv.Atoi(colStr); err != nil {
		return 0, 0, err
	}
	return line, col, nil
}
