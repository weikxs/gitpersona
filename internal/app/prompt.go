package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/paths"
	"github.com/weikxs/gitpersona/internal/resolve"
)

const defaultPromptFormat = "{account}"

// Prompt prints the account Git uses in the current directory, for shell
// prompts. It runs on every command, so it never starts git or touches the
// network: it finds the Git directory itself, applying the same rules as
// `whoami`. Any problem prints nothing rather than breaking the prompt.
func Prompt(e Env, format string) {
	cfg, err := config.Load(e.configFile())
	if err != nil {
		return
	}
	opts := resolve.Options{Home: e.Home, CaseInsensitive: e.CaseInsensitive, Canonicalize: paths.ResolveSymlinks}

	r := resolve.OutsideRepository(cfg)
	if gitDir, ok := findGitDir(e.WorkDir); ok {
		r = resolve.GitDir(cfg, paths.ResolveSymlinks(gitDir), opts)
	}
	if !r.Found {
		return
	}
	if format == "" {
		format = defaultPromptFormat
	}
	fmt.Fprintln(e.Stdout, strings.NewReplacer(
		"{account}", r.Profile.Account,
		"{profile}", r.Profile.ID,
		"{host}", r.Profile.Host,
	).Replace(format))
}

// findGitDir walks up from dir to the Git directory Git would use: a .git
// directory, the target of a .git file (worktrees and submodules), or dir
// itself when it is a Git directory (a bare repository or the inside of .git).
func findGitDir(dir string) (string, bool) {
	for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
		if isGitDir(d) {
			return d, true
		}
		dotGit := filepath.Join(d, ".git")
		if info, err := os.Stat(dotGit); err == nil {
			if info.IsDir() {
				return dotGit, true
			}
			if target, ok := readGitFile(dotGit); ok {
				return target, true
			}
		}
		if parent := filepath.Dir(d); parent == d {
			return "", false
		}
	}
}

// isGitDir reports whether dir looks like a Git directory.
func isGitDir(dir string) bool {
	for _, name := range []string{"HEAD", "objects", "refs"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			return false
		}
	}
	return true
}

// readGitFile reads a ".git" file of the form "gitdir: <path>".
func readGitFile(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	target, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
	if !ok {
		return "", false
	}
	target = strings.TrimSpace(target)
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	return target, true
}
