package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/config"
)

func keptRuleWorld(t *testing.T, special string) (world, Env, string) {
	t.Helper()
	w := newWorld(t)
	withoutConfig(t, w)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[user]\n\tname = Global Demo\n\temail = global@demo.example\n"+
		"[includeIf \"gitdir:~/special/\"]\n\tpath = ~/.gitconfig-special\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gitconfig-special"), "[user]\n\tname = Alex Octo\n\temail = alex.special@example.com\n"+
		"[credential \"https://github.com\"]\n\tusername = octo-alex\n"+special, 0o600)
	repo := filepath.Join(w.home, "special", "repo")
	runGit(t, w.home, "init", "-q", repo)
	env, _ := w.env(t, w.home)
	return w, env, repo
}

func TestRulesSetupKeepsStayEffectiveInTheirFolders(t *testing.T) {
	for name, tc := range map[string]struct {
		special   string
		key, want string
	}{
		"x509 signing":   {"[gpg]\n\tformat = x509\n[user]\n\tsigningkey = 0xDEADBEEF\n[commit]\n\tgpgsign = true\n", "gpg.format", "x509"},
		"a key for tags": {"[user]\n\tsigningkey = ~/.ssh/tags.pub\n[tag]\n\tgpgSign = true\n", "user.signingkey", "~/.ssh/tags.pub"},
		"ssh options":    {"[core]\n\tsshCommand = ssh -i ~/.ssh/id_special -o IdentitiesOnly=yes -p 443\n", "core.sshCommand", "ssh -i ~/.ssh/id_special -o IdentitiesOnly=yes -p 443"},
	} {
		t.Run(name, func(t *testing.T) {
			w, env, repo := keptRuleWorld(t, tc.special)
			mustWrite(t, filepath.Join(w.home, ".ssh", "id_special"), "KEY PLACEHOLDER (never read)\n", 0o600)

			if err := Setup(context.Background(), env, SetupOptions{Yes: true, NoShell: true}); err != nil {
				t.Fatal(err)
			}

			if got := runGit(t, repo, "config", tc.key); got != tc.want {
				t.Errorf("after setup %s = %q in the kept rule's folder, want %q as before", tc.key, got, tc.want)
			}
			if got := readFile(t, filepath.Join(w.home, ".gitconfig")); !strings.Contains(got, "gitdir:~/special/") {
				t.Errorf("the rule should be kept:\n%s", got)
			}
		})
	}
}

func TestAnIdentitySigningWithALiteralSSHKeyIsImported(t *testing.T) {
	w, env, repo := keptRuleWorld(t, "[gpg]\n\tformat = ssh\n[user]\n\tsigningkey = key::ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDemo\n[commit]\n\tgpgsign = true\n")

	if err := Setup(context.Background(), env, SetupOptions{Yes: true, NoShell: true}); err != nil {
		t.Fatal(err)
	}

	if cfg := readFile(t, env.configFile()); !strings.Contains(cfg, `key = "key::ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDemo"`) {
		t.Errorf("the literal key should become the profile's signing key:\n%s", cfg)
	}
	if got := runGit(t, repo, "config", "user.signingkey"); got != "key::ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDemo" {
		t.Errorf("signing key after setup = %q", got)
	}
	if got := readFile(t, filepath.Join(w.home, ".gitconfig")); strings.Contains(got, "gitdir:~/special/") {
		t.Errorf("the profile takes the rule over, so it should be gone:\n%s", got)
	}
}

func TestAFolderGivenToAnotherIdentityFollowsTheProfile(t *testing.T) {
	w := newWorld(t)
	// The user's rule says acme's folder is someone else's; the configuration gives it to acme.
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[includeIf \"gitdir:~/Projects/acme/\"]\n\tpath = ~/.gitconfig-old\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gitconfig-old"), "[user]\n\temail = old@example.com\n", 0o600)
	env, _ := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	if got := runGit(t, w.repos["billing"], "config", "user.email"); got != "1000002+acme-alex@users.noreply.github.com" {
		t.Errorf("~/Projects/acme/billing commits as %q; the folder was given to acme", got)
	}
}

func TestWhoamiDoesNotTakeARepositorysOwnSettingForTheUsersRule(t *testing.T) {
	w := newWorld(t)
	repo := w.repos["billing"]
	runGit(t, repo, "config", "extensions.worktreeConfig", "true")
	runGit(t, repo, "config", "--worktree", "user.email", "wt@demo.example")
	env, out := w.env(t, repo)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	err := WhoamiWith(context.Background(), env, WhoamiOptions{Check: true})

	if err == nil || strings.Contains(out.String(), "your own includeIf rule") {
		t.Errorf("a worktree setting is not the user's rule: err = %v\n%s", err, out.String())
	}
}

func TestUseAddsARuleWhereTheUsersOwnRuleWouldOtherwiseDecide(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[includeIf \"gitdir:~/Projects/acme/xy/\"]\n\tpath = ~/.gitconfig-xy\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gitconfig-xy"), "[user]\n\temail = xy@demo.example\n", 0o600)
	repo := filepath.Join(w.home, "Projects", "acme", "xy", "z")
	runGit(t, w.home, "init", "-q", repo)
	env, _ := w.env(t, repo)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	if err := Use(context.Background(), env, "acme"); err != nil {
		t.Fatal(err)
	}

	if got := runGit(t, repo, "config", "user.email"); got != "1000002+acme-alex@users.noreply.github.com" {
		t.Errorf("after `use acme` the repository commits as %q", got)
	}
}

func TestUninstallPutsBackTheRulesSetupTookOver(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w) // ~/.gitconfig has octo-alex's rule for ~/Projects/personal
	env, out := w.env(t, w.home)
	before := runGit(t, w.repos["blog"], "config", "user.email")
	if err := Setup(context.Background(), env, SetupOptions{Yes: true, NoShell: true}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(w.home, ".gitconfig")); strings.Contains(got, "gitdir:~/Projects/personal/") {
		t.Fatalf("setup should have taken the rule over:\n%s", got)
	}

	if err := Uninstall(context.Background(), env, true); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, filepath.Join(w.home, ".gitconfig")); !strings.Contains(got, "[includeIf \"gitdir:~/Projects/personal/\"]") {
		t.Errorf("uninstall should put the rule back:\n%s", got)
	}
	if got := runGit(t, w.repos["blog"], "config", "user.email"); got != before {
		t.Errorf("after uninstall ~/Projects/personal commits as %q, want %q as before setup", got, before)
	}
	if !strings.Contains(out.String(), `Put back your includeIf rule "gitdir:~/Projects/personal/"`) {
		t.Errorf("uninstall should say what it put back:\n%s", out.String())
	}
}

func TestAKeptRuleInAFolderOfASigningProfileWorksAsBefore(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	mustWrite(t, filepath.Join(w.home, ".ssh", "sig.pub"), "PUBLIC KEY PLACEHOLDER\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[user]\n\tname = Global Demo\n\temail = global@demo.example\n"+
		"[includeIf \"gitdir:~/sig/\"]\n\tpath = ~/.gc-sig\n[includeIf \"gitdir:~/x509/\"]\n\tpath = ~/.gc-x509\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gc-sig"), "[user]\n\tname = Alex Octo\n\temail = alex.personal@example.com\n"+
		"\tsigningkey = ~/.ssh/sig.pub\n[gpg]\n\tformat = ssh\n[commit]\n\tgpgsign = true\n"+
		"[credential \"https://github.com\"]\n\tusername = octo-alex\n", 0o600)
	// The same identity, signing with x509 only when asked.
	mustWrite(t, filepath.Join(w.home, ".gc-x509"), "[user]\n\tname = Alex Octo\n\temail = alex.personal@example.com\n"+
		"\tsigningkey = 0xDEADBEEF\n[gpg]\n\tformat = x509\n", 0o600)
	repo := filepath.Join(w.home, "x509", "r")
	runGit(t, w.home, "init", "-q", repo)
	env, _ := w.env(t, w.home)
	before := map[string]string{}
	for _, key := range []string{"gpg.format", "user.signingkey", "commit.gpgsign"} {
		before[key] = runGit(t, repo, "config", key)
	}

	if err := Setup(context.Background(), env, SetupOptions{Yes: true, NoShell: true}); err != nil {
		t.Fatal(err)
	}

	for key, want := range before {
		if got := runGit(t, repo, "config", key); got != want {
			t.Errorf("in ~/x509 %s = %q after setup, want %q as before", key, got, want)
		}
	}
}

func TestARuleThatOnlyRefinesKeepsTheProfilesIdentity(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[user]\n\tname = Global Demo\n\temail = global@demo.example\n"+
		"[includeIf \"onbranch:main\"]\n\tpath = ~/.gc-main\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gc-main"), "[pull]\n\trebase = true\n", 0o600)
	repo := w.repos["billing"]
	runGit(t, repo, "checkout", "-q", "-B", "main")
	env, _ := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	if got := runGit(t, repo, "config", "user.email"); got != "1000002+acme-alex@users.noreply.github.com" {
		t.Errorf("on main, acme's repository commits as %q; a rule setting only pull.rebase must not bring back the global identity", got)
	}
	if got := runGit(t, repo, "config", "pull.rebase"); got != "true" {
		t.Errorf("the branch rule should still apply: pull.rebase = %q", got)
	}
}

func TestUninstallPutsBackOnlyWhatGitPersonaRemoved(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	gitconfig := filepath.Join(w.home, ".gitconfig")
	// A rule the user deletes by hand before setup must stay deleted.
	mustWrite(t, gitconfig, readFile(t, gitconfig)+"[includeIf \"gitdir:~/gone/\"]\r\n\tpath = ~/.gc-gone\r\n", 0o600)
	env, _ := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil && !strings.Contains(err.Error(), "configuration") {
		t.Fatal(err)
	}
	mustWrite(t, gitconfig, strings.Replace(readFile(t, gitconfig), "[includeIf \"gitdir:~/gone/\"]\r\n\tpath = ~/.gc-gone\r\n", "", 1), 0o600)
	if err := Setup(context.Background(), env, SetupOptions{Yes: true, NoShell: true}); err != nil {
		t.Fatal(err)
	}

	if err := Uninstall(context.Background(), env, true); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, gitconfig)
	if strings.Contains(got, "gitdir:~/gone/") {
		t.Errorf("a rule the user deleted came back:\n%s", got)
	}
	if !strings.Contains(got, "gitdir:~/Projects/personal/") {
		t.Errorf("the rule setup took over should be back:\n%s", got)
	}
}

func TestYourOwnRulesOutsideProfileFoldersWorkAsBeforeWithoutADefault(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[user]\n\tname = Global Demo\n\temail = global@demo.example\n"+
		"[includeIf \"gitdir:~/hdr/\"]\n\tpath = ~/.gc-hdr\n[includeIf \"onbranch:release/**\"]\n\tpath = ~/.gc-rel\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gc-hdr"), "[user]\n\temail = hdr@demo.example\n[http]\n\textraHeader = X-Demo: 1\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gc-rel"), "[user]\n\temail = release@demo.example\n", 0o600)
	repo := filepath.Join(w.home, "hdr", "r")
	runGit(t, w.home, "init", "-q", repo)
	before := runGit(t, repo, "config", "--get-all", "http.extraHeader") + " " + runGit(t, repo, "config", "user.email")
	env, _ := w.env(t, w.home)
	out := &strings.Builder{}
	env.Stdout = out

	if err := Setup(context.Background(), env, SetupOptions{Yes: true, NoShell: true}); err != nil {
		t.Fatal(err)
	}

	if after := runGit(t, repo, "config", "--get-all", "http.extraHeader") + " " + runGit(t, repo, "config", "user.email"); after != before {
		t.Errorf("in ~/hdr: %q before, %q after", before, after)
	}
	if cfg := readFile(t, env.configFile()); strings.Contains(cfg, "default =") {
		t.Errorf("with your own rules left, setup must not propose a default:\n%s", cfg)
	}
	for _, want := range []string{"No default profile: one would override your own includeIf rules everywhere",
		"for ~/hdr", "for onbranch:release/**"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("setup should say %q:\n%s", want, out.String())
		}
	}
}

func TestAProfileFolderInsideYourRuleDoesNotInheritItsSigningOrSSHKey(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[includeIf \"gitdir:~/Projects/\"]\n\tpath = ~/.gc-client\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gc-client"), "[user]\n\temail = client@demo.example\n\tsigningkey = ~/.ssh/client.pub\n"+
		"[gpg]\n\tformat = ssh\n[commit]\n\tgpgsign = true\n[core]\n\tsshCommand = ssh -i ~/.ssh/client\n", 0o600)
	env, _ := w.env(t, w.home)

	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	repo := w.repos["billing"] // ~/Projects/acme: acme's folder inside the user's rule
	for key, bad := range map[string]string{"user.signingkey": "client", "core.sshCommand": "client", "commit.gpgsign": "true"} {
		if got := runGit(t, repo, "config", key); strings.Contains(got, bad) {
			t.Errorf("acme's repository %s = %q, taken from the user's rule around it", key, got)
		}
	}
}

func TestGlobalSettingsAreReadThroughDotfileIncludesButNotGitPersonasOwn(t *testing.T) {
	w := newWorld(t)
	t.Setenv("HOME", w.home)
	mustWrite(t, filepath.Join(w.home, "dot", "git"), "[core]\n\tsshCommand = ssh -o Demo=yes\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[include]\n\tpath = dot/git\n", 0o600)
	env, _ := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	got, err := env.gitConfigAll(context.Background(), "core.sshCommand")

	if err != nil || len(got) != 1 || got[0] != "ssh -o Demo=yes" {
		t.Errorf("gitConfigAll() = %v, %v; want only the dotfile's value, none from GitPersona's profiles", got, err)
	}
}

func TestUninstallPutsARuleBackWhereItWas(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	gitconfig := filepath.Join(w.home, ".gitconfig")
	// octo-alex's rule for ~/Projects/personal comes first; a rule inside it follows.
	mustWrite(t, gitconfig, readFile(t, gitconfig)+"[includeIf \"gitdir:~/Projects/personal/client/\"]\n\tpath = ~/.gc-client\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gc-client"), "[user]\n\temail = client@demo.example\n", 0o600)
	original := readFile(t, gitconfig)
	env, _ := w.env(t, w.home)
	if err := Setup(context.Background(), env, SetupOptions{Yes: true, NoShell: true}); err != nil {
		t.Fatal(err)
	}

	if err := Uninstall(context.Background(), env, true); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, gitconfig); got != original {
		t.Errorf("after uninstall ~/.gitconfig =\n%s\nwant it as before setup:\n%s", got, original)
	}
}

func TestRulesGoBackBeforeTheRightOccurrenceAcrossBackups(t *testing.T) {
	text := "[include]\n\tpath = dot\n[user]\n\temail = me@home\n[core]\n\teditor = vi\n[core]\n\tpager = less\n"
	sections := []ruleSection{
		// From an older backup: A stood before B, which a newer backup removed.
		{condition: "gitdir:~/a/", text: "[includeIf \"gitdir:~/a/\"]\n\tpath = a\n", before: `[includeIf "gitdir:~/b/"]`, beforeN: 1},
		// B stood before the second [core].
		{condition: "gitdir:~/b/", text: "[includeIf \"gitdir:~/b/\"]\n\tpath = b\n", before: "[core]", beforeN: 2},
	}

	got := withRules(text, sections)

	want := "[include]\n\tpath = dot\n[user]\n\temail = me@home\n[core]\n\teditor = vi\n" +
		"[includeIf \"gitdir:~/a/\"]\n\tpath = a\n[includeIf \"gitdir:~/b/\"]\n\tpath = b\n[core]\n\tpager = less\n"
	if got != want {
		t.Errorf("withRules() =\n%s\nwant\n%s", got, want)
	}
}

func TestUninstallAfterDoctorFixPutsTheRuleBackBelowTheGlobalIdentity(t *testing.T) {
	w := newWorld(t)
	gitconfig := filepath.Join(w.home, ".gitconfig")
	mustWrite(t, filepath.Join(w.home, ".gitconfig-personal"), "[user]\n\temail = 1000001+octo-alex@users.noreply.github.com\n", 0o600)
	original := "[include]\n\tpath = ~/dot\n[user]\n\temail = me@demo.example\n" +
		"[includeIf \"gitdir:~/Projects/personal/\"]\n\tpath = ~/.gitconfig-personal\n"
	mustWrite(t, gitconfig, original, 0o600)
	env, _ := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := RemoveOldRules(context.Background(), env); err != nil {
		t.Fatal(err)
	}

	if err := Uninstall(context.Background(), env, true); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, gitconfig); got != original {
		t.Errorf("after uninstall:\n%s\nwant\n%s", got, original)
	}
}

func TestARuleForTheWholeHomeIsYourOwnRuleToo(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[includeIf \"gitdir:"+w.home+"/\"]\n\tpath = ~/.gc-home\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gc-home"), "[user]\n\temail = home@demo.example\n", 0o600)
	env, _ := w.env(t, w.home)

	got, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	if got.Default != "" || len(got.OwnRules) == 0 {
		t.Errorf("default = %q, own rules = %v; a rule for the whole home would be overridden by a default", got.Default, got.OwnRules)
	}
}

func TestOnlyProfilesYourRulesReachResetTheSSHCommand(t *testing.T) {
	w := newWorld(t)
	// The rule covers personal's folders only; acme and globex are elsewhere and the default is acme.
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[includeIf \"gitdir:~/Projects/personal/\"]\n\tpath = ~/.gc-p\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gc-p"), "[core]\n\tsshCommand = ssh -i ~/.ssh/p\n[tag]\n\tgpgSign = true\n", 0o600)
	env, _ := w.env(t, w.home)

	signing, ssh := env.resets(context.Background(), mustConfig(t, w), readFile(t, filepath.Join(w.home, ".gitconfig")))

	if !ssh["personal"] || !signing["personal"] || !ssh["acme"] || ssh["globex"] {
		t.Errorf("resets: signing %v, ssh %v; want personal (its folder) and acme (the default), not globex", signing, ssh)
	}
}

func mustConfig(t *testing.T, w world) config.Config {
	t.Helper()
	cfg, err := config.Load(filepath.Join(w.home, ".config", "gitpersona", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestAKeptRuleInsideAnotherProfileFolderIsReportedAsOverridden(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w) // octo-alex's rule for ~/Projects/personal is taken over
	gitconfig := filepath.Join(w.home, ".gitconfig")
	mustWrite(t, gitconfig, readFile(t, gitconfig)+"[includeIf \"gitdir:~/Projects/personal/x509/\"]\n\tpath = ~/.gc-x509\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gc-x509"), "[user]\n\tname = Alex Personal\n\temail = alex.personal@example.com\n"+
		"\tsigningkey = 0xDEADBEEF\n[gpg]\n\tformat = x509\n[commit]\n\tgpgsign = true\n", 0o600)
	env, _ := w.env(t, w.home)
	out := &strings.Builder{}
	env.Stdout = out
	env.Stdin = strings.NewReader("n\n")

	if err := Setup(context.Background(), env, SetupOptions{}); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "! Your includeIf rule for ~/Projects/personal/x509 does more than a profile can (it signs with x509), but profile octo-alex applies there") {
		t.Errorf("setup must not claim the rule stays in charge:\n%s", out.String())
	}
}
