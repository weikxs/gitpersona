package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/weikxs/gitpersona/internal/config"
)

const glabCommand = "glab"

// glabEnvTokens are the variables glab reads a token from; when the user
// sets one, they chose the account themselves.
var glabEnvTokens = []string{"GITLAB_TOKEN", "GITLAB_ACCESS_TOKEN", "OAUTH_TOKEN"}

// glabEntry is the glab link in the shim directory, which the shell
// integration puts first on PATH.
func (e Env) glabEntry() string { return filepath.Join(e.shimDir(), glabCommand) }

// glabEntryChange returns the change that makes the glab entry point at
// gitpersona, when some profile is on GitLab and the entry is not current.
func (e Env) glabEntryChange(cfg config.Config) (change, bool) {
	if !slices.ContainsFunc(cfg.Profiles, func(p config.Profile) bool { return p.Provider == config.ProviderGitLab }) {
		return change{}, false
	}
	if target, err := os.Readlink(e.glabEntry()); err == nil && target == e.Executable {
		return change{}, false
	}
	_, err := os.Lstat(e.glabEntry())
	return change{path: e.glabEntry(), linkTo: e.Executable, create: os.IsNotExist(err)}, true
}

// GlabShim runs the real glab as the account of the current directory's
// GitLab profile. glab keeps one account per host; GitPersona takes that
// account's token from the user's own credential store (see AuthLogin) and
// puts it only into the environment of that one glab process, with the
// profile's host. glab is left alone when the user set a token or another
// host, runs `glab auth`, or no GitLab profile applies.
func GlabShim(ctx context.Context, e Env, args []string) int {
	realGlab, err := e.realTool(glabCommand)
	if err != nil {
		fmt.Fprintf(e.Stderr, "gitpersona: %v; install it from https://gitlab.com/gitlab-org/cli\n", err)
		return exitCodeFailure
	}
	environ := os.Environ()
	isAuth := len(args) > 0 && args[0] == "auth"
	if p, ok := e.glabTarget(); ok && !isAuth {
		token, err := e.profileToken(ctx, p)
		if err != nil {
			// Never fall back to glab's own account: acting as another account
			// is exactly what GitPersona exists to prevent.
			fmt.Fprintf(e.Stderr, "gitpersona: there is no token for %s on %s, and this directory uses it.\n"+
				"Save one with `gitpersona auth login --profile %s`, or set GITLAB_TOKEN to choose an account yourself.\n",
				p.Account, p.Host, p.ID)
			return exitCodeFailure
		}
		environ = append(environ, "GITLAB_TOKEN="+strings.TrimSpace(token), "GITLAB_HOST="+p.Host)
	}
	if err := e.Exec(realGlab, append([]string{glabCommand}, args...), environ); err != nil {
		fmt.Fprintf(e.Stderr, "gitpersona: run %s: %v\n", realGlab, err)
		return exitCodeFailure
	}
	return 0
}

// glabTarget returns the GitLab profile glab should use here, if GitPersona
// should choose at all.
func (e Env) glabTarget() (config.Profile, bool) {
	if slices.ContainsFunc(glabEnvTokens, func(name string) bool { return os.Getenv(name) != "" }) {
		return config.Profile{}, false
	}
	p, ok := e.shimProfile(config.ProviderGitLab)
	if !ok {
		return config.Profile{}, false
	}
	for _, name := range []string{"GITLAB_HOST", "GITLAB_URI", "GL_HOST"} {
		if host := os.Getenv(name); host != "" && !strings.EqualFold(strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://"), p.Host) {
			return config.Profile{}, false
		}
	}
	return p, true
}
