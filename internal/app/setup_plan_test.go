package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/backup"
)

// withoutConfig removes the demo configuration and writes a hand-made
// includeIf setup: one identity that belongs to octo-alex and one that no
// signed-in account claims.
func withoutConfig(t *testing.T, w world) {
	t.Helper()
	if err := os.Remove(filepath.Join(w.home, ".config", "gitpersona", "config.toml")); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), `[user]
	name = Global Name
[includeIf "gitdir:~/Projects/personal/"]
	path = ~/.gitconfig-personal
[includeIf "gitdir:~/Projects/clients/"]
	path = ~/.gitconfig-clients
`, 0o600)
	mustWrite(t, filepath.Join(w.home, ".gitconfig-personal"), `[user]
	name = Alex Personal
	email = alex.personal@example.com
[credential "https://github.com"]
	username = octo-alex
`, 0o600)
	mustWrite(t, filepath.Join(w.home, ".gitconfig-clients"), "[user]\n\tname = Alex\n\temail = alex@clients.example\n", 0o600)
}

func TestProposeSetupReusesAnExistingSetupAndReportsTheRest(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	env, _ := w.env(t, w.home)

	got, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	if got.Default != "octo-alex" || len(got.Profiles) != 2 {
		t.Fatalf("proposal = %+v, want two profiles with the active account as default", got)
	}
	octo, acme := got.Profiles[0], got.Profiles[1]
	if octo.Source != "existing" || octo.Email != "alex.personal@example.com" || octo.Name != "Alex Personal" ||
		len(octo.Dirs) != 1 || octo.Dirs[0] != "~/Projects/personal" {
		t.Errorf("octo-alex should reuse the existing identity, got %+v", octo)
	}
	if acme.Source != "gh" || acme.Email != "1000002+acme-alex@users.noreply.github.com" || acme.Name != "acme-alex" {
		t.Errorf("acme-alex should come from GitHub, got %+v", acme)
	}
	if len(got.Unmatched) != 1 || got.Unmatched[0].Email != "alex@clients.example" ||
		got.Unmatched[0].Source != "~/.gitconfig-clients" {
		t.Errorf("the unclaimed identity should be reported with a ~/ path, got %+v", got.Unmatched)
	}
}

func TestProposeSetupMatchesByNoreplyEmail(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	mustWrite(t, filepath.Join(w.home, ".gitconfig-clients"),
		"[user]\n\tname = Acme Alex\n\temail = 1000002+Acme-Alex@users.noreply.github.com\n", 0o600)
	env, _ := w.env(t, w.home)

	got, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	if acme := got.Profiles[1]; acme.Source != "existing" || acme.Dirs[0] != "~/Projects/clients" {
		t.Errorf("an identity with the account's noreply email belongs to it, got %+v", acme)
	}
}

func TestProposeSetupWithoutSignedInAccountsExplains(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	t.Setenv("FAKE_GH_ACCOUNTS", "")
	env, _ := w.env(t, w.home)

	if _, err := ProposeSetup(context.Background(), env); err == nil || !strings.Contains(err.Error(), "gh auth login") {
		t.Fatalf("ProposeSetup() without accounts = %v", err)
	}
}

var demoPlan = SetupPlan{Default: "personal", Profiles: []SetupProfile{
	{ID: "personal", Account: "octo-alex", Name: "Alex Octo", Email: "1000001+octo-alex@users.noreply.github.com",
		Dirs: []string{"~/Projects/personal"}},
	{ID: "acme", Account: "acme-alex", Name: "Alex (Acme)", Email: "1000002+acme-alex@users.noreply.github.com",
		Dirs: []string{"~/Projects/acme"}},
}}

func TestPreviewSetupWritesNothingThenApplySetupWritesEverythingInOneBackup(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	env, _ := w.env(t, w.home)
	gitconfigBefore, _ := os.ReadFile(filepath.Join(w.home, ".gitconfig"))

	preview, err := PreviewSetup(context.Background(), env, demoPlan)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview) < 3 || preview[0].Path != "~/.config/gitpersona/config.toml" || preview[0].Action != "create" ||
		!strings.Contains(preview[0].Diff, `+id      = "personal"`) {
		t.Fatalf("preview should start with the new config.toml, got %+v", preview)
	}
	if _, err := os.Stat(filepath.Join(w.home, ".config", "gitpersona", "config.toml")); !os.IsNotExist(err) {
		t.Fatal("preview wrote config.toml")
	}
	if after, _ := os.ReadFile(filepath.Join(w.home, ".gitconfig")); string(after) != string(gitconfigBefore) {
		t.Fatal("preview changed ~/.gitconfig")
	}

	if err := ApplySetup(context.Background(), env, demoPlan); err != nil {
		t.Fatal(err)
	}
	if got := runGit(t, w.repos["billing"], "config", "user.email"); got != "1000002+acme-alex@users.noreply.github.com" {
		t.Errorf("billing uses %q after setup", got)
	}
	if got := runGit(t, w.repos["scratch"], "config", "user.email"); got != "1000001+octo-alex@users.noreply.github.com" {
		t.Errorf("the default profile should apply outside every rule, got %q", got)
	}
	infos, err := backup.List(env.BackupDir())
	if err != nil || len(infos) != 1 || infos[0].Manifest.Command != "setup (web)" {
		t.Fatalf("want one backup labelled setup (web), got %+v (%v)", infos, err)
	}
	if !strings.Contains(readConfigFile(t, w), "# GitPersona configuration, created on the setup page.") {
		t.Error("config.toml should say where it came from")
	}
}

func TestSetupPlanIsCheckedBeforeAnythingIsWritten(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	if _, err := PreviewSetup(context.Background(), env, demoPlan); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("setup over an existing config = %v", err)
	}

	withoutConfig(t, w)
	badEmail := SetupPlan{Profiles: []SetupProfile{{ID: "x", Account: "octo-alex", Name: "X", Email: "not an email"}}}
	newline := SetupPlan{Profiles: []SetupProfile{{ID: "x", Account: "octo-alex", Name: "X\n[core]", Email: "x@example.com"}}}
	unknownDefault := SetupPlan{Default: "nobody", Profiles: demoPlan.Profiles}
	for name, tc := range map[string]struct {
		plan SetupPlan
		want string
	}{
		"no profiles":     {SetupPlan{}, "at least one profile"},
		"bad email":       {badEmail, "email"},
		"control char":    {newline, "control"},
		"unknown default": {unknownDefault, "nobody"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ApplySetup(context.Background(), env, tc.plan); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ApplySetup() = %v, want an error about %q", err, tc.want)
			}
			if _, err := os.Stat(filepath.Join(w.home, ".config", "gitpersona", "config.toml")); !os.IsNotExist(err) {
				t.Fatal("a rejected plan wrote config.toml")
			}
		})
	}
}

func TestProposeSetupUsesThePublicEmailAndMatchesByIt(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	// The hand-made identity uses octo-alex's public email but names no account.
	mustWrite(t, filepath.Join(w.home, ".gitconfig-personal"), "[user]\n\tname = Alex\n\temail = Alex@Octo.example\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gitconfig-clients"), "[user]\n\tname = Other\n\temail = other@clients.example\n", 0o600)
	env, _ := w.env(t, w.home)

	got, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	if octo := got.Profiles[0]; octo.Source != "existing" || octo.Dirs[0] != "~/Projects/personal" {
		t.Errorf("an identity with the account's public email belongs to it, got %+v", octo)
	}
	if acme := got.Profiles[1]; acme.EmailSource != "noreply" || acme.Email != "1000002+acme-alex@users.noreply.github.com" {
		t.Errorf("an account without a public email gets its noreply address, got %+v", acme)
	}

	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "", 0o600)
	got, err = ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	if octo := got.Profiles[0]; octo.Source != "gh" || octo.EmailSource != "public" || octo.Email != "alex@octo.example" {
		t.Errorf("without an existing setup the public email is proposed, got %+v", octo)
	}
}

func TestProposeSetupReportsTheGlobalIdentityAndWhereNamesComeFrom(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[user]\n\tname = Demo Person\n\temail = person@demo.example\n", 0o600)
	env, _ := w.env(t, w.home)

	got, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	if got.Global.Name != "Demo Person" || got.Global.Email != "person@demo.example" {
		t.Errorf("global identity = %+v, want the one in ~/.gitconfig", got.Global)
	}
	if acme := got.Profiles[1]; acme.NameSource != "login" {
		t.Errorf("a name taken from the login should say so, got %+v", acme)
	}
}
