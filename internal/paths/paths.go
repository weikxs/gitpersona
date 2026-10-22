// Package paths normalizes and compares the directory paths used in rules.
//
// Paths are compared as whole path segments, so a rule for ~/work never
// matches ~/work2.
package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const separator = "/"

// CaseInsensitiveFS reports whether paths on this platform should be compared
// without regard to case. macOS uses a case-insensitive file system by default.
func CaseInsensitiveFS() bool {
	return runtime.GOOS == "darwin"
}

// Normalize expands a leading "~" to home and returns a clean absolute path
// without a trailing separator.
func Normalize(p, home string) (string, error) {
	if p == "" {
		return "", errors.New("path is empty")
	}
	expanded, err := expandHome(p, home)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(expanded) {
		return "", fmt.Errorf("path %q must be absolute or start with ~/", p)
	}
	return filepath.Clean(expanded), nil
}

func expandHome(p, home string) (string, error) {
	switch {
	case p == "~":
		return home, nil
	case strings.HasPrefix(p, "~/"):
		return filepath.Join(home, p[2:]), nil
	case strings.HasPrefix(p, "~"):
		return "", fmt.Errorf("path %q: only ~/ is supported, not ~user", p)
	default:
		return p, nil
	}
}

// Within reports whether path is dir itself or lies inside dir.
// Both arguments must already be normalized.
func Within(path, dir string, caseInsensitive bool) bool {
	if caseInsensitive {
		path, dir = strings.ToLower(path), strings.ToLower(dir)
	}
	if path == dir || dir == separator {
		return true
	}
	return strings.HasPrefix(path, dir+separator)
}

// Same reports whether two normalized paths name the same location.
func Same(a, b string, caseInsensitive bool) bool {
	if caseInsensitive {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// ResolveSymlinks returns p with symbolic links resolved in its longest
// existing prefix. Missing trailing components are kept as they are, so rules
// can name directories that do not exist yet.
func ResolveSymlinks(p string) string {
	existing, rest := p, ""
	for {
		if resolved, err := filepath.EvalSymlinks(existing); err == nil {
			return filepath.Join(resolved, rest)
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return p
		}
		rest = filepath.Join(filepath.Base(existing), rest)
		existing = parent
	}
}

// Home returns the user's home directory.
func Home() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return home, nil
}
