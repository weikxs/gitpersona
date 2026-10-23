package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/config"
)

func TestSavingASigningKeyThatDoesNotExistIsRefused(t *testing.T) {
	w := editWorld(t)
	env, _ := w.env(t, w.home)
	before := readConfig(t, w)
	missing := config.Signing{Method: config.SigningSSH, Key: "~/.ssh/work_signing.pub"}

	err := UpdateProfile(context.Background(), env, "acme", ProfileChanges{Signing: &missing})

	if err == nil || !strings.Contains(err.Error(), "signing key ~/.ssh/work_signing.pub does not exist") {
		t.Fatalf("UpdateProfile() with a missing signing key = %v", err)
	}
	if readConfig(t, w) != before {
		t.Fatal("a refused change was written")
	}

	mustWrite(t, filepath.Join(w.home, ".ssh", "work_signing.pub"), "PUBLIC KEY PLACEHOLDER\n", 0o600)
	if err := UpdateProfile(context.Background(), env, "acme", ProfileChanges{Signing: &missing}); err != nil {
		t.Fatalf("UpdateProfile() once the key exists: %v", err)
	}
	if err := UpdateProfile(context.Background(), env, "acme", ProfileChanges{SSHKey: ptr("~/.ssh/nope")}); err == nil ||
		!strings.Contains(err.Error(), "SSH key ~/.ssh/nope does not exist") {
		t.Fatalf("UpdateProfile() with a missing SSH key = %v", err)
	}
}

func TestDoctorReportsMissingKeysMissingDirectoriesAndOldRules(t *testing.T) {
	w := newWorld(t)
	withKey := strings.Replace(demoConfig, `email   = "1000002+acme-alex@users.noreply.github.com"`,
		`email   = "1000002+acme-alex@users.noreply.github.com"`+"\n"+`signing = { method = "ssh", key = "~/.ssh/acme.pub" }`, 1)
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"), withKey, 0o600)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[includeIf \"gitdir:~/old/\"]\n\tpath = ~/.gitconfig-old\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gitconfig-old"), "[user]\n\temail = old@example.com\n", 0o600)
	env, out := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	ok := Doctor(context.Background(), env)

	report := out.String()
	for _, want := range []string{
		"✗ profile acme: signing key ~/.ssh/acme.pub does not exist",
		"! profile globex: directory ~/Projects/clients/globex does not exist yet",
		"! ~/.gitconfig still has includeIf identity rules from before GitPersona (~/old)",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("doctor should report %q:\n%s", want, report)
		}
	}
	if ok {
		t.Error("a missing signing key breaks every commit, so doctor must fail")
	}
}

func TestDoctorIsQuietWhenKeysAndDirectoriesExist(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	Doctor(context.Background(), env)
	if strings.Contains(out.String(), "does not exist") && !strings.Contains(out.String(), "clients/globex") {
		t.Errorf("doctor reported a missing file that exists:\n%s", out.String())
	}
	if strings.Contains(out.String(), "includeIf identity rules") {
		t.Errorf("GitPersona's own include is not an old rule:\n%s", out.String())
	}
}

func TestAnEditThatChangesNothingMakesNoBackup(t *testing.T) {
	w := editWorld(t)
	env, out := w.env(t, w.home)
	if err := SetDefaultProfile(context.Background(), env, "personal"); err != nil {
		t.Fatal(err)
	}
	before, _ := backupCount(t, env)

	if err := SetDefaultProfile(context.Background(), env, "personal"); err != nil {
		t.Fatal(err)
	}

	if after, _ := backupCount(t, env); after != before {
		t.Errorf("an unchanged save made a backup (%d -> %d)", before, after)
	}
	if !strings.Contains(out.String(), "Everything is already up to date") {
		t.Errorf("an unchanged save should say so:\n%s", out.String())
	}
}

func backupCount(t *testing.T, env Env) (int, error) {
	t.Helper()
	entries, err := filepath.Glob(filepath.Join(env.BackupDir(), "*"))
	return len(entries), err
}

func TestOldRulesAreRemovedWithABackupAndOtherSettingsKept(t *testing.T) {
	w := newWorld(t)
	gitconfig := filepath.Join(w.home, ".gitconfig")
	mustWrite(t, gitconfig, "# my settings\n[user]\n\tname = Demo Person\n"+
		"[includeIf \"gitdir:~/old/\"]\n\tpath = ~/.gitconfig-old\n"+
		"[includeIf \"hasconfig:remote.*.url:https://example.com/**\"]\n\tpath = ~/.gitconfig-other\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gitconfig-old"), "[user]\n\temail = old@example.com\n", 0o600)
	env, out := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	before, _ := backupCount(t, env)

	if err := RemoveOldRules(context.Background(), env); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, gitconfig)
	if strings.Contains(got, "~/old/") {
		t.Errorf("the old identity rule is still there:\n%s", got)
	}
	for _, keep := range []string{"# my settings", "name = Demo Person", "hasconfig:remote", "gitpersona"} {
		if !strings.Contains(got, keep) {
			t.Errorf("%q should be kept:\n%s", keep, got)
		}
	}
	if after, _ := backupCount(t, env); after != before+1 {
		t.Errorf("removing old rules should make one backup (%d -> %d)", before, after)
	}
	if !strings.Contains(out.String(), "Removed the old includeIf rule for ~/old") {
		t.Errorf("output should say what was removed:\n%s", out.String())
	}
	out.Reset()
	if err := RemoveOldRules(context.Background(), env); err != nil || !strings.Contains(out.String(), "nothing to change") {
		t.Errorf("a second run should change nothing: %v\n%s", err, out.String())
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestTheGlobalIncludeIsWrittenRelativeToHome(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, filepath.Join(w.home, ".gitconfig"))

	if !strings.Contains(got, "path = ~/.config/gitpersona/rules.gitconfig") || strings.Contains(got, w.home) {
		t.Errorf("~/.gitconfig should include the rules as ~/..., so dotfiles work on any machine:\n%s", got)
	}
}

func TestOldRulesAreLeftInAReadOnlyGitConfig(t *testing.T) {
	w := newWorld(t)
	gitconfig := filepath.Join(w.home, ".gitconfig")
	mustWrite(t, filepath.Join(w.home, ".gitconfig-old"), "[user]\n\temail = old@example.com\n", 0o600)
	mustWrite(t, gitconfig, "[includeIf \"gitdir:~/old/\"]\n\tpath = ~/.gitconfig-old\n", 0o400)
	env, _ := w.env(t, w.home)

	err := RemoveOldRules(context.Background(), env)

	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("RemoveOldRules() on a read-only file = %v", err)
	}
	if !strings.Contains(readFile(t, gitconfig), "~/old/") {
		t.Error("a read-only file was changed")
	}
}
