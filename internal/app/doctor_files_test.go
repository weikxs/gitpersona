package app

import (
	"context"
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
