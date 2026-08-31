package model

import (
	"bufio"
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// gitStatusTTL bounds how long the memoized git status may go without a
// re-probe. File-save events normally refresh it much sooner; the backstop
// covers repos changed from outside the running session (another terminal,
// a background build).
var gitStatusTTL = 10 * time.Second

// gitStatus is the memoized state shown in the sidebar's path row.
type gitStatus struct {
	branch  string
	ahead   int
	behind  int
	changed int
	isRepo  bool
}

// gitStatusMsg delivers a git status probe fetched off-thread.
type gitStatusMsg gitStatus

// requestGitStatusRefresh schedules an off-thread git status probe. While a
// probe is already in flight it only marks the state dirty; applyGitStatus
// re-dispatches so the freshest result still lands.
func (m *UI) requestGitStatusRefresh() tea.Cmd {
	if m.gitFetchInFlight {
		m.gitRefreshQueued = true
		return nil
	}
	return m.dispatchGitStatusRefresh()
}

// dispatchGitStatusRefresh returns a command that shells out to git off the
// Update goroutine. The closure captures only the working directory (never
// m) so it is safe to run off-thread.
func (m *UI) dispatchGitStatusRefresh() tea.Cmd {
	if m.gitFetchInFlight || m.com == nil || m.com.Workspace == nil {
		return nil
	}
	m.gitFetchInFlight = true
	m.gitCheckedAt = time.Now()
	ws := m.com.Workspace
	return func() tea.Msg {
		return gitStatusMsg(probeGitStatus(ws.WorkingDir()))
	}
}

// applyGitStatus stores an off-thread git status probe and re-dispatches
// when a refresh was requested while it was in flight.
func (m *UI) applyGitStatus(msg gitStatusMsg) tea.Cmd {
	m.gitFetchInFlight = false
	m.gitCheckedAt = time.Now()
	m.gitStatus = gitStatus(msg)
	if m.gitRefreshQueued {
		m.gitRefreshQueued = false
		return m.dispatchGitStatusRefresh()
	}
	return nil
}

// probeGitStatus shells out to git for the branch name, ahead/behind count,
// and number of changed files, using a single `status --porcelain=v1
// --branch` call. Any failure (not a repo, git not installed, timeout) is
// reported as isRepo: false rather than an error — the sidebar simply omits
// git info in that case.
func probeGitStatus(workDir string) gitStatus {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain=v1", "--branch")
	cmd.Dir = workDir
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return gitStatus{}
	}

	var st gitStatus
	st.isRepo = true
	scanner := bufio.NewScanner(&out)
	for scanner.Scan() {
		line := scanner.Text()
		if branch, ahead, behind, ok := parseGitBranchLine(line); ok {
			st.branch, st.ahead, st.behind = branch, ahead, behind
			continue
		}
		if line != "" {
			st.changed++
		}
	}
	return st
}

// parseGitBranchLine parses the "## branch...tracking [ahead N, behind M]"
// header line from `git status --porcelain=v1 --branch`. Detached HEAD
// (e.g. "## HEAD (no branch)") and a branch with no upstream both parse to
// just the branch name with ahead/behind at 0.
func parseGitBranchLine(line string) (branch string, ahead, behind int, ok bool) {
	rest, ok := strings.CutPrefix(line, "## ")
	if !ok {
		return "", 0, 0, false
	}
	branch = rest
	if i := strings.Index(rest, "..."); i >= 0 {
		branch = rest[:i]
	} else if i := strings.IndexByte(rest, ' '); i >= 0 {
		branch = rest[:i]
	}
	if open := strings.IndexByte(rest, '['); open >= 0 {
		if close := strings.IndexByte(rest[open:], ']'); close >= 0 {
			for _, part := range strings.Split(rest[open+1:open+close], ", ") {
				part = strings.TrimSpace(part)
				switch {
				case strings.HasPrefix(part, "ahead "):
					ahead = parseGitInt(strings.TrimPrefix(part, "ahead "))
				case strings.HasPrefix(part, "behind "):
					behind = parseGitInt(strings.TrimPrefix(part, "behind "))
				}
			}
		}
	}
	return branch, ahead, behind, true
}

func parseGitInt(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}
