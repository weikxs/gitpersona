package app

import (
	"context"
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
	perm    os.FileMode // 0 means fsutil.FilePerm
}

// Apply regenerates every file from the configuration. Only files whose
// content changes are backed up and written, so running it twice is a no-op.
func Apply(ctx context.Context, e Env) error {
	changes, err := planApply(ctx, e)
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		fmt.Fprintln(e.Stdout, "Everything is already up to date.")
		return nil
	}

	targets := make([]string, len(changes))
	for i, c := range changes {
		targets[i] = c.path
	}
	snapshot, err := backup.Snapshot(e.backupDir(), e.Now(), targets)
	if err != nil {
		return err
	}
	for _, c := range changes {
		if err := c.write(); err != nil {
			return fmt.Errorf("%w (previous files are saved in %s)", err, snapshot)
		}
		fmt.Fprintf(e.Stdout, "%s %s\n", c.verb(), e.display(c.path))
	}
	fmt.Fprintf(e.Stdout, "Backup saved in %s\n", e.display(snapshot))
	return nil
}

// planApply computes the changes without touching any file.
func planApply(ctx context.Context, e Env) ([]change, error) {
	cfg, err := config.Load(e.configFile())
	if err != nil {
		return nil, err
	}
	files, err := generate.Generate(cfg, e.generateOptions(ctx, cfg))
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
	for _, c := range desired {
		current, err := readOptional(c.path)
		if err != nil {
			return nil, err
		}
		if current != c.content {
			changes = append(changes, c)
		}
	}
	return append(changes, e.staleProfiles(desired)...), nil
}

func (e Env) generateOptions(ctx context.Context, cfg config.Config) generate.Options {
	sshCommand := e.gitConfigAll(ctx, "core.sshCommand")
	opts := generate.Options{
		Home:            e.Home,
		ConfigDir:       e.ConfigDir,
		Executable:      e.Executable,
		CaseInsensitive: e.CaseInsensitive,
		FallbackHelpers: map[string][]string{},
		Canonicalize:    paths.ResolveSymlinks,
	}
	if len(sshCommand) > 0 {
		opts.FallbackSSHCommand = sshCommand[len(sshCommand)-1]
	}
	generic := e.gitConfigAll(ctx, "credential.helper")
	for _, p := range cfg.Profiles {
		url := "https://" + p.Host
		if _, done := opts.FallbackHelpers[url]; done {
			continue
		}
		helpers := append(slices.Clone(generic), e.gitConfigAll(ctx, "credential."+url+".helper")...)
		opts.FallbackHelpers[url] = userHelpers(helpers)
	}
	return opts
}

// userHelpers drops list resets and any GitPersona helper, keeping order.
func userHelpers(helpers []string) []string {
	kept := []string{}
	for _, h := range helpers {
		if h != "" && !strings.Contains(h, " credential --provider ") {
			kept = append(kept, h)
		}
	}
	return kept
}

func (e Env) gitConfigWithBlock() (change, error) {
	path := e.userGitConfig()
	current, err := readOptional(path)
	if err != nil {
		return change{}, err
	}
	updated, err := block.Upsert(current, generate.IncludeBody(filepath.Join(e.ConfigDir, generate.RulesFile)))
	if err != nil {
		return change{}, fmt.Errorf("%s: %w", path, err)
	}
	perm := fsutil.FilePerm
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm() // the file belongs to the user; keep its mode
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
	if c.remove {
		if err := os.Remove(c.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", c.path, err)
		}
		return nil
	}
	perm := c.perm
	if perm == 0 {
		perm = fsutil.FilePerm
	}
	return fsutil.WriteAtomic(c.path, []byte(c.content), perm)
}

func (c change) verb() string {
	if c.remove {
		return "removed"
	}
	return "updated"
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
