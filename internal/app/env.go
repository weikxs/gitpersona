// Package app implements the gitpersona commands on top of the core
// packages. Everything that touches the outside world comes in through Env,
// so tests can run the commands against a temporary home directory.
package app

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/weikxs/gitpersona/internal/i18n"
	"github.com/weikxs/gitpersona/internal/paths"
	"github.com/weikxs/gitpersona/internal/provider"
	"github.com/weikxs/gitpersona/internal/shell"
)

// Env is the outside world as seen by the commands.
type Env struct {
	Home      string
	ConfigDir string
	WorkDir   string
	// Executable is the path Git runs gitpersona by. Empty means
	// FindExecutable finds it, on first use: only apply and setup need it,
	// not the prompt, the shims or the credential helper.
	Executable      string
	FindExecutable  func() (string, error)
	CaseInsensitive bool
	// GlobalGitConfig is GIT_CONFIG_GLOBAL when set; empty means ~/.gitconfig.
	GlobalGitConfig string
	// Lang is the language of the messages shown to the user: the page's
	// language for the configuration page; English (the zero value) on the
	// command line. Content written to files never depends on it.
	Lang i18n.Lang

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	Now      func() time.Time
	LookPath func(string) (string, error)
	Run      provider.Runner
	// RunInput runs a program with input (nil: provider.ExecInputRunner).
	RunInput provider.InputRunner
	// LoginShell detects the user's default (login) shell; nil means unknown.
	LoginShell func(context.Context) shell.Detected
	// CurrentShell detects the shell running gitpersona right now; nil means unknown.
	CurrentShell func(context.Context) shell.Detected
	// InstalledShells lists the supported shells on the system; nil means none known.
	InstalledShells func() []shell.Installed
	// Exec replaces the process with another program (the real gh).
	Exec func(path string, argv, env []string) error
	// ForgetQueries drops the remembered answers of gh's account queries, so
	// the next one asks the host again (nil: nothing is remembered).
	ForgetQueries func()

	// Writes keeps an interruption (Ctrl-C) from stopping gitpersona while
	// it writes files; nil means no guard (tests).
	Writes *WriteGuard
	// configLocked is set while this command holds the configuration lock.
	configLocked bool
	// lockWait is how long to wait for the lock (0: defaultLockWait).
	lockWait time.Duration
	// write writes one change (nil: change.write); tests inject failures.
	write func(change) error

	// probes holds answers about the system kept for the whole process
	// (nil: ask each time).
	probes *probes
}

// accountQueryTTL is how long the answers of gh's account queries, which
// reach the host once per signed-in account, are reused (provider.CachingRunner).
const accountQueryTTL = 10 * time.Minute

// FromOS builds the Env of the running process.
func FromOS(stdin io.Reader, stdout, stderr io.Writer) (Env, error) {
	home, err := paths.Home()
	if err != nil {
		return Env{}, err
	}
	wd, err := os.Getwd()
	if err != nil {
		return Env{}, i18n.Errorf("find current directory: %w", err)
	}
	queries := &provider.CachingRunner{Next: provider.ExecRunner, TTL: accountQueryTTL, Now: time.Now}
	env := Env{
		Home:            home,
		ConfigDir:       configDir(home),
		WorkDir:         wd,
		FindExecutable:  sync.OnceValues(findExecutable),
		CaseInsensitive: paths.CaseInsensitiveAt(home),
		GlobalGitConfig: os.Getenv("GIT_CONFIG_GLOBAL"),
		Stdin:           stdin,
		Stdout:          stdout,
		Stderr:          stderr,
		Now:             time.Now,
		LookPath:        exec.LookPath,
		Run:             queries.Run,
		ForgetQueries:   queries.Forget,
		Exec:            syscall.Exec,
		Writes:          &WriteGuard{},
	}
	return env.withProbes(systemFacts{
		goos: runtime.GOOS,
		username: func() string {
			if u, err := user.Current(); err == nil {
				return u.Username
			}
			return ""
		},
		getenv: os.Getenv,
		etcShells: func() string {
			etc, _ := os.ReadFile("/etc/shells")
			return string(etc)
		},
		pid: os.Getpid(),
	}), nil
}

// WriteGuard lets files be written to the end before an interrupted
// gitpersona exits: writing holds it shared, and Hold waits for every write
// in progress and keeps new ones from starting.
type WriteGuard struct{ mu sync.RWMutex }

// Writing marks a write phase; call the returned function when it ends.
func (g *WriteGuard) Writing() func() {
	if g == nil {
		return func() {}
	}
	g.mu.RLock()
	return g.mu.RUnlock
}

// Hold waits until no files are being written and keeps them from being
// written from then on; call it before exiting on a signal.
func (g *WriteGuard) Hold() { g.mu.Lock() }

// findExecutable returns the path of the running gitpersona as Git should
// run it.
func findExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", i18n.Errorf("find gitpersona executable: %w", err)
	}
	return stableExecutable(exe, exec.LookPath), nil
}

// executable is Executable, found on first use when it is not set.
func (e Env) executable() (string, error) {
	if e.Executable != "" || e.FindExecutable == nil {
		return e.Executable, nil
	}
	return e.FindExecutable()
}

// self is the running program's own file, for telling it apart from the
// real gh or glab; cheaper than executable, which also searches PATH.
func (e Env) self() string {
	if e.Executable != "" {
		return e.Executable
	}
	exe, _ := os.Executable()
	return exe
}

// stableExecutable returns the path Git should use to run gitpersona as a
// credential helper. Package managers install the real binary in a versioned
// directory and put a symbolic link on PATH; the link survives upgrades while
// the versioned path does not, so the PATH entry wins when it is this binary.
func stableExecutable(exe string, lookPath func(string) (string, error)) string {
	onPath, err := lookPath("gitpersona")
	if err != nil {
		return exe
	}
	abs, err := filepath.Abs(onPath)
	if err != nil {
		return exe
	}
	a, errA := os.Stat(abs)
	b, errB := os.Stat(exe)
	if errA != nil || errB != nil || !os.SameFile(a, b) {
		return exe
	}
	return abs
}

// configDir follows the XDG base directory convention.
func configDir(home string) string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "gitpersona")
	}
	return filepath.Join(home, ".config", "gitpersona")
}

// Locations derived from the environment.
func (e Env) configFile() string { return filepath.Join(e.ConfigDir, "config.toml") }
func (e Env) backupDir() string  { return filepath.Join(e.ConfigDir, "backups") }

// userGitConfig is the global Git configuration file Git reads, which may be
// a symbolic link managed by a dotfiles tool.
func (e Env) userGitConfig() string {
	if e.GlobalGitConfig != "" {
		return e.GlobalGitConfig
	}
	return filepath.Join(e.Home, ".gitconfig")
}

// git runs git in dir and returns its trimmed standard output.
func (e Env) git(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := e.Run(ctx, "git", append([]string{"-C", dir}, args...)...)
	return strings.TrimSpace(string(out)), err
}

// gitConfigMissing is the exit status of `git config --get` for an absent key.
const gitConfigMissing = 1

// globalSettings reads the user's own global Git settings: the global files
// and the files they include (such as dotfiles), never GitPersona's, with
// one git process per file. It maps each key (as normalKey writes it) to its
// values in the order Git reads them. Any failure (for example a broken
// configuration file) is an error, so callers never act on a silently
// empty answer.
func (e Env) globalSettings(ctx context.Context) (map[string][]string, error) {
	settings := map[string][]string{}
	for _, file := range e.globalFiles() {
		if _, err := os.Stat(file); err != nil {
			continue
		}
		out, err := e.Run(ctx, "git", "-C", e.Home, "config", "--file", file, "--list", "-z")
		if err != nil {
			return nil, e.trErr("read global Git settings from %s: %w", e.display(file), err)
		}
		for _, entry := range strings.Split(string(out), "\x00") {
			if key, value, _ := strings.Cut(entry, "\n"); key != "" {
				settings[normalKey(key)] = append(settings[normalKey(key)], value)
			}
		}
	}
	return settings, nil
}

// lastSetting returns the value Git uses for key among settings, or "".
func lastSetting(settings map[string][]string, key string) string {
	values := settings[normalKey(key)]
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

// gitConfigRegexp returns the global settings whose names match pattern,
// as name/value pairs in the order Git reads them.
// The system settings come first, as Git reads them (unless
// GIT_CONFIG_NOSYSTEM is set): many installations set a credential helper
// there.
func (e Env) gitConfigRegexp(ctx context.Context, pattern string) ([][2]string, error) {
	sources := [][]string{}
	if os.Getenv("GIT_CONFIG_NOSYSTEM") == "" {
		sources = append(sources, []string{"--system"})
	}
	for _, file := range e.globalFiles() {
		if _, err := os.Stat(file); err == nil {
			sources = append(sources, []string{"--file", file})
		}
	}
	var entries [][2]string
	for _, src := range sources {
		args := append(append([]string{"config"}, src...), "--null", "--get-regexp", pattern)
		out, err := e.git(ctx, e.Home, args...)
		var exit *exec.ExitError
		switch {
		case errors.As(err, &exit) && exit.ExitCode() == gitConfigMissing:
		case err != nil:
			return nil, e.trErr("read Git settings %s (%s): %w", pattern, e.display(src[len(src)-1]), err)
		default:
			for _, entry := range strings.Split(out, "\x00") {
				// A name without a value ("helper" with no "=") is not
				// a reset; Git refuses it.
				if name, value, hasValue := strings.Cut(entry, "\n"); name != "" && hasValue {
					entries = append(entries, [2]string{name, value})
				}
			}
		}
	}
	return entries, nil
}

// repoRoot returns the top-level directory of the repository containing dir.
func (e Env) repoRoot(ctx context.Context, dir string) (string, bool) {
	root, err := e.git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil || root == "" {
		return "", false
	}
	return root, true
}

// readOptional returns the file content, or "" if the file does not exist.
func readOptional(path string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", i18n.Errorf("read %s: %w", path, err)
	}
	return string(data), nil
}

// GlobalGitConfigPath is the global Git configuration file GitPersona edits.
func (e Env) GlobalGitConfigPath() string { return e.userGitConfig() }

// BackupDir is where backups are kept.
func (e Env) BackupDir() string { return e.backupDir() }

// Display shortens a path under the home directory to ~/...
func (e Env) Display(path string) string { return e.display(path) }

// tr is a message for the user in e's language. Only for text that is shown:
// never for file content, values that are parsed, or keys that are compared.
func (e Env) tr(format string, args ...any) string {
	return i18n.T(e.lang(), format, args...)
}

// trErr is an error for the user, shown in any language (the page translates
// it where it shows it; the command line shows English). It wraps with %w.
func (e Env) trErr(format string, args ...any) error {
	return i18n.Errorf(format, args...)
}

func (e Env) lang() i18n.Lang {
	if e.Lang == "" {
		return i18n.EN
	}
	return e.Lang
}
