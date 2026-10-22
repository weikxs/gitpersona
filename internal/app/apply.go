package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/weikxs/gitpersona/internal/backup"
	"github.com/weikxs/gitpersona/internal/block"
	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/fsutil"
	"github.com/weikxs/gitpersona/internal/generate"
	"github.com/weikxs/gitpersona/internal/paths"
)

// change is one file write (or removal, when remove is set).
type change struct {
	path    string
	content string
	remove  bool
	create  bool        // the file does not exist yet
	perm    os.FileMode // 0 means fsutil.FilePerm
	linkTo  string      // make path a symbolic link to this target instead
}

// backupsToKeep bounds how many snapshots accumulate in the backup directory.
const backupsToKeep = 20

// ApplyOptions controls how Apply makes its changes.
type ApplyOptions struct {
	DryRun bool // show the changes without writing anything
	Yes    bool // allow removing files
	// SkipRemovals writes every other change and only reports removals.
	SkipRemovals bool
	// NoPrune keeps every backup.
	NoPrune bool
	// Command names the change in the backup, for `gitpersona rollback`.
	Command string
}

// Apply regenerates every file from the configuration. Only files whose
// content changes are backed up and written, so running it twice is a no-op.
// Removing files needs explicit consent.
func Apply(ctx context.Context, e Env, opts ApplyOptions) error {
	changes, err := planApply(ctx, e)
	if err != nil {
		return err
	}
	return e.applyChanges(changes, opts)
}

// applyChanges backs up every target in one snapshot, so a later rollback
// undoes the whole operation, and then writes the changes.
func (e Env) applyChanges(changes []change, opts ApplyOptions) error {
	if len(changes) == 0 {
		fmt.Fprintln(e.Stdout, "Everything is already up to date.")
		return nil
	}
	if opts.DryRun {
		for _, c := range changes {
			fmt.Fprintf(e.Stdout, "would %s %s\n", c.verb(), e.display(c.path))
		}
		fmt.Fprintln(e.Stdout, "Nothing was written (dry run).")
		return nil
	}
	if removals := e.removals(changes); len(removals) > 0 && !opts.Yes {
		msg := fmt.Sprintf("apply would remove %s; run `gitpersona apply --dry-run` to review, then `gitpersona apply --yes`",
			strings.Join(removals, ", "))
		if !opts.SkipRemovals {
			return errors.New(msg)
		}
		fmt.Fprintf(e.Stdout, "Not removing anything: %s\n", msg)
		changes = slices.DeleteFunc(slices.Clone(changes), func(c change) bool { return c.remove })
		if len(changes) == 0 {
			return nil
		}
	}

	targets := make([]string, len(changes))
	for i, c := range changes {
		targets[i] = c.path
	}
	snapshot, err := backup.SnapshotFor(e.backupDir(), e.Now(), cmp.Or(opts.Command, "apply"), targets)
	if err != nil {
		return err
	}
	for _, c := range changes {
		if err := c.write(); err != nil {
			return fmt.Errorf("%w (previous files are saved in %s)", err, snapshot)
		}
		fmt.Fprintf(e.Stdout, "%sd %s\n", c.verb(), e.display(c.path))
	}
	fmt.Fprintf(e.Stdout, "Backup saved in %s\n", e.display(snapshot))
	if opts.NoPrune {
		return nil
	}
	return backup.Prune(e.backupDir(), backupsToKeep)
}

func (e Env) removals(changes []change) []string {
	var names []string
	for _, c := range changes {
		if c.remove {
			names = append(names, e.display(c.path))
		}
	}
	return names
}

// planApply computes the changes without touching any file.
func planApply(ctx context.Context, e Env) ([]change, error) {
	cfg, err := e.loadConfig()
	if err != nil {
		return nil, err
	}
	return e.planWith(ctx, cfg)
}

// planWith computes the changes that generate the files for cfg.
func (e Env) planWith(ctx context.Context, cfg config.Config) ([]change, error) {
	opts, err := e.generateOptions(ctx)
	if err != nil {
		return nil, err
	}
	files, err := generate.Generate(cfg, opts)
	if err != nil {
		return nil, err
	}

	desired := make([]change, 0, len(files)+1)
	for _, f := range files {
		desired = append(desired, change{path: filepath.Join(e.ConfigDir, f.Name), content: f.Content})
	}
	gitconfig, err := e.gitConfigWithBlock()
	if err != nil {
		return nil, err
	}
	desired = append(desired, gitconfig)

	var changes []change
	if link, ok := e.ghEntryChange(cfg); ok {
		changes = append(changes, link)
	}
	for _, c := range desired {
		current, err := readOptional(c.path)
		if err != nil {
			return nil, err
		}
		if current != c.content {
			_, statErr := os.Stat(c.path)
			c.create = os.IsNotExist(statErr)
			changes = append(changes, c)
		}
	}
	return append(changes, e.staleProfiles(desired)...), nil
}

func (e Env) generateOptions(ctx context.Context) (generate.Options, error) {
	sshCommand, err := e.gitConfigAll(ctx, "core.sshCommand")
	if err != nil {
		return generate.Options{}, err
	}
	opts := generate.Options{
		Home:            e.Home,
		ConfigDir:       e.ConfigDir,
		Executable:      e.Executable,
		CaseInsensitive: e.CaseInsensitive,
		Canonicalize:    paths.ResolveSymlinks,
	}
	if len(sshCommand) > 0 {
		opts.FallbackSSHCommand = sshCommand[len(sshCommand)-1]
	}
	for key, field := range map[string]*string{
		"commit.gpgsign":  &opts.FallbackSigning.GPGSign,
		"user.signingkey": &opts.FallbackSigning.SigningKey,
		"gpg.format":      &opts.FallbackSigning.Format,
	} {
		values, err := e.gitConfigAll(ctx, key)
		if err != nil {
			return generate.Options{}, err
		}
		if len(values) > 0 {
			*field = values[len(values)-1]
		}
	}
	return opts, nil
}

// loadConfig reads the configuration and, when it is missing, says how to
// create it.
func (e Env) loadConfig() (config.Config, error) {
	cfg, err := config.Load(e.configFile())
	if errors.Is(err, config.ErrNotFound) {
		return cfg, fmt.Errorf("%w; create it as shown in the quick start: %s", err, quickStartURL)
	}
	return cfg, err
}

const quickStartURL = "https://github.com/weikxs/gitpersona#quick-start"

// resolvedUserGitConfig follows symbolic links, so dotfiles managers keep
// working when GitPersona writes the global Git configuration.
func (e Env) resolvedUserGitConfig() string {
	path := e.userGitConfig()
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	// A dangling link (the dotfiles target does not exist yet): follow it the
	// way Git does when it writes, instead of replacing the link.
	for range maxSymlinkHops {
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			return path
		}
		target, err := os.Readlink(path)
		if err != nil {
			return path
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		path = target
	}
	return path
}

// maxSymlinkHops bounds symlink chains, like the operating system does.
const maxSymlinkHops = 40

// errGlobalConfigDisabled explains GIT_CONFIG_GLOBAL=/dev/null.
var errGlobalConfigDisabled = fmt.Errorf("the global Git configuration is disabled (GIT_CONFIG_GLOBAL=%s); "+
	"GitPersona works through it, so unset GIT_CONFIG_GLOBAL or point it to a file", os.DevNull)

// userWritablePerm is the owner-write bit; without it the user has marked the
// file read-only and GitPersona must not change it.
const userWritablePerm os.FileMode = 0o200

func (e Env) gitConfigWithBlock() (change, error) {
	link := e.userGitConfig()
	if link == os.DevNull {
		return change{}, errGlobalConfigDisabled
	}
	path := e.resolvedUserGitConfig()
	current, err := readOptional(path)
	if err != nil {
		return change{}, err
	}
	updated, err := block.UpsertLast(current, generate.IncludeBody(filepath.Join(e.ConfigDir, generate.RulesFile)))
	if err != nil {
		return change{}, fmt.Errorf("%s: %w. Open the file and keep exactly one pair of the lines %q and %q "+
			"(or delete both), then run `gitpersona apply` again", e.display(link), err, block.Begin, block.End)
	}

	perm := fsutil.FilePerm
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm() // the file belongs to the user; keep its mode
		if perm&userWritablePerm == 0 && updated != current {
			return change{}, fmt.Errorf("%s is read-only; make it writable (chmod u+w), or add these lines at its end "+
				"yourself (keep the marker lines so GitPersona recognizes them):\n\n%s",
				e.display(link), block.Render(generate.IncludeBody(filepath.Join(e.ConfigDir, generate.RulesFile))))
		}
	}
	return change{path: path, content: updated, perm: perm}, nil
}

// staleProfiles returns removals for generated profile files whose profile
// no longer exists.
func (e Env) staleProfiles(desired []change) []change {
	existing, _ := filepath.Glob(filepath.Join(e.ConfigDir, "profiles", "*.gitconfig"))
	var stale []change
	for _, path := range existing {
		if !slices.ContainsFunc(desired, func(c change) bool { return c.path == path }) {
			stale = append(stale, change{path: path, remove: true})
		}
	}
	return stale
}

func (c change) write() error {
	if c.remove || c.linkTo != "" {
		if err := os.Remove(c.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", c.path, err)
		}
	}
	if c.remove {
		return nil
	}
	if c.linkTo != "" {
		if err := os.MkdirAll(filepath.Dir(c.path), fsutil.DirPerm); err != nil {
			return err
		}
		if err := os.Symlink(c.linkTo, c.path); err != nil {
			return fmt.Errorf("link %s: %w", c.path, err)
		}
		return nil
	}
	perm := c.perm
	if perm == 0 {
		perm = fsutil.FilePerm
	}
	return fsutil.WriteAtomic(c.path, []byte(c.content), perm)
}

// verb is the change in the present tense; add "d" for the past tense.
func (c change) verb() string {
	switch {
	case c.remove:
		return "remove"
	case c.create:
		return "create"
	default:
		return "update"
	}
}

// display shortens paths under the home directory to ~/..., also when the
// path arrives with symbolic links resolved.
func (e Env) display(path string) string {
	for _, home := range []string{e.Home, paths.ResolveSymlinks(e.Home)} {
		if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.Join("~", rel)
		}
	}
	return path
}
