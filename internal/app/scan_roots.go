package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/weikxs/gitpersona/internal/paths"
)

// DefaultScanRoots are the usual code folders scanned for repositories.
var DefaultScanRoots = []string{"~/Projects", "~/projects", "~/code", "~/src", "~/dev", "~/Developer", "~/work",
	"~/workspace", "~/repos", "~/git", "~/Documents/GitHub"}

// ScanRoots are folders to scan, resolved to their real paths so a
// symlinked ~/code is scanned too.
type ScanRoots struct {
	Shown   []string // the folders that exist, written with ~
	Missing []string // the folders that do not exist, written with ~
	Real    []string // the real paths to walk
	written map[string]string
}

// ResolveScanRoots resolves folders written as absolute paths or ~/...
func (e Env) ResolveScanRoots(roots []string) (ScanRoots, error) {
	out := ScanRoots{Shown: []string{}, Missing: []string{}, written: map[string]string{}}
	var seen []os.FileInfo
	for _, root := range roots {
		norm, err := paths.Normalize(strings.TrimSpace(root), e.Home)
		if err != nil {
			return ScanRoots{}, err
		}
		real, err := filepath.EvalSymlinks(norm)
		info, statErr := os.Stat(real)
		if err != nil || statErr != nil || !info.IsDir() {
			out.Missing = append(out.Missing, e.display(norm))
			continue
		}
		if slices.ContainsFunc(seen, func(other os.FileInfo) bool { return os.SameFile(info, other) }) {
			continue // ~/Projects and ~/projects are one folder on macOS
		}
		seen = append(seen, info)
		out.Shown = append(out.Shown, e.display(norm))
		out.Real = append(out.Real, real)
		out.written[real] = norm
	}
	return out, nil
}

// Written maps a path found under a real root back to the folder as the user
// wrote it, then writes the home as ~. When roots overlap, the first one in
// order wins, so a repository always gets the same name.
func (r ScanRoots) Written(e Env, dir string) string {
	for _, real := range r.Real {
		if rest, ok := strings.CutPrefix(dir, real); ok && (rest == "" || strings.HasPrefix(rest, string(filepath.Separator))) {
			return e.display(r.written[real] + rest)
		}
	}
	return e.display(dir)
}
