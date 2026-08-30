package tui

import (
	"sync"
	"time"
)

// ConfigSync is the App's background config-poller. The Hermes
// pattern is: every concern that needs the full config registers a
// wait; the first concern to ask kicks off the actual RPC and
// downstream consumers get the same promise back (so concurrent
// callers don't issue N parallel config fetches).
type ConfigSync struct {
	mu        sync.Mutex
	cached    map[string]string
	cachedAt  time.Time
	inflight  *configRequest
	clock     func() time.Time
	fetchFn   func() (map[string]string, error)
	ttl       time.Duration
}

type configRequest struct {
	done chan struct{}
	data map[string]string
	err  error
}

// newConfigSync returns a sync with the default 5s TTL.
func newConfigSync(fetch func() (map[string]string, error)) *ConfigSync {
	return &ConfigSync{
		cached:  nil,
		clock:   time.Now,
		fetchFn: fetch,
		ttl:     5 * time.Second,
	}
}

// Get returns the cached config; if the cache is stale or empty, it
// issues a fresh fetch and waits for it. The first caller in a stale
// window pays the latency; concurrent callers join the same wait
// channel.
func (c *ConfigSync) Get() (map[string]string, error) {
	c.mu.Lock()
	if c.cached != nil && c.clock().Sub(c.cachedAt) < c.ttl {
		out := c.cached
		c.mu.Unlock()
		return out, nil
	}
	if c.inflight != nil {
		req := c.inflight
		c.mu.Unlock()
		<-req.done
		return req.data, req.err
	}
	// We're the first; kick off the fetch.
	req := &configRequest{done: make(chan struct{})}
	c.inflight = req
	c.mu.Unlock()

	data, err := c.fetchFn()

	c.mu.Lock()
	req.data = data
	req.err = err
	c.cached = data
	c.cachedAt = c.clock()
	close(req.done)
	c.inflight = nil
	c.mu.Unlock()
	return data, err
}

// Invalidate drops the cached config so the next Get() refetches.
// Used when the user explicitly edits the config file at runtime.
func (c *ConfigSync) Invalidate() {
	c.mu.Lock()
	c.cached = nil
	c.mu.Unlock()
}
