package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/weikxs/gitpersona/internal/generate"
	"github.com/weikxs/gitpersona/internal/paths"
)

// VerifyCache keeps the last repository checks of the configuration page,
// so opening the overview again does not start a few git processes per
// repository when nothing changed. An answer is reused while the files it
// read are unchanged (each repository's .git/config, the global Git
// configuration and GitPersona's own files) and for at most TTL, since
// gh's accounts can change too. The page drops it after every change.
type VerifyCache struct {
	TTL time.Duration
	Now func() time.Time

	mu      sync.Mutex
	entries map[string]verifyEntry
}

type verifyEntry struct {
	fingerprint string
	at          time.Time
	report      VerifyReport
}

// Verify checks the repositories in the profiles' folders and extra, at
// most limit of them (0: all), or returns the same check's earlier answer.
func (c *VerifyCache) Verify(ctx context.Context, e Env, extra []string, limit int) (VerifyReport, error) {
	t, err := e.verifyTargets(ctx, extra)
	if err != nil {
		return VerifyReport{}, err
	}
	key := fmt.Sprintf("%s\x00%d\x00%s", e.lang(), limit, strings.Join(extra, "\x00"))
	fingerprint := e.verifyFingerprint(t)
	c.mu.Lock()
	entry, ok := c.entries[key]
	c.mu.Unlock()
	if ok && entry.fingerprint == fingerprint && c.Now().Sub(entry.at) < c.TTL {
		return entry.report, nil
	}
	report := e.checkTargets(ctx, t, false, limit)
	if ctx.Err() == nil {
		c.mu.Lock()
		if c.entries == nil {
			c.entries = map[string]verifyEntry{}
		}
		c.entries[key] = verifyEntry{fingerprint: fingerprint, at: c.Now(), report: report}
		c.mu.Unlock()
	}
	return report, nil
}

// Forget drops every kept answer.
func (c *VerifyCache) Forget() {
	c.mu.Lock()
	c.entries = nil
	c.mu.Unlock()
}

// verifyFingerprint describes, by size and modification time, every file
// whose content a check of t depends on, and which repositories it found.
func (e Env) verifyFingerprint(t verifyTargets) string {
	files := []string{e.configFile(), filepath.Join(e.ConfigDir, generate.RulesFile)}
	profiles, _ := filepath.Glob(filepath.Join(e.ConfigDir, "profiles", "*.gitconfig"))
	files = append(append(files, profiles...), e.globalFiles()...)
	for _, r := range t.found.Repos {
		dir := r.Dir
		if norm, err := paths.Normalize(dir, e.Home); err == nil {
			dir = norm
		}
		files = append(files, filepath.Join(dir, ".git"), filepath.Join(dir, ".git", "config"))
	}
	var b strings.Builder
	for _, f := range files {
		b.WriteString(f)
		if info, err := os.Stat(f); err == nil {
			fmt.Fprintf(&b, "\x00%d\x00%d", info.Size(), info.ModTime().UnixNano())
		}
		b.WriteByte('\n')
	}
	return b.String()
}
