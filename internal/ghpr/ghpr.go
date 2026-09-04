// Package ghpr reads a GitHub pull request's metadata and diff through
// the gh CLI -- the same tool this project's own contributing workflow
// already relies on for PR operations, rather than a hand-rolled HTTP
// client against the GitHub API. gh already solves authentication
// (its own stored credentials, GITHUB_TOKEN, SSH-based auth) and knows
// how to find the right repository from the current directory; a second
// implementation of that would only be a second place for it to be
// wrong.
//
// Everything here is read-only: it shells out to `gh pr view` and
// `gh pr diff`, never to anything that could create, comment on, merge,
// or close a pull request.
package ghpr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// PR is one pull request's metadata and diff.
type PR struct {
	Number       int
	Title        string
	Body         string
	Author       string
	State        string
	BaseRefName  string
	HeadRefName  string
	URL          string
	Additions    int
	Deletions    int
	ChangedFiles int
	// Diff is the unified diff, empty if `gh pr diff` failed even though
	// the metadata lookup succeeded -- a PR too large to diff still has
	// metadata worth reporting.
	Diff string
}

// ErrGHMissing is returned when the gh binary is not on PATH.
var ErrGHMissing = errors.New("gh is not installed or not on PATH")

// ErrNotAuthenticated is returned when gh is installed but has no
// credentials for the request.
var ErrNotAuthenticated = errors.New("gh is not authenticated; run `gh auth login`")

// ErrBadRef is returned when the given reference isn't a recognisable
// pull request number, "owner/repo#number", or GitHub PR URL.
var ErrBadRef = errors.New("not a recognised pull request reference")

// Runner executes gh with the given arguments in dir and returns its
// stdout. It exists as a seam so the argument-building and JSON-parsing
// logic can be tested without gh installed; Client's production use
// (New) is backed by the real binary.
type Runner func(ctx context.Context, dir string, args ...string) (string, error)

// Client fetches pull requests.
type Client struct {
	run Runner
}

// New returns a Client backed by the real gh binary.
func New() *Client {
	return &Client{run: runGH}
}

// View fetches ref's metadata and diff. ref may be a bare number ("42"
// or "#42", resolved against dir's repository the way `gh pr view` does
// on its own), "owner/repo#42", or a full
// https://github.com/owner/repo/pull/42 URL.
func (c *Client) View(ctx context.Context, dir, ref string) (PR, error) {
	owner, repo, number, err := parseRef(ref)
	if err != nil {
		return PR{}, err
	}

	viewArgs := []string{"pr", "view"}
	if number != "" {
		viewArgs = append(viewArgs, number)
	}
	if owner != "" {
		viewArgs = append(viewArgs, "-R", owner+"/"+repo)
	}
	viewArgs = append(viewArgs, "--json",
		"number,title,body,author,state,baseRefName,headRefName,url,additions,deletions,changedFiles")

	out, err := c.run(ctx, dir, append([]string{"gh"}, viewArgs...)...)
	if err != nil {
		return PR{}, err
	}

	var raw struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
		State        string `json:"state"`
		BaseRefName  string `json:"baseRefName"`
		HeadRefName  string `json:"headRefName"`
		URL          string `json:"url"`
		Additions    int    `json:"additions"`
		Deletions    int    `json:"deletions"`
		ChangedFiles int    `json:"changedFiles"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return PR{}, fmt.Errorf("parsing gh pr view output: %w", err)
	}

	pr := PR{
		Number: raw.Number, Title: raw.Title, Body: raw.Body,
		Author: raw.Author.Login, State: raw.State,
		BaseRefName: raw.BaseRefName, HeadRefName: raw.HeadRefName, URL: raw.URL,
		Additions: raw.Additions, Deletions: raw.Deletions, ChangedFiles: raw.ChangedFiles,
	}

	diffArgs := []string{"pr", "diff", strconv.Itoa(pr.Number)}
	if owner != "" {
		diffArgs = append(diffArgs, "-R", owner+"/"+repo)
	}
	if diff, err := c.run(ctx, dir, append([]string{"gh"}, diffArgs...)...); err == nil {
		pr.Diff = diff
	}

	return pr, nil
}

var (
	prURLPattern      = regexp.MustCompile(`^https?://github\.com/([^/\s]+)/([^/\s]+)/pull/(\d+)`)
	ownerRepoPattern  = regexp.MustCompile(`^([^/\s#]+)/([^/\s#]+)#(\d+)$`)
	bareNumberPattern = regexp.MustCompile(`^#?(\d+)$`)
)

// parseRef reads a PR reference into its parts. owner and repo are both
// empty together -- a bare number relies on gh's own detection of the
// repository from the current directory's git remote.
func parseRef(ref string) (owner, repo, number string, err error) {
	ref = strings.TrimSpace(ref)
	switch {
	case prURLPattern.MatchString(ref):
		m := prURLPattern.FindStringSubmatch(ref)
		return m[1], strings.TrimSuffix(m[2], ".git"), m[3], nil
	case ownerRepoPattern.MatchString(ref):
		m := ownerRepoPattern.FindStringSubmatch(ref)
		return m[1], m[2], m[3], nil
	case bareNumberPattern.MatchString(ref):
		m := bareNumberPattern.FindStringSubmatch(ref)
		return "", "", m[1], nil
	default:
		return "", "", "", fmt.Errorf("%w: %q", ErrBadRef, ref)
	}
}

// runGH is the production Runner: it shells out to the real gh binary.
func runGH(ctx context.Context, dir string, args ...string) (string, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return "", ErrGHMissing
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "not logged") || strings.Contains(lower, "auth login") || strings.Contains(lower, "gh auth login") {
			return "", ErrNotAuthenticated
		}
		if msg == "" {
			return "", fmt.Errorf("gh %s: %w", strings.Join(args[1:], " "), err)
		}
		return "", fmt.Errorf("gh %s: %s", strings.Join(args[1:], " "), firstLine(msg))
	}
	return stdout.String(), nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
