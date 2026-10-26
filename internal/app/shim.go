package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/paths"
	"github.com/weikxs/gitpersona/internal/provider"
	"github.com/weikxs/gitpersona/internal/resolve"
)

const ghCommand = "gh"

// ghEntry is the gh link in the shim directory, which the shell integration
// puts first on PATH.
func (e Env) ghEntry() string { return filepath.Join(e.shimDir(), ghCommand) }

// ghEntryChange returns the change that makes the gh entry point at
// gitpersona, when some profile is on GitHub and the entry is not current.
func (e Env) ghEntryChange(cfg config.Config) (change, bool) {
	if !slices.ContainsFunc(cfg.Profiles, func(p config.Profile) bool { return p.Provider == config.ProviderGitHub }) {
		return change{}, false
	}
	if target, err := os.Readlink(e.ghEntry()); err == nil && target == e.Executable {
		return change{}, false
	}
	_, err := os.Lstat(e.ghEntry())
	return change{path: e.ghEntry(), linkTo: e.Executable, create: os.IsNotExist(err)}, true
}

// GHShim runs the real gh as the account of the current directory's profile.
// The profile's token goes only into the environment of that one gh process;
// it is never printed or stored. gh is left alone when the user set GH_TOKEN
// or GITHUB_TOKEN, targets another host, or no GitHub profile applies.
func GHShim(ctx context.Context, e Env, args []string) int {
	realGH, err := e.realGH()
	if err != nil {
		fmt.Fprintln(e.Stderr, "gitpersona: "+e.tr("%v", err))
		return exitCodeFailure
	}
	environ := withoutEmptyTokens(os.Environ())
	// gh auth manages accounts itself (and refuses to while GH_TOKEN is set).
	isAuth := len(args) > 0 && args[0] == "auth"
	if account, host, ok := e.ghTarget(); ok && !isAuth {
		token, err := provider.GitHub{GH: realGH, Run: e.Run}.Token(ctx, host, account)
		token = strings.TrimSpace(token)
		if err != nil || token == "" {
			// Never fall back to gh's active account here: acting as another
			// account is exactly what GitPersona exists to prevent.
			fmt.Fprintln(e.Stderr, "gitpersona: "+e.tr("%s is not signed in to gh on %s, and this directory uses it.\n"+
				"Sign in with `gh auth login --hostname %s`, or set GH_TOKEN to choose an account yourself.", account, host, host))
			return exitCodeFailure
		}
		environ = append(environ, "GH_TOKEN="+token)
	}
	if err := e.Exec(realGH, append([]string{ghCommand}, args...), environ); err != nil {
		fmt.Fprintln(e.Stderr, "gitpersona: "+e.tr("run %s: %v", realGH, err))
		return exitCodeFailure
	}
	return 0
}

const exitCodeFailure = 1

// withoutEmptyTokens drops GH_TOKEN= and GITHUB_TOKEN= with empty values.
// They choose nothing, but a process may read the first of two GH_TOKEN
// entries, so an empty one could hide the profile's token.
func withoutEmptyTokens(environ []string) []string {
	return slices.DeleteFunc(slices.Clone(environ), func(kv string) bool {
		return kv == "GH_TOKEN=" || kv == "GITHUB_TOKEN="
	})
}

// ghTarget returns the account and host gh should use here, if GitPersona
// should choose at all.
func (e Env) ghTarget() (string, string, bool) {
	if os.Getenv("GH_TOKEN") != "" || os.Getenv("GITHUB_TOKEN") != "" {
		return "", "", false // the user chose a token explicitly
	}
	p, ok := e.shimProfile(config.ProviderGitHub)
	if !ok {
		return "", "", false
	}
	if host := os.Getenv("GH_HOST"); host != "" && !strings.EqualFold(host, p.Host) {
		return "", "", false
	}
	return p.Account, p.Host, true
}

// shimProfile returns the profile of the current directory when it is on
// provider: the repository's, or outside a repository the directory rule's
// (what gh or glab does there, such as clone, makes repositories that will
// belong to that profile).
func (e Env) shimProfile(provider string) (config.Profile, bool) {
	cfg, err := config.Load(e.configFile())
	if err != nil {
		return config.Profile{}, false
	}
	opts := resolve.Options{Home: e.Home, CaseInsensitive: e.CaseInsensitive, Canonicalize: paths.ResolveSymlinks}
	r := resolve.Resolve(cfg, paths.ResolveSymlinks(e.WorkDir), opts)
	if gitDir, ok := findGitDir(e.WorkDir); ok {
		r = resolve.GitDir(cfg, paths.ResolveSymlinks(gitDir), opts)
	}
	if !r.Found || r.Profile.Provider != provider {
		return config.Profile{}, false
	}
	return r.Profile, true
}

// realGH finds gh on PATH, skipping the shim directory and anything that is
// gitpersona itself, so the shim never calls itself.
func (e Env) realGH() (string, error) {
	path, err := e.realTool(ghCommand)
	if err != nil {
		return "", e.trErr("%w; install it from https://cli.github.com", err)
	}
	return path, nil
}

// realTool finds a program on PATH, skipping the shim directory and
// anything that is gitpersona itself.
func (e Env) realTool(name string) (string, error) {
	// One stat per PATH entry finds the shim directory, however it is
	// written; resolving each entry's symbolic links would cost one lstat
	// per path component on every gh command.
	shim, shimErr := os.Stat(e.shimDir())
	self, selfErr := os.Stat(e.Executable)
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		if info, err := os.Stat(dir); err != nil || (shimErr == nil && os.SameFile(info, shim)) {
			continue
		}
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate) //nolint:gosec // only stats the user's own PATH entries to find the real tool
		if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			continue
		}
		if selfErr == nil && os.SameFile(info, self) {
			continue
		}
		return candidate, nil
	}
	return "", e.trErr("could not find the real %s on PATH (outside %s)", name, e.display(e.shimDir()))
}
