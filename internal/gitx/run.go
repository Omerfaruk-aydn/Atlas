// Package gitx runs git and parses its machine-readable output.
//
// It shells out to the git binary rather than using a pure-Go
// implementation. That is deliberate: git's own porcelain formats are
// stable by contract, and the binary is the only thing that gets
// worktrees, submodules, sparse checkouts, alternate object stores,
// includeIf config and hooks right. A reimplementation gets the common
// case right and then quietly disagrees on somebody's real repository.
//
// Everything here is read-only. Commands that write are the caller's
// business and must go through the permission system.
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ErrNotARepository is returned when the given directory is not inside a
// git working tree. Callers surface it as guidance rather than failure:
// "this is not a repository" is an answer, not an error condition.
var ErrNotARepository = errors.New("not a git repository")

// ErrGitMissing is returned when no git binary is on PATH.
var ErrGitMissing = errors.New("git is not installed or not on PATH")

// maxOutputBytes bounds what a single git invocation may return. A log
// or a diff over a large repository can be hundreds of megabytes, and
// buffering that to answer a question about the last twenty commits is
// how a tool call turns into an out-of-memory kill.
const maxOutputBytes = 8 << 20 // 8 MiB

// Run executes git in dir with the given arguments and returns stdout.
//
// The environment is pinned so output does not depend on the user's
// config: no pager, no colour, English messages. Without that, parsing
// breaks on any machine with a locale set or a pager configured, which is
// the kind of failure that only ever shows up on someone else's laptop.
func Run(ctx context.Context, dir string, args ...string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", ErrGitMissing
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(),
		"GIT_PAGER=cat",
		"GIT_TERMINAL_PROMPT=0", // Never block waiting for credentials.
		"LC_ALL=C",
		"LANG=C",
		"NO_COLOR=1",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	out := stdout.String()
	if len(out) > maxOutputBytes {
		out = out[:maxOutputBytes]
	}

	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "not a git repository") {
			return "", ErrNotARepository
		}
		if msg == "" {
			return out, fmt.Errorf("git %s: %w", args[0], err)
		}
		return out, fmt.Errorf("git %s: %s", args[0], firstLine(msg))
	}
	return out, nil
}

// IsRepository reports whether dir is inside a git working tree.
func IsRepository(ctx context.Context, dir string) bool {
	out, err := Run(ctx, dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// Root returns the absolute path of the working tree's top level.
func Root(ctx context.Context, dir string) (string, error) {
	out, err := Run(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// splitNul splits git's -z output, which terminates each record with a
// NUL rather than separating with one -- so the last element is empty and
// must be dropped, and a filename containing a newline survives intact.
func splitNul(s string) []string {
	parts := strings.Split(s, "\x00")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}
