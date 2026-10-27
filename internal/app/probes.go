package app

import (
	"context"
	"os"
	"sync"

	"github.com/weikxs/gitpersona/internal/shell"
)

// systemFacts are what the probes need to know about the running process.
type systemFacts struct {
	goos      string
	username  func() string
	getenv    func(string) string
	etcShells func() string // the content of /etc/shells
	pid       int
}

// probes remembers the facts about the system that do not change while
// gitpersona runs: Git's version. Env copies share it, so the configuration
// page asks once per process instead of once per request.
type probes struct {
	gitVersion func(context.Context) (gitVersion, bool)
}

// withProbes returns e with the login shell, the current shell, the
// installed shells and Git's version each asked at most once, through e.Run.
// The page's state used to ask dscl three times, walk the process tree twice
// and read /etc/shells and `git version` on every request.
func (e Env) withProbes(facts systemFacts) Env {
	run := shell.Runner(e.Run)
	e.LoginShell = memoize(func(ctx context.Context) shell.Detected {
		return shell.LoginShell(ctx, facts.goos, facts.username(), run, facts.getenv)
	})
	e.CurrentShell = memoize(func(ctx context.Context) shell.Detected {
		return shell.CurrentShell(ctx, facts.pid, run)
	})
	e.InstalledShells = sync.OnceValue(func() []shell.Installed {
		return shell.InstalledShells(facts.etcShells(), fileExists)
	})
	ask := e.Run
	versionOf := memoize(func(ctx context.Context) gitVersionAnswer {
		out, err := ask(ctx, "git", "version")
		if err != nil {
			return gitVersionAnswer{}
		}
		v, ok := parseGitVersion(string(out))
		return gitVersionAnswer{v, ok}
	})
	e.probes = &probes{gitVersion: func(ctx context.Context) (gitVersion, bool) {
		a := versionOf(ctx)
		return a.version, a.ok
	}}
	return e
}

// fileExists reports whether path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

type gitVersionAnswer struct {
	version gitVersion
	ok      bool
}

// memoize returns f that runs once and then repeats its answer. Concurrent
// callers wait for the first. An answer given while ctx ended (a cancelled
// request) is not kept, so the next caller asks again.
func memoize[T any](f func(context.Context) T) func(context.Context) T {
	var (
		mu    sync.Mutex
		done  bool
		value T
	)
	return func(ctx context.Context) T {
		mu.Lock()
		defer mu.Unlock()
		if done {
			return value
		}
		v := f(ctx)
		if ctx.Err() == nil {
			value, done = v, true
		}
		return v
	}
}
