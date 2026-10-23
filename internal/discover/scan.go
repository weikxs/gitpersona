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
var DefaultScanLimits = ScanLimits{MaxDepth: 4, Timeout: 10 * time.Second}

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
	repos, _ := Repos(ctx, roots, limits)
	for _, r := range repos {
		if r.Lone || !strings.EqualFold(r.Host, "github.com") || r.Owner == "" {
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

// maxNesting bounds how deep repositories inside repositories are looked for.
const maxNesting = 3

// walkRepos calls found for every repository under root, not descending into
// skipped or hidden directories, or below maxDepth. Inside a repository it
// only looks at the folders directly in it, for repositories kept there;
// those, and root when it is a repository itself, are found as lone.
func walkRepos(ctx context.Context, root string, maxDepth int, found func(dir string, lone bool)) {
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
		if isRepo(path) {
			found(path, path == root)
			nestedRepos(ctx, path, maxNesting, found)
			return filepath.SkipDir
		}
		if strings.Count(path, string(filepath.Separator))-base >= maxDepth {
			return filepath.SkipDir
		}
		return nil
	})
}

// nestedRepos finds repositories kept directly inside the repository dir,
// and inside those, up to depth levels.
func nestedRepos(ctx context.Context, dir string, depth int, found func(string, bool)) {
	if depth == 0 || ctx.Err() != nil {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || strings.HasPrefix(name, ".") || skippedDirs[name] {
			continue
		}
		if child := filepath.Join(dir, name); isRepo(child) {
			found(child, true)
			nestedRepos(ctx, child, depth-1, found)
		}
	}
}

// isRepo reports whether dir is a repository with its own .git directory
// (submodules and worktrees, whose .git is a file, follow their parent).
func isRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git", "config"))
	return err == nil
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
