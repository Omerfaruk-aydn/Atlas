// Package logtail reads the tail of a log file with an optional
// substring or log-level filter, without loading the whole file into
// memory.
package logtail

import (
	"bufio"
	"container/ring"
	"os"
	"regexp"
	"strings"
)

// Options narrows a tail read.
type Options struct {
	// Lines is how many trailing lines to return. Zero means 100.
	Lines int
	// Grep restricts the result to lines containing this substring,
	// case-insensitively. Applied before Lines truncates, so a filtered
	// tail still returns the requested count of matching lines rather
	// than fewer.
	Grep string
	// Level restricts to lines that look like they carry this log
	// level -- "error", "warn", "info", "debug" -- matched as a whole
	// word, case-insensitively. Common enough spellings (ERROR, [ERROR],
	// level=error, "level":"error") are all recognised.
	Level string
}

// Result is the outcome of a tail read.
type Result struct {
	Lines []string
	// TotalLines is how many lines the file has, before Grep/Level
	// filtering -- useful to know whether a filter matched nothing
	// because the file is short or because the pattern doesn't occur.
	TotalLines int
	// Truncated reports that more matching lines existed than Lines
	// could hold.
	Truncated bool
}

const defaultLines = 100

// Tail reads path and returns its last N lines, optionally filtered.
func Tail(path string, opts Options) (Result, error) {
	limit := opts.Lines
	if limit <= 0 {
		limit = defaultLines
	}

	f, err := os.Open(path)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()

	levelPattern := levelRegexp(opts.Level)
	grepLower := strings.ToLower(opts.Grep)

	buf := ring.New(limit)
	total, matched := 0, 0

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		total++

		if grepLower != "" && !strings.Contains(strings.ToLower(line), grepLower) {
			continue
		}
		if levelPattern != nil && !levelPattern.MatchString(line) {
			continue
		}
		matched++
		buf.Value = line
		buf = buf.Next()
	}
	if err := scanner.Err(); err != nil {
		return Result{}, err
	}

	var lines []string
	buf.Do(func(v any) {
		if v != nil {
			lines = append(lines, v.(string))
		}
	})

	return Result{
		Lines:      lines,
		TotalLines: total,
		Truncated:  matched > limit,
	}, nil
}

// levelRegexp builds a case-insensitive whole-word matcher for a log
// level, accepting the common ways one shows up in a log line:
// "ERROR", "[error]", "level=error", and "\"level\":\"error\"" all match.
func levelRegexp(level string) *regexp.Regexp {
	level = strings.TrimSpace(level)
	if level == "" {
		return nil
	}
	return regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(level) + `\b`)
}
