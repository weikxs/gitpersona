package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/weikxs/gitpersona/internal/block"
	"github.com/weikxs/gitpersona/internal/shell"
)

// shimDir holds GitPersona's gh and glab entries, put first on PATH by the
// shell integration.
func (e Env) shimDir() string { return filepath.Join(e.ConfigDir, "bin") }

// InitScript prints the integration script for a shell (`gitpersona init <shell>`).
func InitScript(e Env, kind shell.Kind) {
	fmt.Fprint(e.Stdout, shell.Script(kind, e.shimDir()))
}

// loader is what GitPersona adds to a shell's startup file: it loads the
// script from the binary, so upgrading GitPersona updates it too.
var loader = map[shell.Kind]string{
	shell.Fish: "command -q gitpersona; and gitpersona init fish | source\n",
	shell.Zsh:  "(( $+commands[gitpersona] )) && eval \"$(command gitpersona init zsh)\"\n",
	shell.Bash: "command -v gitpersona >/dev/null 2>&1 && eval \"$(command gitpersona init bash)\"\n",
}

// shellFiles returns the startup files GitPersona edits for a shell. fish
// gets a file of its own in conf.d; zsh uses ~/.zshenv, which non-interactive
// shells read too; bash uses ~/.bashrc (and ~/.bash_profile on macOS, where
// terminals start login shells).
func (e Env) shellFiles(k shell.Kind) []string {
	switch k {
	case shell.Fish:
		return []string{filepath.Join(filepath.Dir(e.ConfigDir), "fish", "conf.d", "gitpersona.fish")}
	case shell.Zsh:
		return []string{filepath.Join(e.Home, ".zshenv")}
	default:
		files := []string{filepath.Join(e.Home, ".bashrc")}
		if runtime.GOOS == "darwin" {
			files = append(files, filepath.Join(e.Home, ".bash_profile"))
		}
		return files
	}
}

func (e Env) loginShell(ctx context.Context) shell.Detected {
	if e.LoginShell == nil {
		return shell.Detected{}
	}
	return e.LoginShell(ctx)
}

// InstallShell adds the integration to the given shells, or to the login
// shell when none are given.
func InstallShell(ctx context.Context, e Env, kinds []shell.Kind, dryRun bool) error {
	if len(kinds) == 0 {
		login := e.loginShell(ctx)
		if login.Kind == shell.Unknown {
			return fmt.Errorf("could not tell which shell you use (%q); choose with --shell fish, zsh or bash", login.Path)
		}
		fmt.Fprintf(e.Stdout, "Setting up %s (your login shell, from %s).\n", login.Kind, login.Source)
		kinds = []shell.Kind{login.Kind}
		e.mentionOtherShells(login.Kind)
	}

	var changes []change
	for _, k := range kinds {
		for _, path := range e.shellFiles(k) {
			c, err := e.upsertFile(path, loader[k])
			if err != nil {
				return err
			}
			if c != nil {
				changes = append(changes, *c)
			}
		}
	}
	if err := e.applyChanges(changes, ApplyOptions{DryRun: dryRun}); err != nil {
		return err
	}
	if len(changes) > 0 && !dryRun {
		fmt.Fprintln(e.Stdout, "Open a new terminal (or source the file) to start using it.")
	}
	return nil
}

// upsertFile returns the change that puts body in the GitPersona block of
// path (following symlinks, keeping the file's mode), or nil if it is current.
func (e Env) upsertFile(path, body string) (*change, error) {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	current, err := readOptional(path)
	if err != nil {
		return nil, err
	}
	updated, err := block.Upsert(current, body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", e.display(path), err)
	}
	if updated == current {
		return nil, nil
	}
	c := change{path: path, content: updated}
	if info, err := os.Stat(path); err == nil {
		c.perm = info.Mode().Perm()
	} else {
		c.create, c.perm = true, 0o644 // shell startup files are conventionally readable
	}
	return &c, nil
}

// removeShellBlocks returns the changes that take the integration out of
// every shell; files that held only the block are removed.
func (e Env) removeShellBlocks() ([]change, error) {
	var changes []change
	for _, k := range shell.All {
		for _, path := range e.shellFiles(k) {
			if real, err := filepath.EvalSymlinks(path); err == nil {
				path = real
			}
			current, err := readOptional(path)
			if err != nil || !block.Contains(current) {
				continue
			}
			without, err := block.Remove(current)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", e.display(path), err)
			}
			perm := os.FileMode(0)
			if info, err := os.Stat(path); err == nil {
				perm = info.Mode().Perm()
			}
			changes = append(changes, change{path: path, content: without, perm: perm, remove: without == ""})
		}
	}
	return changes, nil
}

// checkShellIntegration warns when the default shell, or the shell in use
// right now, lacks the integration.
func (e Env) checkShellIntegration(ctx context.Context) []check {
	var checks []check
	login := e.loginShell(ctx)
	if login.Kind != shell.Unknown {
		if path, ok := e.integrated(login.Kind); ok {
			checks = append(checks, check{ok: true, label: fmt.Sprintf("%s integration is installed (%s)", login.Kind, e.display(path))})
		} else {
			checks = append(checks, check{warn: true,
				label:  fmt.Sprintf("%s integration is not installed (prompt display and gh switching need it)", login.Kind),
				advice: "run `gitpersona install-shell`"})
		}
	}
	current := e.currentShell(ctx)
	if current.Kind != shell.Unknown && current.Kind != login.Kind {
		if _, ok := e.integrated(current.Kind); !ok {
			checks = append(checks, check{warn: true,
				label:  fmt.Sprintf("you are using %s right now, but your default shell is %s; %s has no integration", current.Kind, login.Kind, current.Kind),
				advice: fmt.Sprintf("run `gitpersona install-shell --shell %s` if you use it regularly", current.Kind)})
		}
	}
	return checks
}

// integrated reports whether a shell's startup file has the GitPersona block.
func (e Env) integrated(k shell.Kind) (string, bool) {
	for _, path := range e.shellFiles(k) {
		if content, err := readOptional(path); err == nil && block.Contains(content) {
			return path, true
		}
	}
	return "", false
}

func (e Env) currentShell(ctx context.Context) shell.Detected {
	if e.CurrentShell == nil {
		return shell.Detected{}
	}
	return e.CurrentShell(ctx)
}

func (e Env) installedShells() []shell.Installed {
	if e.InstalledShells == nil {
		return nil
	}
	return e.InstalledShells()
}

// mentionOtherShells tells the user about installed shells besides the default.
func (e Env) mentionOtherShells(def shell.Kind) {
	var others, all []string
	for _, in := range e.installedShells() {
		all = append(all, string(in.Kind))
		if in.Kind != def {
			others = append(others, string(in.Kind))
		}
	}
	if len(others) > 0 {
		fmt.Fprintf(e.Stdout, "Also installed: %s. To set them up too: gitpersona install-shell --shell %s\n",
			strings.Join(others, ", "), strings.Join(all, ","))
	}
}
