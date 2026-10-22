// Package app implements the gitpersona commands on top of the core
// packages. Everything that touches the outside world comes in through Env,
// so tests can run the commands against a temporary home directory.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/weikxs/gitpersona/internal/paths"
	"github.com/weikxs/gitpersona/internal/provider"
)

// Env is the outside world as seen by the commands.
type Env struct {
	Home            string
	ConfigDir       string
	WorkDir         string
	Executable      string
	CaseInsensitive bool

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	Now      func() time.Time
	LookPath func(string) (string, error)
	Run      provider.Runner
}

// FromOS builds the Env of the running process.
func FromOS(stdin io.Reader, stdout, stderr io.Writer) (Env, error) {
	home, err := paths.Home()
	if err != nil {
		return Env{}, err
	}
	wd, err := os.Getwd()
	if err != nil {
		return Env{}, fmt.Errorf("find current directory: %w", err)
	}
	exe, err := os.Executable()
	if err != nil {
		return Env{}, fmt.Errorf("find gitpersona executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return Env{
		Home:            home,
		ConfigDir:       configDir(home),
		WorkDir:         wd,
		Executable:      exe,
		CaseInsensitive: paths.CaseInsensitiveFS(),
		Stdin:           stdin,
		Stdout:          stdout,
		Stderr:          stderr,
		Now:             time.Now,
		LookPath:        exec.LookPath,
		Run:             provider.ExecRunner,
	}, nil
}

// configDir follows the XDG base directory convention.
func configDir(home string) string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "gitpersona")
	}
	return filepath.Join(home, ".config", "gitpersona")
}

// Locations derived from the environment.
func (e Env) configFile() string    { return filepath.Join(e.ConfigDir, "config.toml") }
func (e Env) backupDir() string     { return filepath.Join(e.ConfigDir, "backups") }
func (e Env) userGitConfig() string { return filepath.Join(e.Home, ".gitconfig") }

// git runs git in dir and returns its trimmed standard output.
func (e Env) git(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := e.Run(ctx, "git", append([]string{"-C", dir}, args...)...)
	return strings.TrimSpace(string(out)), err
}

// gitConfigAll returns every value of a global Git setting, or nil when the
// setting is absent.
func (e Env) gitConfigAll(ctx context.Context, key string) []string {
	out, err := e.git(ctx, e.Home, "config", "--global", "--get-all", key)
	if err != nil || out == "" {
		return nil
	}
	return strings.Split(out, "\n")
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
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), nil
}
