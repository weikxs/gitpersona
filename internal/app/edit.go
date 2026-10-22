package app

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/fsutil"
	"github.com/weikxs/gitpersona/internal/paths"
	"github.com/weikxs/gitpersona/internal/resolve"
)

// Use assigns the current directory to a profile, moving it from another
// profile if needed, and applies the change.
func Use(ctx context.Context, e Env, profileID string) error {
	text, cfg, err := e.readConfigText()
	if err != nil {
		return err
	}
	if err := knownProfile(cfg, profileID, "use"); err != nil {
		return err
	}
	dir := e.display(e.WorkDir)
	opts := e.editOptions()
	if r := e.resolveDir(cfg); r.Found && r.Reason == resolve.ReasonRule && r.Profile.ID == profileID && !samePath(r.Match, dir, opts) {
		fmt.Fprintf(e.Stdout, "%s already uses profile %s through %s; nothing to change.\n", dir, profileID, r.Match)
		return nil
	}

	edited, want, from := text, cfg, ""
	if removed, owner, err := config.RemoveDir(text, dir, opts); err == nil {
		if owner == profileID {
			fmt.Fprintf(e.Stdout, "%s already uses profile %s.\n", dir, profileID)
			return nil
		}
		edited, want, from = removed, withoutDir(cfg, owner, dir, opts), owner
	}
	edited, err = config.AddDir(edited, profileID, dir, opts)
	if err != nil {
		return err
	}
	want = withDir(want, profileID, dir)
	if err := e.saveEdited(ctx, "use "+profileID, edited, want); err != nil {
		return err
	}
	if from != "" {
		fmt.Fprintf(e.Stdout, "%s now uses profile %s (moved from %s).\n", dir, profileID, from)
	} else {
		fmt.Fprintf(e.Stdout, "%s now uses profile %s.\n", dir, profileID)
	}
	return nil
}

// Remove unassigns a directory (the current one when dir is empty).
func Remove(ctx context.Context, e Env, dir string) error {
	text, cfg, err := e.readConfigText()
	if err != nil {
		return err
	}
	if dir == "" {
		dir = e.display(e.WorkDir)
	}
	opts := e.editOptions()
	edited, owner, err := config.RemoveDir(text, dir, opts)
	if err != nil {
		if r := e.resolveDirAt(cfg, dir); r.Found && r.Reason == resolve.ReasonRule {
			return fmt.Errorf("%s has no rule of its own; it uses profile %s through %s (run `gitpersona remove %s` to remove that rule)",
				dir, r.Profile.ID, r.Match, r.Match)
		}
		return err
	}
	if err := e.saveEdited(ctx, "remove "+dir, edited, withoutDir(cfg, owner, dir, opts)); err != nil {
		return err
	}
	fmt.Fprintf(e.Stdout, "%s no longer uses profile %s.\n", dir, owner)
	return nil
}

// Pin binds the current repository to a profile.
func Pin(ctx context.Context, e Env, profileID string) error {
	text, cfg, err := e.readConfigText()
	if err != nil {
		return err
	}
	if err := knownProfile(cfg, profileID, "pin"); err != nil {
		return err
	}
	repo, err := e.pinnableRepo(ctx)
	if err != nil {
		return err
	}
	opts := e.editOptions()
	edited, err := config.AddPin(text, repo, profileID, opts)
	if err != nil {
		return err
	}
	if err := e.saveEdited(ctx, "pin "+profileID, edited, withPin(cfg, repo, profileID, opts)); err != nil {
		return err
	}
	fmt.Fprintf(e.Stdout, "%s is pinned to profile %s.\n", repo, profileID)
	return nil
}

// Unpin removes the pin of the current repository.
func Unpin(ctx context.Context, e Env) error {
	text, cfg, err := e.readConfigText()
	if err != nil {
		return err
	}
	repo, err := e.pinnableRepo(ctx)
	if err != nil {
		return err
	}
	opts := e.editOptions()
	edited, err := config.RemovePin(text, repo, opts)
	if err != nil {
		return err
	}
	if err := e.saveEdited(ctx, "unpin", edited, withoutPin(cfg, repo, opts)); err != nil {
		return err
	}
	fmt.Fprintf(e.Stdout, "%s is no longer pinned.\n", repo)
	return nil
}

// pinnableRepo returns the current repository as written in the
// configuration. Worktrees follow their main repository, so they cannot be
// pinned on their own.
func (e Env) pinnableRepo(ctx context.Context) (string, error) {
	root, ok := e.repoRoot(ctx, e.WorkDir)
	if !ok {
		return "", fmt.Errorf("%s is not inside a Git repository", e.display(e.WorkDir))
	}
	gitDir, _ := e.git(ctx, root, "rev-parse", "--absolute-git-dir")
	common, _ := e.git(ctx, root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if gitDir != common {
		return "", fmt.Errorf("%s is a worktree; worktrees follow their main repository (%s), so pin that instead",
			e.display(root), e.display(strings.TrimSuffix(common, "/.git")))
	}
	return e.display(root), nil
}

// saveEdited checks that edited parses to exactly want, backs up the
// configuration, writes the edit and applies it. The check guards against
// text edits that an unusual layout could make go wrong.
func (e Env) saveEdited(ctx context.Context, command, edited string, want config.Config) error {
	got, err := config.Parse([]byte(edited))
	if err != nil || !reflect.DeepEqual(got, want) {
		return fmt.Errorf("could not change %s safely; edit it by hand (nothing was written)", e.display(e.configFile()))
	}
	if err := got.Validate(e.Home, e.CaseInsensitive); err != nil {
		return err
	}
	generated, err := e.planWith(ctx, got)
	if err != nil {
		return err
	}
	perm := fsutil.FilePerm
	if info, err := os.Stat(e.configFile()); err == nil {
		perm = info.Mode().Perm()
	}
	// One snapshot covers the configuration and the generated files, so a
	// rollback undoes the whole edit.
	// An edit that changes nothing writes nothing and makes no backup.
	changes := generated
	if current, err := os.ReadFile(e.configFile()); err != nil || string(current) != edited {
		changes = append([]change{{path: e.configFile(), content: edited, perm: perm}}, generated...)
	}
	return e.applyChanges(changes, ApplyOptions{Yes: true, Command: command})
}

func (e Env) readConfigText() (string, config.Config, error) {
	cfg, err := e.loadConfig()
	if err != nil {
		return "", cfg, err
	}
	data, err := os.ReadFile(e.configFile())
	if err != nil {
		return "", cfg, err
	}
	return string(data), cfg, nil
}

func (e Env) editOptions() config.EditOptions {
	return config.EditOptions{Home: e.Home, CaseInsensitive: e.CaseInsensitive}
}

// knownProfile explains a missing or unknown profile argument.
func knownProfile(cfg config.Config, id, command string) error {
	if _, ok := cfg.ProfileByID(id); ok {
		return nil
	}
	if id == "" {
		return fmt.Errorf("choose a profile: gitpersona %s <%s>", command, profileIDs(cfg))
	}
	return fmt.Errorf("unknown profile %q; choose one of: %s", id, profileIDs(cfg))
}

// resolveDir resolves the current directory by directory rules.
func (e Env) resolveDir(cfg config.Config) resolve.Result {
	return e.resolveDirAt(cfg, e.WorkDir)
}

func (e Env) resolveDirAt(cfg config.Config, dir string) resolve.Result {
	norm, err := paths.Normalize(dir, e.Home)
	if err != nil {
		return resolve.Result{}
	}
	return resolve.Resolve(cfg, paths.ResolveSymlinks(norm), resolve.Options{
		Home: e.Home, CaseInsensitive: e.CaseInsensitive, Canonicalize: paths.ResolveSymlinks,
	})
}

func profileIDs(cfg config.Config) string {
	ids := make([]string, len(cfg.Profiles))
	for i, p := range cfg.Profiles {
		ids[i] = p.ID
	}
	return strings.Join(ids, ", ")
}

// The functions below compute the intended configuration of an edit.

func withDir(cfg config.Config, id, dir string) config.Config {
	out := cfg
	out.Profiles = append([]config.Profile{}, cfg.Profiles...)
	for i, p := range out.Profiles {
		if p.ID == id {
			out.Profiles[i].Dirs = append(append([]string{}, p.Dirs...), dir)
		}
	}
	return out
}

func withoutDir(cfg config.Config, id, dir string, o config.EditOptions) config.Config {
	out := cfg
	out.Profiles = append([]config.Profile{}, cfg.Profiles...)
	for i, p := range out.Profiles {
		if p.ID != id {
			continue
		}
		var kept []string
		for _, d := range p.Dirs {
			if !samePath(d, dir, o) {
				kept = append(kept, d)
			}
		}
		out.Profiles[i].Dirs = kept
	}
	return out
}

func withPin(cfg config.Config, repo, id string, o config.EditOptions) config.Config {
	out := withoutPin(cfg, repo, o)
	for i, p := range cfg.Pins {
		if samePath(p.Repo, repo, o) {
			out.Pins = append([]config.Pin{}, cfg.Pins...)
			out.Pins[i].Profile = id
			return out
		}
	}
	out.Pins = append(append([]config.Pin{}, cfg.Pins...), config.Pin{Repo: repo, Profile: id})
	return out
}

func withoutPin(cfg config.Config, repo string, o config.EditOptions) config.Config {
	out := cfg
	out.Pins = nil
	for _, p := range cfg.Pins {
		if !samePath(p.Repo, repo, o) {
			out.Pins = append(out.Pins, p)
		}
	}
	return out
}

func samePath(a, b string, o config.EditOptions) bool {
	na, errA := paths.Normalize(a, o.Home)
	nb, errB := paths.Normalize(b, o.Home)
	return errA == nil && errB == nil && paths.Same(na, nb, o.CaseInsensitive)
}
