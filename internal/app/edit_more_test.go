package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/config"
)

// blockBackups puts a file where the backup folder should be, so that any
// edit that has something to write fails before it writes.
func blockBackups(t *testing.T, w world) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(w.home, ".config", "gitpersona", "backups")); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "backups"), "not a folder", 0o600)
}

func TestUseSaysSoWhenTheDirectoryIsAlreadyARuleOfThatProfile(t *testing.T) {
	w := newWorld(t)
	dir := filepath.Join(w.home, "Projects", "acme")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	env, out := w.env(t, dir)
	before := readFile(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"))

	if err := Use(context.Background(), env, "acme"); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "~/Projects/acme already uses profile acme.") {
		t.Errorf("output = %q", out.String())
	}
	if readFile(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml")) != before {
		t.Error("the configuration changed although nothing had to")
	}
}

func TestUseKeepsTheOtherFoldersOfTheProfileItMovesADirectoryFrom(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, filepath.Join(w.home, "Projects", "personal"))

	if err := Use(context.Background(), env, "acme"); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(filepath.Join(w.home, ".config", "gitpersona", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	personal, _ := cfg.ProfileByID("personal")
	if len(personal.Dirs) != 1 || personal.Dirs[0] != "~/Projects/acme/oss" {
		t.Errorf("personal keeps %v, want only ~/Projects/acme/oss", personal.Dirs)
	}
}

func TestEditsStopWhenTheBackupFolderCannotBeWritten(t *testing.T) {
	ctx := context.Background()
	for name, run := range map[string]func(env Env) error{
		"remove":          func(env Env) error { return Remove(ctx, env, "~/Projects/personal") },
		"pin":             func(env Env) error { return Pin(ctx, env, "acme") },
		"add profile":     func(env Env) error { return AddProfile(ctx, env, demoProfile()) },
		"remove profile":  func(env Env) error { return RemoveProfile(ctx, env, "acme") },
		"set default":     func(env Env) error { return SetDefaultProfile(ctx, env, "personal") },
		"update profile":  func(env Env) error { return UpdateProfile(ctx, env, "acme", ProfileChanges{Name: ptr("Alex")}) },
		"use":             func(env Env) error { return Use(ctx, env, "globex") },
		"unpin (a repo)":  func(env Env) error { return Unpin(ctx, env) },
		"apply":           func(env Env) error { return Apply(ctx, env, ApplyOptions{}) },
		"old rules":       func(env Env) error { return RemoveOldRules(ctx, env) },
		"uninstall (yes)": func(env Env) error { return Uninstall(ctx, env, true) },
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			if name == "pin" || name == "unpin (a repo)" {
				mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"),
					readFile(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"))+"\n[[pin]]\nrepo = \"~/Desktop/scratch\"\nprofile = \"acme\"\n", 0o600)
			}
			if name == "old rules" {
				mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[includeIf \"gitdir:~/Projects/personal/\"]\n\tpath = ~/.gitconfig-personal\n", 0o600)
				mustWrite(t, filepath.Join(w.home, ".gitconfig-personal"), "[user]\n\temail = 1000001+Octo-Alex@users.noreply.github.com\n", 0o600)
			}
			if name == "uninstall (yes)" || name == "old rules" {
				env, _ := w.env(t, w.home)
				if err := Apply(ctx, env, ApplyOptions{}); err != nil {
					t.Fatal(err)
				}
			}
			env, _ := w.env(t, w.repos["scratch"])
			if name == "pin" {
				env.WorkDir = w.repos["blog"]
			}
			blockBackups(t, w)

			if err := run(env); err == nil {
				t.Errorf("%s with an unwritable backup folder succeeded, want an error", name)
			}
		})
	}
}

func demoProfile() config.Profile {
	return config.Profile{ID: "demo", Account: "octo-alex", Name: "Alex Demo", Email: "1000003+octo-alex@users.noreply.github.com", Dirs: []string{"~/Projects/demo"}}
}

func TestRemoveOfARelativePathReportsTheRuleIsMissing(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)

	err := Remove(context.Background(), env, "Projects/personal")

	if err == nil {
		t.Fatal("Remove of a relative path succeeded")
	}
}

func TestPinRefusesAnUnknownProfileAndUnpinRefusesWhatIsNotPinned(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	env, _ := w.env(t, w.repos["blog"])

	if err := Pin(ctx, env, "no-such-profile"); err == nil || !strings.Contains(err.Error(), "unknown profile") {
		t.Errorf("Pin(unknown) = %v", err)
	}
	if err := Unpin(ctx, env); err == nil || !strings.Contains(err.Error(), "not pinned") {
		t.Errorf("Unpin of an unpinned repository = %v", err)
	}
	env.WorkDir = w.home
	if err := Unpin(ctx, env); err == nil || !strings.Contains(err.Error(), "not inside a Git repository") {
		t.Errorf("Unpin outside a repository = %v", err)
	}
}

func TestProfileCommandsRefuseWhatCannotBeDone(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	env, _ := w.env(t, w.home)
	withMissingKey := demoProfile()
	withMissingKey.SSHKey = "~/.ssh/id_missing"
	duplicate := demoProfile()
	duplicate.ID = "acme"

	cases := map[string]error{
		"add with a missing key":    AddProfile(ctx, env, withMissingKey),
		"add a duplicate id":        AddProfile(ctx, env, duplicate),
		"update an unknown profile": UpdateProfile(ctx, env, "no-such-profile", ProfileChanges{Name: ptr("A")}),
		"set default to unknown":    SetDefaultProfile(ctx, env, "no-such-profile"),
		"remove an unknown profile": RemoveProfile(ctx, env, "no-such-profile"),
	}
	for name, err := range cases {
		if err == nil {
			t.Errorf("%s succeeded, want an error", name)
		}
	}
}

func TestUpdateProfileChangesTheAccountAndLeavesOtherProfilesAlone(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)

	if err := UpdateProfile(context.Background(), env, "personal", ProfileChanges{Account: ptr("octo-alex-two")}); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(filepath.Join(w.home, ".config", "gitpersona", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	personal, _ := cfg.ProfileByID("personal")
	acme, _ := cfg.ProfileByID("acme")
	if personal.Account != "octo-alex-two" || acme.Account != "acme-alex" {
		t.Errorf("accounts = %q and %q", personal.Account, acme.Account)
	}
}

func TestRemoveProfileKeepsThePinsOfOtherProfiles(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)

	if err := RemoveProfile(context.Background(), env, "personal"); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(filepath.Join(w.home, ".config", "gitpersona", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Pins) != 1 || cfg.Pins[0].Profile != "globex" {
		t.Errorf("pins = %+v, want the globex pin kept", cfg.Pins)
	}
}
