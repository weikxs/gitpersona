package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
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
// keyring) are never kept, and neither are failures.
type CachingRunner struct {
	Next Runner
	TTL  time.Duration
	Now  func() time.Time
	// Dir, when set, keeps the answers on disk too, so every gitpersona
	// process (each doctor, verify and page) shares them for TTL instead of
	// asking the host again. The files hold gh's answers to the queries above:
	// account names and states, public profiles, never a token.
	Dir string
	// Stamp fingerprints gh's own account file: an answer saved under another
	// stamp (gh auth login, logout or switch since) is not reused.
	Stamp func() string

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

	stamp := c.stamp()
	if out, at, ok := c.load(key, stamp); ok {
		e.out, e.at = out, at
	} else {
		e.out, e.err = c.Next(ctx, name, args...)
		e.at = c.Now()
		if e.err == nil && ctx.Err() == nil {
			c.save(key, stamp, e.out, e.at)
		}
	}
	if e.err != nil || ctx.Err() != nil {
		// A failure (gh signed out for a moment, no network) or a cancelled
		// call says nothing lasting about the account: the next caller asks
		// again. Callers already waiting share this answer.
		c.mu.Lock()
		if c.entries[key] == e {
			delete(c.entries, key)
		}
		c.mu.Unlock()
	}
	close(e.done)
	return slices.Clone(e.out), e.err
}

// Forget drops every remembered answer, on disk too, so the next query asks
// the host.
func (c *CachingRunner) Forget() {
	c.mu.Lock()
	c.entries = nil
	c.mu.Unlock()
	if c.Dir == "" {
		return
	}
	files, _ := filepath.Glob(filepath.Join(c.Dir, cacheFilePrefix+"*"))
	for _, f := range files {
		_ = os.Remove(f) // a file left behind is only an answer asked again later
	}
}

// cacheFilePrefix names the files a CachingRunner keeps in Dir.
const cacheFilePrefix = "gh-"

// savedAnswer is one answer as kept on disk.
type savedAnswer struct {
	Stamp string    `json:"stamp"`
	At    time.Time `json:"at"`
	Out   []byte    `json:"out"`
}

func (c *CachingRunner) stamp() string {
	if c.Stamp == nil {
		return ""
	}
	return c.Stamp()
}

func (c *CachingRunner) file(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(c.Dir, cacheFilePrefix+hex.EncodeToString(sum[:16])+".json")
}

// load returns the answer saved for key under stamp, if it is younger than
// TTL. An answer dated in the future (the clock went back) is not trusted.
func (c *CachingRunner) load(key, stamp string) ([]byte, time.Time, bool) {
	if c.Dir == "" {
		return nil, time.Time{}, false
	}
	data, err := os.ReadFile(c.file(key))
	if err != nil {
		return nil, time.Time{}, false
	}
	var saved savedAnswer
	if json.Unmarshal(data, &saved) != nil || saved.Stamp != stamp {
		return nil, time.Time{}, false
	}
	age := c.Now().Sub(saved.At)
	if age < 0 || age >= c.TTL {
		return nil, time.Time{}, false
	}
	return saved.Out, saved.At, true
}

// save keeps an answer for other processes. The cache is an optimisation:
// a directory that cannot be written only means the next process asks again.
func (c *CachingRunner) save(key, stamp string, out []byte, at time.Time) {
	if c.Dir == "" {
		return
	}
	data, err := json.Marshal(savedAnswer{Stamp: stamp, At: at, Out: out})
	if err != nil || os.MkdirAll(c.Dir, 0o700) != nil {
		return
	}
	tmp, err := os.CreateTemp(c.Dir, ".tmp-*") // created 0600
	if err != nil {
		return
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), c.file(key)) != nil {
		_ = os.Remove(tmp.Name())
	}
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
