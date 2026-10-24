package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeRule adds an includeIf rule for condition to file, with the included
// file holding body.
func writeRule(t *testing.T, w world, file, condition, includedName, body string) {
	t.Helper()
	current, _ := os.ReadFile(file)
	mustWrite(t, file, string(current)+"[includeIf \""+condition+"\"]\n\tpath = ~/"+includedName+"\n", 0o600)
	mustWrite(t, filepath.Join(w.home, includedName), body, 0o600)
}

func TestDoctorFixSaysWhenItCouldNotFixSomething(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.home)
	blockBackups(t, w)

	DoctorWith(context.Background(), env, DoctorOptions{Fix: true})

	if !strings.Contains(out.String(), "could not fix it:") {
		t.Errorf("doctor --fix should report the failed fix:\n%s", out.String())
	}
}

func TestDoctorJSONOffersTheOldRulesFixOnTheCheckThatHasIt(t *testing.T) {
	w := newWorld(t)
	writeRule(t, w, filepath.Join(w.home, ".gitconfig"), "gitdir:~/Projects/personal/", ".gitconfig-personal",
		"[user]\n\temail = 1000001+Octo-Alex@users.noreply.github.com\n")
	env, out := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	DoctorWith(context.Background(), env, DoctorOptions{JSON: true})

	var report DoctorReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	found := false
	for _, c := range report.Checks {
		found = found || c.Fix == FixOldRules
	}
	if !found {
		t.Errorf("the check about taken-over rules should name its fix: %+v", report.Checks)
	}
}

func TestDoctorReportsAnUnreadableGitConfig(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("file permissions do not stop root")
	}
	w := newWorld(t)
	env, out := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	gitconfig := filepath.Join(w.home, ".gitconfig")
	if err := os.Chmod(gitconfig, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(gitconfig, 0o600) })
	out.Reset()

	if ok := Doctor(context.Background(), env); ok {
		t.Errorf("doctor passed with an unreadable ~/.gitconfig:\n%s", out.String())
	}
}

func TestDoctorWarnsWhenARuleNeverAppliesBecauseALaterGlobalIdentityComesAfterIt(t *testing.T) {
	w := newWorld(t)
	// Git reads ~/.config/git/config first, so the identity in ~/.gitconfig wins in ~/oss.
	mustWrite(t, filepath.Join(w.home, ".config", "git", "config"), "[includeIf \"gitdir:~/oss/\"]\n\tpath = ~/.gc-rule\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[user]\n\tname = Global Me\n\temail = global@demo.example\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gc-rule"), "[user]\n\tname = Rule Me\n\temail = rule@demo.example\n", 0o600)
	cfgFile := filepath.Join(w.home, ".config", "gitpersona", "config.toml")
	mustWrite(t, cfgFile, strings.Replace(readFile(t, cfgFile), "default = \"acme\"\n", "", 1), 0o600)
	env, out := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	Doctor(context.Background(), env)

	if !strings.Contains(out.String(), "your includeIf rule for ~/oss (Rule Me <rule@demo.example>) never applies") {
		t.Errorf("doctor should say the rule never applies:\n%s", out.String())
	}
}

func TestDoctorWarnsWhenAProfileCoversARuleThatDoesMoreThanAProfile(t *testing.T) {
	w := newWorld(t)
	writeRule(t, w, filepath.Join(w.home, ".gitconfig"), "gitdir:~/Projects/acme/", ".gitconfig-acme",
		"[user]\n\temail = 1000002+acme-alex@users.noreply.github.com\n\tname = Alex (Acme)\n[core]\n\tsshCommand = ssh -p 2222\n")
	env, out := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	Doctor(context.Background(), env)

	if !strings.Contains(out.String(), "covers your includeIf rule for ~/Projects/acme, which does more than a profile can") {
		t.Errorf("doctor should warn about what the profile loses:\n%s", out.String())
	}
}

func TestDoctorSaysWhichProfileFoldersLieInsideAnotherIdentitysRule(t *testing.T) {
	w := newWorld(t)
	cfgFile := filepath.Join(w.home, ".config", "gitpersona", "config.toml")
	mustWrite(t, cfgFile, strings.Replace(readFile(t, cfgFile), "default = \"acme\"\n", "", 1), 0o600)
	writeRule(t, w, filepath.Join(w.home, ".gitconfig"), "gitdir:~/Projects/", ".gitconfig-projects",
		"[user]\n\temail = projects@demo.example\n\tname = Projects Me\n")
	env, out := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	Doctor(context.Background(), env)

	if !strings.Contains(out.String(), "your includeIf rule for ~/Projects applies there (Projects Me <projects@demo.example>), except in ") ||
		!strings.Contains(out.String(), "which you gave to profiles") {
		t.Errorf("doctor should list the folders given to profiles:\n%s", out.String())
	}
}

func TestDoctorWarnsThatTheDefaultProfileOverridesARuleItCannotReplace(t *testing.T) {
	w := newWorld(t)
	writeRule(t, w, filepath.Join(w.home, ".gitconfig"), "hasconfig:remote.*.url:https://git.demo.example/**", ".gitconfig-remote",
		"[user]\n\temail = remote@demo.example\n\tname = Remote Me\n")
	env, out := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	Doctor(context.Background(), env)

	want := "the default profile acme overrides your includeIf rule"
	if !gitHasRemoteConditions(t) {
		want = "never applies: Git" // older Git ignores the rule, which says more than an override
	}
	if !strings.Contains(out.String(), want) {
		t.Errorf("doctor should say %q:\n%s", want, out.String())
	}
}

func TestKeyOnlyRulesOfOtherGlobalFilesNameTheFileAndSkipRulesWithNothingToKeep(t *testing.T) {
	w := newWorld(t)
	writeRule(t, w, filepath.Join(w.home, ".config", "git", "config"), "gitdir:~/k1/", ".gc-k1", "[core]\n\tsshCommand = ssh -p 2222\n")
	writeRule(t, w, filepath.Join(w.home, ".gitconfig"), "gitdir:~/k2/", ".gc-k2", "[pull]\n\trebase = true\n")
	env, _ := w.env(t, w.home)

	rules := env.keyOnlyRules(context.Background())

	if len(rules) != 1 || rules[0].Dir != "~/k1" || rules[0].File != "~/.config/git/config" || !strings.Contains(rules[0].Keep, "core.sshcommand") {
		t.Errorf("keyOnlyRules = %+v, want only the rule of ~/k1, from the XDG file", rules)
	}
}

func TestOneIdentityInTwoGlobalFilesIsOneIdentityWithAllItsFolders(t *testing.T) {
	w := newWorld(t)
	body := "[user]\n\temail = same@demo.example\n\tname = Same Me\n"
	writeRule(t, w, filepath.Join(w.home, ".config", "git", "config"), "gitdir:~/a/", ".gc-same", body)
	writeRule(t, w, filepath.Join(w.home, ".gitconfig"), "gitdir:~/b/", ".gc-same", body)
	writeRule(t, w, filepath.Join(w.home, ".gitconfig"), "gitdir:~/a/", ".gc-same", body)
	env, _ := w.env(t, w.home)

	ids := env.ExistingIdentities(context.Background())

	if len(ids) != 1 || len(ids[0].Dirs) != 2 {
		t.Errorf("identities = %+v, want one with ~/a and ~/b", ids)
	}
}

func TestAnIncludeOfAnUnreadableFileIsSkippedAndOthersStillCount(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("file permissions do not stop root")
	}
	w := newWorld(t)
	unreadable := filepath.Join(w.home, ".gc-unreadable")
	mustWrite(t, unreadable, "[user]\n\tname = Hidden\n", 0o600)
	if err := os.Chmod(unreadable, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o600) })
	mustWrite(t, filepath.Join(w.home, ".gc-readable"), "[user]\n\tname = Seen\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[include]\n\tpath = ~/.gc-unreadable\n\tpath = ~/.gc-readable\n", 0o600)
	env, _ := w.env(t, w.home)

	files := env.globalFiles()

	joined := strings.Join(files, "\n")
	if !strings.Contains(joined, ".gc-readable") || strings.Contains(joined, ".gc-unreadable") {
		t.Errorf("globalFiles = %v, want the readable include and not the unreadable one", files)
	}
}
