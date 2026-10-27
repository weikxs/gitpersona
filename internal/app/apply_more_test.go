package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func skipAsRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("file permissions do not stop root")
	}
}

func TestApplyReportsAShimFolderItCannotCreate(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	mustWrite(t, env.shimDir(), "a file where the folder should be", 0o600)

	err := Apply(context.Background(), env, ApplyOptions{})

	if err == nil {
		t.Error("Apply succeeded although the shim folder cannot be created")
	}
}

func TestApplyReportsAShimFolderItCannotWriteTo(t *testing.T) {
	skipAsRoot(t)
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	if err := os.MkdirAll(env.shimDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(env.shimDir(), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(env.shimDir(), 0o700) })

	err := Apply(context.Background(), env, ApplyOptions{})

	if err == nil || !strings.Contains(err.Error(), "link ") || !strings.Contains(err.Error(), "put back as it was (backup in ~/") {
		t.Errorf("Apply = %v, want the failed link, that the files were put back and where the backup is", err)
	}
}

func TestApplyReportsAGeneratedFileItCannotWrite(t *testing.T) {
	skipAsRoot(t)
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	ctx := context.Background()
	if err := Apply(ctx, env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	profiles := filepath.Join(w.home, ".config", "gitpersona", "profiles")
	if err := os.Chmod(profiles, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(profiles, 0o700) })
	cfgFile := filepath.Join(w.home, ".config", "gitpersona", "config.toml")
	mustWrite(t, cfgFile, strings.Replace(readFile(t, cfgFile), `name    = "Alex Octo"`, `name    = "Alex Changed"`, 1), 0o600)

	err := Apply(ctx, env, ApplyOptions{})

	if err == nil || !strings.Contains(err.Error(), "put back as it was (backup in ~/") {
		t.Errorf("Apply = %v", err)
	}
	if got := readFile(t, filepath.Join(w.home, ".config", "gitpersona", "profiles", "personal.gitconfig")); strings.Contains(got, "Alex Changed") {
		t.Error("the profile file changed although apply failed")
	}
}

func TestApplyReportsAGeneratedFileItCannotRead(t *testing.T) {
	skipAsRoot(t)
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	ctx := context.Background()
	if err := Apply(ctx, env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	generated := filepath.Join(w.home, ".config", "gitpersona", "profiles", "personal.gitconfig")
	if err := os.Chmod(generated, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(generated, 0o600) })

	if err := Apply(ctx, env, ApplyOptions{}); err == nil {
		t.Error("Apply succeeded although a generated file cannot be read")
	}
}

func TestAGitConfigLinkThatLoopsIsNotFollowedForever(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	link := filepath.Join(w.home, ".gitconfig")
	if err := os.Symlink(".gitconfig", link); err != nil {
		t.Fatal(err)
	}

	got := env.resolvedUserGitConfig()

	if got == "" {
		t.Error("resolvedUserGitConfig gave nothing for a looping link")
	}
}

func TestRemoveOldRulesTellsWhichRulesItKeptBecauseTheyDoMore(t *testing.T) {
	w := newWorld(t)
	writeRule(t, w, filepath.Join(w.home, ".gitconfig"), "gitdir:~/Projects/acme/", ".gitconfig-acme",
		"[user]\n\temail = 1000002+acme-alex@users.noreply.github.com\n\tname = Alex (Acme)\n[core]\n\tsshCommand = ssh -p 2222\n")
	writeRule(t, w, filepath.Join(w.home, ".gitconfig"), "gitdir:~/Projects/personal/", ".gitconfig-personal",
		"[user]\n\temail = 1000001+Octo-Alex@users.noreply.github.com\n")
	env, out := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	if err := RemoveOldRules(context.Background(), env); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "Kept the includeIf rule for ~/Projects/acme: ") {
		t.Errorf("output should name the kept rule:\n%s", out.String())
	}
	if !strings.Contains(readFile(t, filepath.Join(w.home, ".gitconfig")), "gitdir:~/Projects/acme/") {
		t.Error("the rule that does more than a profile was removed")
	}
}
