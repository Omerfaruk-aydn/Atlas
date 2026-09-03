// Package lint runs a project's configured Go linter and parses its
// findings.
//
// It drives golangci-lint when it is installed and falls back to `go vet`
// when it is not. The fallback matters: vet ships with the toolchain, so
// there is always something useful to say rather than a "not installed"
// message that ends the conversation.
package lint

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Issue is one finding.
type Issue struct {
	File   string
	Line   int
	Column int
	// Linter names the check that produced it, e.g. "errcheck" or "vet".
	Linter  string
	Message string
	// Severity is the linter's own label when it gives one.
	Severity string
}

// Result is one lint run.
type Result struct {
	Issues []Issue
	// Tool names what actually ran, so a caller can tell a thorough
	// golangci-lint run from the narrower vet fallback.
	Tool string
	// Fallback reports that golangci-lint was unavailable and vet ran
	// instead, which finds far less.
	Fallback bool
	// Truncated reports that the issue list was cut short.
	Truncated bool
}

// Options select what to lint.
type Options struct {
	// Packages is a Go package pattern. Empty means "./...".
	Packages string
	// Timeout bounds the run. Zero means two minutes.
	Timeout time.Duration
	// MaxIssues caps how many findings are collected. Zero means 200.
	MaxIssues int
	// Linters, when non-empty, restricts golangci-lint to these checks.
	// Ignored by the vet fallback, which has no such flag.
	Linters []string
	// ForceVet skips golangci-lint even when it is installed.
	ForceVet bool
}

const (
	defaultTimeout   = 2 * time.Minute
	defaultMaxIssues = 200
)

// Run lints the tree.
//
// A non-zero exit status is how both tools report findings, so it is not
// an error here. An error is returned only when neither tool could run.
func Run(ctx context.Context, dir string, opts Options) (Result, error) {
	if opts.Packages == "" {
		opts.Packages = "./..."
	}
	if opts.Timeout <= 0 {
		opts.Timeout = defaultTimeout
	}
	if opts.MaxIssues <= 0 {
		opts.MaxIssues = defaultMaxIssues
	}

	if !opts.ForceVet {
		if path, err := exec.LookPath("golangci-lint"); err == nil {
			result, err := runGolangciLint(ctx, dir, path, opts)
			if err == nil {
				return result, nil
			}
			// A golangci-lint that is installed but cannot run -- a
			// config it rejects, a version mismatch -- should not end
			// the answer. Fall through to vet and say so.
		}
	}

	return runVet(ctx, dir, opts)
}

// golangciOutput is the subset of golangci-lint's JSON report we read.
type golangciOutput struct {
	Issues []struct {
		FromLinter string `json:"FromLinter"`
		Text       string `json:"Text"`
		Severity   string `json:"Severity"`
		Pos        struct {
			Filename string `json:"Filename"`
			Line     int    `json:"Line"`
			Column   int    `json:"Column"`
		} `json:"Pos"`
	} `json:"Issues"`
}

func runGolangciLint(ctx context.Context, dir, binary string, opts Options) (Result, error) {
	runCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	args := []string{"run", "--output.json.path=stdout", "--timeout", opts.Timeout.String()}
	if len(opts.Linters) > 0 {
		args = append(args, "--enable-only="+strings.Join(opts.Linters, ","))
	}
	args = append(args, opts.Packages)

	cmd := exec.CommandContext(runCtx, binary, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	// The JSON report is the contract; a non-zero exit just means
	// findings exist. Only a run that produced no parsable report at all
	// counts as a failure worth falling back from.
	//
	// A Decoder rather than Unmarshal, because golangci-lint follows the
	// JSON object on stdout with a human-readable tally ("1 issues:\n*
	// govet: 1"). Unmarshalling the whole buffer fails on that trailing
	// text with "invalid character after top-level value", which would
	// silently demote every real run to the vet fallback.
	var parsed golangciOutput
	if err := json.NewDecoder(bytes.NewReader(stdout.Bytes())).Decode(&parsed); err != nil {
		if runErr != nil {
			return Result{}, fmt.Errorf("golangci-lint: %s", firstLine(stderr.String()))
		}
		return Result{}, errors.New("golangci-lint produced no report")
	}

	result := Result{Tool: "golangci-lint"}
	for _, issue := range parsed.Issues {
		if len(result.Issues) >= opts.MaxIssues {
			result.Truncated = true
			break
		}
		result.Issues = append(result.Issues, Issue{
			File:     normalisePath(dir, issue.Pos.Filename),
			Line:     issue.Pos.Line,
			Column:   issue.Pos.Column,
			Linter:   issue.FromLinter,
			Message:  issue.Text,
			Severity: issue.Severity,
		})
	}
	sortIssues(result.Issues)
	return result, nil
}

// vetLine matches `go vet`'s "file:line:col: message" output. The column
// is optional -- vet omits it for some checks.
var vetLine = regexp.MustCompile(`^(.+?):(\d+):(?:(\d+):)?\s+(.*)$`)

func runVet(ctx context.Context, dir string, opts Options) (Result, error) {
	if _, err := exec.LookPath("go"); err != nil {
		return Result{}, errors.New("neither golangci-lint nor the go toolchain is on PATH, so nothing can be linted")
	}

	runCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "go", "vet", opts.Packages)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	_ = cmd.Run() // Findings are reported through exit status; not an error.

	result := Result{Tool: "go vet", Fallback: !opts.ForceVet}

	// vet writes its findings to stderr, and stdout stays empty.
	for line := range strings.SplitSeq(stderr.String(), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := vetLine.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		if len(result.Issues) >= opts.MaxIssues {
			result.Truncated = true
			break
		}
		lineNo, _ := strconv.Atoi(m[2])
		col, _ := strconv.Atoi(m[3])
		result.Issues = append(result.Issues, Issue{
			File:    normalisePath(dir, m[1]),
			Line:    lineNo,
			Column:  col,
			Linter:  "vet",
			Message: m[4],
		})
	}
	sortIssues(result.Issues)
	return result, nil
}

// normalisePath makes a finding's path relative to the linted directory,
// since golangci-lint and vet disagree about whether to print absolute or
// relative paths and a mixed report is hard to scan.
func normalisePath(dir, path string) string {
	if path == "" {
		return path
	}
	if !filepath.IsAbs(path) {
		return filepath.ToSlash(path)
	}
	if rel, err := filepath.Rel(dir, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

// sortIssues groups by file and then by position, so a reader fixing one
// file sees all of its findings together.
func sortIssues(issues []Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].File != issues[j].File {
			return issues[i].File < issues[j].File
		}
		if issues[i].Line != issues[j].Line {
			return issues[i].Line < issues[j].Line
		}
		return issues[i].Column < issues[j].Column
	})
}

// ByLinter counts findings per check, which is what says whether a report
// is one systemic problem or many separate ones.
func ByLinter(issues []Issue) map[string]int {
	out := map[string]int{}
	for _, issue := range issues {
		out[issue.Linter]++
	}
	return out
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	if s == "" {
		return "no output"
	}
	return s
}
