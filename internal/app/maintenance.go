package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/weikxs/gitpersona/internal/backup"
	"github.com/weikxs/gitpersona/internal/block"
	"github.com/weikxs/gitpersona/internal/generate"
)

// Rollback lists the backups (when which is empty) or restores one ("latest"
// or a snapshot name). Restoring needs yes, and the current state is backed
// up first so the rollback itself can be undone.
func Rollback(_ context.Context, e Env, which string, yes bool) error {
	infos, err := backup.List(e.backupDir())
	if err != nil {
		return err
	}
	if len(infos) == 0 {
		fmt.Fprintln(e.Stdout, "No backups yet.")
		return nil
	}
	if which == "" {
		for i, info := range infos {
			name := filepath.Base(info.Dir)
			if i == 0 {
				name += " (latest)"
			}
			fmt.Fprintf(e.Stdout, "%s  %s\n", name, strings.Join(e.entryNames(info.Manifest), ", "))
		}
		fmt.Fprintln(e.Stdout, "\nRestore one with: gitpersona rollback <name|latest> --yes")
		return nil
	}

	chosen, err := pickSnapshot(infos, which)
	if err != nil {
		return err
	}
	names := e.entryNames(chosen.Manifest)
	if !yes {
		return fmt.Errorf("rollback would restore %s; run `gitpersona rollback %s --yes`", strings.Join(names, ", "), which)
	}
	paths := make([]string, len(chosen.Manifest.Entries))
	for i, en := range chosen.Manifest.Entries {
		paths[i] = en.Path
	}
	undo, err := backup.Snapshot(e.backupDir(), e.Now(), paths)
	if err != nil {
		return err
	}
	if err := backup.Restore(chosen.Dir); err != nil {
		return err
	}
	fmt.Fprintf(e.Stdout, "Restored %s from %s.\nThe state before the rollback is saved in %s.\n",
		strings.Join(names, ", "), filepath.Base(chosen.Dir), e.display(undo))
	return nil
}

func pickSnapshot(infos []backup.Info, which string) (backup.Info, error) {
	if which == "latest" {
		return infos[0], nil
	}
	for _, info := range infos {
		if filepath.Base(info.Dir) == which {
			return info, nil
		}
	}
	return backup.Info{}, fmt.Errorf("no backup named %q; run `gitpersona rollback` to list them", which)
}

func (e Env) entryNames(m backup.Manifest) []string {
	names := make([]string, len(m.Entries))
	for i, en := range m.Entries {
		names[i] = e.display(en.Path)
	}
	return names
}

// Uninstall removes the GitPersona block from the global Git configuration
// and deletes the generated files. config.toml and the backups are kept.
func Uninstall(_ context.Context, e Env, yes bool) error {
	changes, err := e.planUninstall()
	if err != nil {
		return err
	}
	if !yes {
		names := make([]string, len(changes))
		for i, c := range changes {
			names[i] = e.display(c.path)
		}
		return fmt.Errorf("uninstall would change %s; run `gitpersona uninstall --yes`", strings.Join(names, ", "))
	}
	if err := e.applyChanges(changes, ApplyOptions{Yes: true}); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(e.ConfigDir, "profiles")) // only succeeds when empty
	_ = os.Remove(e.shimDir())
	fmt.Fprintf(e.Stdout, "GitPersona is uninstalled. Your configuration stays in %s.\n", e.display(e.configFile()))
	return nil
}

func (e Env) planUninstall() ([]change, error) {
	changes, err := e.removeShellBlocks()
	if err != nil {
		return nil, err
	}
	gitconfig := e.resolvedUserGitConfig()
	current, err := readOptional(gitconfig)
	if err != nil {
		return nil, err
	}
	if block.Contains(current) {
		without, err := block.Remove(current)
		if err != nil {
			return nil, err
		}
		perm := os.FileMode(0)
		if info, err := os.Stat(gitconfig); err == nil {
			perm = info.Mode().Perm()
		}
		// A file that only held the GitPersona block was created by it.
		changes = append(changes, change{path: gitconfig, content: without, perm: perm, remove: strings.TrimSpace(without) == ""})
	}
	if _, err := os.Lstat(e.ghEntry()); err == nil {
		changes = append(changes, change{path: e.ghEntry(), remove: true})
	}
	generated, _ := filepath.Glob(filepath.Join(e.ConfigDir, "profiles", "*.gitconfig"))
	generated = append(generated, filepath.Join(e.ConfigDir, generate.RulesFile))
	for _, path := range generated {
		if _, err := os.Stat(path); err == nil {
			changes = append(changes, change{path: path, remove: true})
		}
	}
	return changes, nil
}
