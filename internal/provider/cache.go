package provider

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// CachingRunner remembers, for TTL, the answers of the gh queries that reach
// the host: `gh auth status` checks every signed-in account against the API,
// and `gh api` reads a profile. Asking again on every page refresh, every
// doctor check and every repository would send a burst of requests from each
// account in turn, which a host can read as automated activity. Every other
// command runs each time; tokens (`gh auth token`, read from the local
// keyring) are never kept.
type CachingRunner struct {
	Next Runner
	TTL  time.Duration
	Now  func() time.Time

	mu      sync.Mutex
	entries map[string]*cacheEntry
}

type cacheEntry struct {
	done chan struct{} // closed once out and err are set
	at   time.Time
	out  []byte
	err  error
}

// Run runs the command through Next, or returns the answer it gave less than
// TTL ago. Callers asking while the same query runs wait for its answer.
func (c *CachingRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if !cacheable(name, args) {
		return c.Next(ctx, name, args...)
	}
	key := cacheKey(name, args)
	c.mu.Lock()
	if e, ok := c.entries[key]; ok && c.fresh(e) {
		c.mu.Unlock()
		<-e.done
		return slices.Clone(e.out), e.err
	}
	e := &cacheEntry{done: make(chan struct{})}
	if c.entries == nil {
		c.entries = map[string]*cacheEntry{}
	}
	c.entries[key] = e
	c.mu.Unlock()

	e.out, e.err = c.Next(ctx, name, args...)
	e.at = c.Now()
	if ctx.Err() != nil {
		// A cancelled or timed-out call says nothing about the account.
		c.mu.Lock()
		if c.entries[key] == e {
			delete(c.entries, key)
		}
		c.mu.Unlock()
	}
	close(e.done)
	return slices.Clone(e.out), e.err
}

// Forget drops every remembered answer, so the next query asks the host.
func (c *CachingRunner) Forget() {
	c.mu.Lock()
	c.entries = nil
	c.mu.Unlock()
}

// fresh reports whether e is still running or younger than TTL; c.mu is held.
func (c *CachingRunner) fresh(e *cacheEntry) bool {
	select {
	case <-e.done:
		return c.Now().Sub(e.at) < c.TTL
	default:
		return true
	}
}

// cacheable reports whether the command is a gh query that reaches the host
// and holds no secret: `gh auth status` and a `gh api` read.
func cacheable(name string, args []string) bool {
	base := strings.TrimSuffix(filepath.Base(name), ".exe")
	if base != "gh" || len(args) == 0 {
		return false
	}
	switch args[0] {
	case "auth":
		return len(args) > 1 && args[1] == "status" && !slices.Contains(args, "--show-token") && !slices.Contains(args, "-t")
	case "api":
		return !slices.ContainsFunc(args, func(a string) bool {
			return a == "-X" || a == "--method" || strings.HasPrefix(a, "--method=") || a == "-f" || a == "-F" || a == "--input"
		})
	}
	return false
}

func cacheKey(name string, args []string) string {
	return name + "\x00" + strings.Join(args, "\x00")
}
