package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/weikxs/gitpersona/internal/block"
	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/paths"
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
	// bash's login file may be ~/.profile, which sh reads too: run only under bash.
	shell.Bash: "[ -n \"$BASH_VERSION\" ] && command -v gitpersona >/dev/null 2>&1 && eval \"$(command gitpersona init bash)\"\n",
}

// shellFiles returns the startup files install-shell writes for a shell.
//   - fish: a file of its own in conf.d.
//   - zsh: .zshenv (non-interactive shells read it too) and .zshrc, which
//     runs after macOS path_helper (in /etc/zprofile) reorders PATH, in
//     $ZDOTDIR when it is set (see zshFile).
//   - bash: ~/.bashrc, and on macOS, where terminals start login shells, the
//     first login file bash would read; ~/.bash_profile is created only when
//     none exists, because it would hide ~/.bash_login and ~/.profile.
func (e Env) shellFiles(k shell.Kind) []string {
	switch k {
	case shell.Fish:
		return []string{filepath.Join(e.fishConfigDir(), "conf.d", "gitpersona.fish")}
	case shell.Zsh:
		return []string{e.zshFile(".zshenv"), e.zshFile(".zshrc")}
	default:
		files := []string{filepath.Join(e.Home, ".bashrc")}
		if runtime.GOOS == "darwin" {
			files = append(files, e.bashLoginFile())
		}
		return files
	}
}

// zshDir is where zsh reads its startup files: $ZDOTDIR, else the home
// directory (as install.sh does).
func (e Env) zshDir() string {
	if dir := os.Getenv("ZDOTDIR"); filepath.IsAbs(dir) {
		return dir
	}
	return e.Home
}

// zshFile is the startup file name that zsh reads. ZDOTDIR is usually
// exported from ~/.zshenv, which zsh reads before it knows ZDOTDIR; that file
// stays where it is, and the others are in ZDOTDIR.
func (e Env) zshFile(name string) string {
	inDir := filepath.Join(e.zshDir(), name)
	if name == ".zshenv" && !fileExists(inDir) && fileExists(filepath.Join(e.Home, name)) {
		return filepath.Join(e.Home, name)
	}
	return inDir
}

func (e Env) fishConfigDir() string { return filepath.Join(filepath.Dir(e.ConfigDir), "fish") }

// bashLoginFile is the file a bash login shell reads: the first existing of
// ~/.bash_profile, ~/.bash_login and ~/.profile.
func (e Env) bashLoginFile() string {
	for _, name := range []string{".bash_profile", ".bash_login", ".profile"} {
		if _, err := os.Stat(filepath.Join(e.Home, name)); err == nil {
			return filepath.Join(e.Home, name)
		}
	}
	return filepath.Join(e.Home, ".bash_profile")
}

// startupFiles lists every file where a shell's integration may live,
// including the setups docs/integrations.md describes (fisher, init lines
// written by hand).
func (e Env) startupFiles(k shell.Kind) []string {
	switch k {
	case shell.Fish:
		files, _ := filepath.Glob(filepath.Join(e.fishConfigDir(), "conf.d", "*.fish"))
		return append(files, filepath.Join(e.fishConfigDir(), "config.fish"))
	case shell.Zsh:
		files := []string{}
		for _, name := range []string{".zshenv", ".zshrc", ".zprofile", ".zlogin"} {
			files = append(files, filepath.Join(e.zshDir(), name))
		}
		if e.zshDir() != e.Home {
			files = append(files, filepath.Join(e.Home, ".zshenv")) // where ZDOTDIR is often set
		}
		return files
	default:
		files := []string{}
		for _, name := range []string{".bashrc", ".bash_profile", ".bash_login", ".profile"} {
			files = append(files, filepath.Join(e.Home, name))
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

// PreviewShells returns the startup files InstallShell would change for the
// given shells, with their diffs, without writing anything.
func PreviewShells(e Env, kinds []shell.Kind) ([]FileChange, error) {
	quiet := e
	quiet.Stdout = io.Discard
	changes, err := quiet.shellChanges(kinds)
	if err != nil {
		return nil, err
	}
	return e.fileChanges(changes), nil
}

// shellChanges adds the loader block to each startup file of kinds, leaving
// shells that are set up some other way alone.
func (e Env) shellChanges(kinds []shell.Kind) ([]change, error) {
	var changes []change
	for _, k := range kinds {
		if path, ok := e.integratedElsewhere(k); ok {
			fmt.Fprintln(e.Stdout, e.tr("%s is already set up in %s; leaving it as it is.", k, e.display(path)))
			continue
		}
		for _, path := range e.shellFiles(k) {
			c, err := e.upsertFile(path, loader[k])
			if err != nil {
				return nil, err
			}
			if c != nil {
				changes = append(changes, *c)
			}
		}
	}
	return changes, nil
}

// InstallShell adds the integration to the given shells, or to the login
// shell when none are given.
func InstallShell(ctx context.Context, e Env, kinds []shell.Kind, dryRun bool) error {
	e, unlock, err := e.lockConfig()
	if err != nil {
		return err
	}
	defer unlock()
	if len(kinds) == 0 {
		login := e.loginShell(ctx)
		if login.Kind == shell.Unknown {
			return e.trErr("could not tell which shell you use (%q); choose with --shell fish, zsh or bash", login.Path)
		}
		fmt.Fprintln(e.Stdout, e.tr("Setting up %s (your login shell, from %s).", login.Kind, login.Source))
		kinds = []shell.Kind{login.Kind}
		e.mentionOtherShells(login.Kind)
	}

	changes, err := e.shellChanges(kinds)
	if err != nil {
		return err
	}
	if err := e.applyChanges(changes, ApplyOptions{DryRun: dryRun, Command: "install-shell"}); err != nil {
		return err
	}
	if len(changes) > 0 && !dryRun {
		fmt.Fprintln(e.Stdout, e.tr("Open a new terminal (or source the file) to start using it."))
	}
	return nil
}

// upsertFile returns the change that puts body in the GitPersona block of
// path (following symlinks, keeping the file's mode), or nil if it is current.
func (e Env) upsertFile(path, body string) (*change, error) {
	if realPath, err := filepath.EvalSymlinks(path); err == nil {
		path = realPath
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
		for _, path := range e.startupFiles(k) {
			if realPath, err := filepath.EvalSymlinks(path); err == nil {
				path = realPath
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
// When the configuration turns the integration off, its absence is fine.
func (e Env) checkShellIntegration(ctx context.Context, cfg config.Config) []check {
	var checks []check
	login := e.loginShell(ctx)
	if login.Kind != shell.Unknown {
		if path, ok := e.integrated(login.Kind); ok {
			checks = append(checks, check{ok: true, label: e.tr("%s integration is installed (%s)", login.Kind, e.display(path))})
		} else if cfg.ShellIntegrationOff() {
			return append(checks, check{ok: true, label: e.tr("shell integration is turned off (shell_integration = false in %s)", e.display(e.configFile()))})
		} else {
			checks = append(checks, check{warn: true,
				label:  e.tr("%s integration is not installed (prompt display and gh/glab switching need it)", login.Kind),
				advice: e.tr("run `gitpersona install-shell`"),
				fix:    func(ctx context.Context, fixer Env) error { return InstallShell(ctx, fixer, nil, false) }, fixKey: "shell"})
		}
	}
	installed := slices.ContainsFunc([]shell.Kind{shell.Fish, shell.Zsh, shell.Bash}, func(k shell.Kind) bool {
		_, ok := e.integrated(k)
		return ok
	})
	hasGitHub := slices.ContainsFunc(cfg.Profiles, func(p config.Profile) bool { return p.Provider == config.ProviderGitHub })
	if files := e.ghSwitchers(); len(files) > 0 && installed && hasGitHub {
		checks = append(checks, check{warn: true, label: e.ghSwitcherNote(files),
			advice: e.tr("remove your own gh function if GitPersona's profiles cover the same directories")})
	}
	current := e.currentShell(ctx)
	if current.Kind != shell.Unknown && current.Kind != login.Kind {
		if _, ok := e.integrated(current.Kind); !ok {
			checks = append(checks, check{warn: true,
				label:  e.tr("you are using %s right now, but your default shell is %s; %s has no integration", current.Kind, login.Kind, current.Kind),
				advice: e.tr("run `gitpersona install-shell --shell %s` if you use it regularly", current.Kind)})
		}
	}
	return checks
}

// integrated reports whether any startup file of a shell loads the
// integration, however it was set up.
func (e Env) integrated(k shell.Kind) (string, bool) {
	marker := "gitpersona init " + string(k)
	for _, path := range e.startupFiles(k) {
		if content, err := readOptional(path); err == nil && strings.Contains(content, marker) {
			return path, true
		}
	}
	return "", false
}

// integratedElsewhere reports a setup that install-shell did not make (no
// GitPersona block), such as a fisher plugin, which install-shell must leave alone.
func (e Env) integratedElsewhere(k shell.Kind) (string, bool) {
	path, ok := e.integrated(k)
	if !ok {
		return "", false
	}
	content, _ := readOptional(path)
	return path, !block.Contains(content)
}

// checkGHRouting warns when the gh found on PATH in this shell is not
// GitPersona's entry, so gh would not follow the directory.
func (e Env) checkGHRouting(ctx context.Context) []check {
	if _, err := os.Lstat(e.ghEntry()); err != nil {
		return nil
	}
	if _, ok := e.integrated(e.loginShell(ctx).Kind); !ok {
		return nil
	}
	found, err := e.LookPath("gh")
	if err == nil && paths.ResolveSymlinks(found) == paths.ResolveSymlinks(e.ghEntry()) {
		return []check{{ok: true, label: e.tr("gh in the shell that started gitpersona goes through GitPersona")}}
	}
	return []check{{warn: true,
		label:  e.tr("gh in the shell that started gitpersona is not GitPersona's (%s), so gh does not follow your directories there", e.display(found)),
		advice: e.tr("open a new terminal; if it persists, something later in your startup files puts another directory before %s", e.display(e.shimDir()))}}
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
		fmt.Fprintln(e.Stdout, e.tr("Also installed: %s. To set them up too: gitpersona install-shell --shell %s",
			e.setupList(others), strings.Join(all, ",")))
	}
}
