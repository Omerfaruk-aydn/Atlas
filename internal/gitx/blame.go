package gitx

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// BlameLine attributes one line of a file to the commit that last
// touched it.
type BlameLine struct {
	Line    int
	Hash    string
	Short   string
	Author  string
	Date    time.Time
	Summary string
	Content string
}

// BlameSpan groups consecutive lines that share a commit, which is how
// blame output is actually read: a block of lines arrived together.
type BlameSpan struct {
	Start   int
	End     int
	Hash    string
	Short   string
	Author  string
	Date    time.Time
	Summary string
}

// Lines reports how many lines the span covers.
func (s BlameSpan) Lines() int { return s.End - s.Start + 1 }

// BlameOptions narrows a blame query.
type BlameOptions struct {
	// StartLine and EndLine bound the range, 1-based and inclusive. Zero
	// means the whole file.
	StartLine int
	EndLine   int
	// Rev blames the file as of this revision instead of the working
	// tree.
	Rev string
	// IgnoreWhitespace stops a reformatting commit from claiming
	// authorship of every line it reindented.
	IgnoreWhitespace bool
}

// Blame attributes each line of path to the commit that last changed it.
//
// It uses --line-porcelain, which repeats the full commit header for
// every line. That is more output than --porcelain, which emits each
// header once and expects the consumer to carry state forward -- and
// carrying that state is exactly where blame parsers go wrong, silently
// attributing a block of lines to the previous commit. The extra bytes
// buy a parser that cannot drift.
func Blame(ctx context.Context, dir, path string, opts BlameOptions) ([]BlameLine, error) {
	args := []string{"blame", "--line-porcelain"}
	if opts.IgnoreWhitespace {
		args = append(args, "-w")
	}
	if opts.StartLine > 0 {
		end := opts.EndLine
		if end <= 0 {
			end = opts.StartLine
		}
		args = append(args, "-L", strconv.Itoa(opts.StartLine)+","+strconv.Itoa(end))
	}
	if opts.Rev != "" {
		args = append(args, opts.Rev)
	}
	args = append(args, "--", path)

	out, err := Run(ctx, dir, args...)
	if err != nil {
		return nil, err
	}
	return parseBlame(out), nil
}

// parseBlame reads --line-porcelain output. Each record starts with a
// header line "<hash> <origLine> <finalLine> [<groupSize>]", is followed
// by key/value lines, and ends with the source line prefixed by a tab.
func parseBlame(out string) []BlameLine {
	var (
		lines   []BlameLine
		current BlameLine
		inEntry bool
	)

	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSuffix(line, "\r")

		// The content line is the only one starting with a tab, and it
		// terminates the record.
		if strings.HasPrefix(line, "\t") {
			if inEntry {
				current.Content = line[1:]
				lines = append(lines, current)
				inEntry = false
			}
			continue
		}

		if !inEntry {
			// A header is "<40-hex> <n> <n>" possibly with a fourth
			// field. Anything else at this point is not a record start.
			fields := strings.Fields(line)
			if len(fields) < 3 || len(fields[0]) != 40 {
				continue
			}
			finalLine, err := strconv.Atoi(fields[2])
			if err != nil {
				continue
			}
			current = BlameLine{
				Line:  finalLine,
				Hash:  fields[0],
				Short: fields[0][:8],
			}
			inEntry = true
			continue
		}

		key, value, found := strings.Cut(line, " ")
		if !found {
			continue
		}
		switch key {
		case "author":
			current.Author = value
		case "author-time":
			if sec, err := strconv.ParseInt(value, 10, 64); err == nil {
				current.Date = time.Unix(sec, 0)
			}
		case "summary":
			current.Summary = value
		}
	}
	return lines
}

// Spans collapses consecutive lines sharing a commit into blocks.
func Spans(lines []BlameLine) []BlameSpan {
	var spans []BlameSpan
	for _, l := range lines {
		if n := len(spans); n > 0 && spans[n-1].Hash == l.Hash && spans[n-1].End == l.Line-1 {
			spans[n-1].End = l.Line
			continue
		}
		spans = append(spans, BlameSpan{
			Start:   l.Line,
			End:     l.Line,
			Hash:    l.Hash,
			Short:   l.Short,
			Author:  l.Author,
			Date:    l.Date,
			Summary: l.Summary,
		})
	}
	return spans
}

// AuthorStat summarises one author's share of a file.
type AuthorStat struct {
	Author string
	Lines  int
	// Latest is the most recent commit date among their lines, which is
	// what says whether they are still the person to ask.
	Latest time.Time
}

// Authors summarises who wrote a file, most lines first.
func Authors(lines []BlameLine) []AuthorStat {
	byAuthor := map[string]*AuthorStat{}
	for _, l := range lines {
		stat, ok := byAuthor[l.Author]
		if !ok {
			stat = &AuthorStat{Author: l.Author}
			byAuthor[l.Author] = stat
		}
		stat.Lines++
		if l.Date.After(stat.Latest) {
			stat.Latest = l.Date
		}
	}

	out := make([]AuthorStat, 0, len(byAuthor))
	for _, stat := range byAuthor {
		out = append(out, *stat)
	}
	// Most lines first, then most recent, so the top entry is the person
	// with both the most context and the freshest memory.
	sortAuthorStats(out)
	return out
}

func sortAuthorStats(stats []AuthorStat) {
	for i := 1; i < len(stats); i++ {
		for j := i; j > 0; j-- {
			a, b := stats[j-1], stats[j]
			if a.Lines > b.Lines || (a.Lines == b.Lines && !a.Latest.Before(b.Latest)) {
				break
			}
			stats[j-1], stats[j] = stats[j], stats[j-1]
		}
	}
}
