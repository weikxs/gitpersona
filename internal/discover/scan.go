package discover

import (
	"cmp"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
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

var remoteURLPattern = regexp.MustCompile(`(?m)^\s*url\s*=\s*(.+?)\s*$`)

// Scan walks roots looking for repositories, reads only the remote URLs in
// their .git/config, and suggests a rule for each parent directory whose
// repositories on github.com mostly have one owner.
func Scan(ctx context.Context, roots, accounts []string, limits ScanLimits) []Suggestion {
	ctx, cancel := context.WithTimeout(ctx, limits.Timeout)
	defer cancel()

	type tally struct {
		total   int
		byOwner map[string]int
	}
	parents := map[string]*tally{}
	for _, root := range roots {
		walkRepos(ctx, root, limits.MaxDepth, func(repo string) {
			owner := repoOwner(repo)
			if owner == "" {
				return
			}
			parent := filepath.Dir(repo)
			t := parents[parent]
			if t == nil {
				t = &tally{byOwner: map[string]int{}}
				parents[parent] = t
			}
			t.total++
			t.byOwner[strings.ToLower(owner)]++
		})
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
			found(path)
			return filepath.SkipDir
		}
		if strings.Count(path, string(filepath.Separator))-base >= maxDepth {
			return filepath.SkipDir
		}
		return nil
	})
}

// repoOwner returns the github.com owner of the repository's first remote.
func repoOwner(repo string) string {
	data, err := os.ReadFile(filepath.Join(repo, ".git", "config"))
	if err != nil {
		return ""
	}
	m := remoteURLPattern.FindSubmatch(data)
	if m == nil {
		return ""
	}
	return remoteOwner(string(m[1]), "github.com")
}

// remoteOwner extracts the owner from a remote URL on host, or "".
func remoteOwner(url, host string) string {
	var path string
	switch {
	case strings.Contains(url, "://"):
		rest := url[strings.Index(url, "://")+3:]
		at := strings.LastIndex(rest, "@")
		slash := strings.Index(rest, "/")
		if at >= 0 && (slash < 0 || at < slash) {
			rest = rest[at+1:]
		}
		h, p, ok := strings.Cut(rest, "/")
		if !ok || !strings.EqualFold(h, host) {
			return ""
		}
		path = p
	case strings.Contains(url, ":"):
		h, p, _ := strings.Cut(url, ":")
		if _, after, ok := strings.Cut(h, "@"); ok {
			h = after
		}
		if !strings.EqualFold(h, host) {
			return ""
		}
		path = p
	default:
		return ""
	}
	owner, _, _ := strings.Cut(strings.TrimPrefix(path, "/"), "/")
	return owner
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
