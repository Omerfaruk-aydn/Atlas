package tui

import (
	"os/exec"
	"sync"
	"time"
)

// GitBranchCache is the dedicated git-branch hook with TTL +
// in-flight dedup. Mirrors Hermes's useGitBranch (15s TTL, 500ms
// timeout, inflight dedup map so concurrent callers share one
// subprocess per cwd).
type GitBranchCache struct {
	mu        sync.Mutex
	cache     map[string]gitBranchEntry
	inflight  map[string]*gitBranchInflight
	ttl       time.Duration
	timeout   time.Duration
	clock     func() time.Time
	execFn    func(cwd string) (string, error)
}

type gitBranchEntry struct {
	branch string
	at     time.Time
}

type gitBranchInflight struct {
	done chan struct{}
	branch string
	err  error
}

const (
	gitBranchTTL     = 15 * time.Second
	gitBranchTimeout = 500 * time.Millisecond
)

// newGitBranchCache returns a cache with the default TTL + timeout.
func newGitBranchCache() *GitBranchCache {
	return &GitBranchCache{
		cache:    make(map[string]gitBranchEntry),
		inflight: make(map[string]*gitBranchInflight),
		ttl:      gitBranchTTL,
		timeout:  gitBranchTimeout,
		clock:    time.Now,
		execFn:   defaultGitBranchExec,
	}
}

// Get returns the current branch for cwd, fetching on miss.
// Concurrent calls for the same cwd share the same subprocess
// (the inflight map is the dedup key).
func (g *GitBranchCache) Get(cwd string) (string, error) {
	g.mu.Lock()
	// Fast path: cache hit within TTL.
	if e, ok := g.cache[cwd]; ok && g.clock().Sub(e.at) < g.ttl {
		g.mu.Unlock()
		return e.branch, nil
	}
	// Dedup: someone else is already fetching this cwd.
	if inflight, ok := g.inflight[cwd]; ok {
		g.mu.Unlock()
		select {
		case <-inflight.done:
			return inflight.branch, inflight.err
		case <-time.After(g.timeout):
			// Timed out waiting on the inflight fetch. Return
			// what we have (possibly ""), don't block forever.
			return inflight.branch, inflight.err
		}
	}
	// We're the first; kick off the fetch.
	inf := &gitBranchInflight{done: make(chan struct{})}
	g.inflight[cwd] = inf
	g.mu.Unlock()

	branch, err := g.execFn(cwd)
	g.mu.Lock()
	inf.branch = branch
	inf.err = err
	if err == nil {
		g.cache[cwd] = gitBranchEntry{branch: branch, at: g.clock()}
	}
	delete(g.inflight, cwd)
	close(inf.done)
	g.mu.Unlock()
	return branch, err
}

// Invalidate drops the cached entry for cwd (used when the user
// checks out a different branch and we want the next Get to re-fetch).
func (g *GitBranchCache) Invalidate(cwd string) {
	g.mu.Lock()
	delete(g.cache, cwd)
	g.mu.Unlock()
}

// defaultGitBranchExec is the actual subprocess invocation. Treats
// empty / "HEAD" output as "no branch" (detached HEAD).
func defaultGitBranchExec(cwd string) (string, error) {
	ctx, cancel := newTimeoutContext(gitBranchTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", cwd, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", err
	}
	branch := trimNewline(string(out))
	if branch == "" || branch == "HEAD" {
		return "", nil
	}
	return branch, nil
}

// trimNewline strips a single trailing newline; used for shell
// output that comes back with a "\n" suffix.
func trimNewline(s string) string {
	if len(s) > 0 && s[len(s)-1] == '\n' {
		return s[:len(s)-1]
	}
	return s
}
