// Package discover finds what a user already has, so GitPersona can suggest
// a configuration instead of asking for one. It only reads Git configuration
// and repository metadata; it never reads private keys or tokens.
package discover

import (
	"context"
	"path/filepath"
	"slices"
	"strings"

	"github.com/weikxs/gitpersona/internal/provider"
)

// Identity is one identity found in an existing setup.
type Identity struct {
	Name    string
	Email   string
	Account string   // credential username for github.com, if set
	Dirs    []string // directories, written with ~/ where possible
	Source  string   // the included file the identity came from
}

const includeIfPrefix = "includeif."

// Existing reads the includeIf rules of a global Git configuration and the
// files they include, and returns one identity per distinct email, in the
// order the rules appear. Rules that are not plain directory rules (such as
// hasconfig:) are skipped. GitPersona's own include is ignored.
func Existing(ctx context.Context, home, gitconfig string, run provider.Runner) ([]Identity, error) {
	out, err := run(ctx, "git", "config", "--file", gitconfig, "--get-regexp", `^includeif\..*\.path$`)
	if err != nil {
		return nil, nil // no includeIf rules (or no file): nothing to import
	}

	var identities []Identity
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		key, path, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		condition := strings.TrimSuffix(strings.TrimPrefix(key, includeIfPrefix), ".path")
		dir := gitdirToDir(condition)
		if dir == "" {
			continue
		}
		included := expandHome(strings.TrimSpace(path), home)
		id := readIdentity(ctx, included, run)
		if id.Email == "" {
			continue
		}
		identities = merge(identities, id, dir)
	}
	return identities, nil
}

func readIdentity(ctx context.Context, file string, run provider.Runner) Identity {
	get := func(key string) string {
		out, err := run(ctx, "git", "config", "--file", file, "--get", key)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	return Identity{
		Name:    get("user.name"),
		Email:   get("user.email"),
		Account: get("credential.https://github.com.username"),
		Source:  file,
	}
}

func merge(identities []Identity, id Identity, dir string) []Identity {
	i := slices.IndexFunc(identities, func(x Identity) bool { return x.Email == id.Email })
	if i < 0 {
		id.Dirs = []string{dir}
		return append(identities, id)
	}
	out := slices.Clone(identities)
	if !slices.Contains(out[i].Dirs, dir) {
		out[i].Dirs = append(slices.Clone(out[i].Dirs), dir)
	}
	return out
}

// gitdirToDir turns an includeIf condition into the directory it covers, or
// "" when the condition is not a plain directory rule.
func gitdirToDir(condition string) string {
	pattern, ok := strings.CutPrefix(condition, "gitdir:")
	if !ok {
		pattern, ok = strings.CutPrefix(condition, "gitdir/i:")
	}
	if !ok {
		return ""
	}
	pattern = strings.TrimSuffix(strings.TrimSuffix(pattern, "**"), "/")
	if !strings.HasSuffix(condition, "/") && !strings.HasSuffix(condition, "/**") {
		return "" // an exact .git directory: a single repository, not a directory rule
	}
	if !strings.HasPrefix(pattern, "~/") && !filepath.IsAbs(pattern) {
		return ""
	}
	return pattern
}

func expandHome(path, home string) string {
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		return filepath.Join(home, rest)
	}
	return path
}
