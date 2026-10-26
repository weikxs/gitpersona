package app

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/weikxs/gitpersona/internal/paths"
)

// configEntry is one setting as `git config --list --show-scope
// --show-origin -z` reports it. Keys are as Git prints them: section and
// name in lower case, a subsection as written.
type configEntry struct {
	scope  string // system, global, local, worktree, command
	origin string // "file:<path>", "command line:", …
	key    string
	value  string
}

// repoConfig is every setting Git reads in one directory, in the order Git
// reads them, so the last entry of a key is the value Git uses. Reading it
// once replaces one git process per setting.
type repoConfig []configEntry

// readRepoConfig lists the settings Git uses in dir (includes followed).
// Outside a repository it holds the system and global settings.
func (e Env) readRepoConfig(ctx context.Context, dir string) (repoConfig, error) {
	out, err := e.Run(ctx, "git", "-C", dir, "config", "--list", "--show-scope", "--show-origin", "-z")
	if err != nil {
		return nil, err
	}
	return parseRepoConfig(string(out)), nil
}

// parseRepoConfig reads the -z format: scope, origin and "key\nvalue" (or
// "key" for a setting without a value), each ended by NUL.
func parseRepoConfig(out string) repoConfig {
	fields := strings.Split(out, "\x00")
	var cfg repoConfig
	for i := 0; i+2 < len(fields); i += 3 {
		key, value, _ := strings.Cut(fields[i+2], "\n")
		cfg = append(cfg, configEntry{scope: fields[i], origin: fields[i+1], key: key, value: value})
	}
	return cfg
}

// get returns the value Git uses for key ("section.name", any case), as
// `git config <key>` does.
func (c repoConfig) get(key string) string {
	e, _ := c.last(key, "")
	return e.value
}

// getIn returns key's value from one scope only, includes and all.
func (c repoConfig) getIn(scope, key string) string {
	e, _ := c.last(key, scope)
	return e.value
}

// getLocal returns key's value from the repository's own config file, as
// `git config --local <key>` does: files that file includes do not count.
// root is the directory Git ran in, gitDir the repository's Git directory.
func (c repoConfig) getLocal(key, root, gitDir string) string {
	own := paths.ResolveSymlinks(filepath.Join(gitDir, "config"))
	key = normalKey(key)
	for i := len(c) - 1; i >= 0; i-- {
		e := c[i]
		if e.scope != "local" || normalKey(e.key) != key {
			continue
		}
		if file := originPath(e.origin, root); file != "" && paths.ResolveSymlinks(file) == own {
			return e.value
		}
	}
	return ""
}

// last finds the last entry for key, in scope when scope is not "".
func (c repoConfig) last(key, scope string) (configEntry, bool) {
	key = normalKey(key)
	for i := len(c) - 1; i >= 0; i-- {
		if normalKey(c[i].key) == key && (scope == "" || c[i].scope == scope) {
			return c[i], true
		}
	}
	return configEntry{}, false
}

// originFile returns the absolute path of the file key comes from, or "".
// Git prints the repository's own files relative to root.
func (c repoConfig) originFile(key, root string) string {
	e, ok := c.last(key, "")
	if !ok {
		return ""
	}
	return originPath(e.origin, root)
}

// originPath returns the absolute path of a "file:" origin, or "".
func originPath(origin, root string) string {
	file, isFile := strings.CutPrefix(origin, "file:")
	if !isFile || file == "" {
		return ""
	}
	if !filepath.IsAbs(file) {
		file = filepath.Join(root, file)
	}
	return file
}

// normalKey lowers the section and the name of key, which Git compares
// without case; a subsection keeps its case.
func normalKey(key string) string {
	first := strings.Index(key, ".")
	last := strings.LastIndex(key, ".")
	if first < 0 {
		return strings.ToLower(key)
	}
	return strings.ToLower(key[:first]) + key[first:last] + strings.ToLower(key[last:])
}
