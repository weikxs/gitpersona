package discover

import (
	"cmp"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// ScanLimits bounds a scan so it stays fast on large disks.
type ScanLimits struct {
	MaxDepth int
	Timeout  time.Duration
}

// DefaultScanLimits suit a home directory's usual code folders.
var DefaultScanLimits = ScanLimits{MaxDepth: 4, Timeout: 5 * time.Second}

// Suggestion proposes a directory rule: most repositories directly under Dir
// belong to Owner. Account is the signed-in account that is Owner, or "" when
// Owner is someone else, such as an organization, and the user must choose.
type Suggestion struct {
	Dir     string `json:"dir"`
	Account string `json:"account"`
	Owner   string `json:"owner"`
	Repos   int    `json:"repos"` // repositories of Owner under Dir
	Total   int    `json:"total"` // repositories under Dir with a remote on the host
}

// skippedDirs are never descended into.
var skippedDirs = map[string]bool{"node_modules": true, "vendor": true, ".cache": true, "Library": true}

// Scan finds the repositories under roots (see Repos) and suggests a rule for each parent directory whose
// repositories on github.com mostly have one owner.
func Scan(ctx context.Context, roots, accounts []string, limits ScanLimits) []Suggestion {

	type tally struct {
		total   int
		byOwner map[string]int
	}
	parents := map[string]*tally{}
	for _, r := range Repos(ctx, roots, limits) {
		if !strings.EqualFold(r.Host, "github.com") || r.Owner == "" {
			continue
		}
		parent := filepath.Dir(r.Dir)
		t := parents[parent]
		if t == nil {
			t = &tally{byOwner: map[string]int{}}
			parents[parent] = t
		}
		t.total++
		t.byOwner[strings.ToLower(r.Owner)]++
	}

	var suggestions []Suggestion
	for dir, t := range parents {
		best, count := "", 0
		for owner, n := range t.byOwner {
			if n > count || (n == count && owner < best) {
				best, count = owner, n
			}
		}
		// Suggest only when one owner has most repositories there.
		if count*2 > t.total {
			suggestions = append(suggestions, Suggestion{Dir: dir, Account: accountFor(accounts, best), Owner: best, Repos: count, Total: t.total})
		}
	}
	slices.SortFunc(suggestions, func(a, b Suggestion) int { return cmp.Compare(a.Dir, b.Dir) })
	return suggestions
}

// walkRepos calls found for every repository under root, not descending into
// repositories, skipped or hidden directories, or below maxDepth.
func walkRepos(ctx context.Context, root string, maxDepth int, found func(string)) {
	base := strings.Count(filepath.Clean(root), string(filepath.Separator))
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return filepath.SkipAll
		}
		if err != nil || !d.IsDir() {
			return nil
		}
		name := d.Name()
		if path != root && (strings.HasPrefix(name, ".") || skippedDirs[name]) {
			return filepath.SkipDir
		}
		if _, err := os.Stat(filepath.Join(path, ".git", "config")); err == nil {
			if path == root {
				return filepath.SkipAll // a folder to scan that is itself one repository says nothing about folders
			}
			found(path)
			return filepath.SkipDir
		}
		if strings.Count(path, string(filepath.Separator))-base >= maxDepth {
			return filepath.SkipDir
		}
		return nil
	})
}

// accountFor returns the account (as gh spells it) whose login is owner.
func accountFor(accounts []string, owner string) string {
	for _, a := range accounts {
		if strings.EqualFold(a, owner) {
			return a
		}
	}
	return ""
}
