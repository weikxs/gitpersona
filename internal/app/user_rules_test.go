package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/generate"
	"github.com/weikxs/gitpersona/internal/paths"
)

func TestUserRulesAreReadSoTheyCanBeIncludedFromElsewhere(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	text := "[user]\n\tname = Demo\n" +
		"[includeIf \"gitdir:~/oss/\"]\n\tpath = .gitconfig-oss ; relative to the home\n" +
		"[includeIf \"gitdir:./rel/**\"] path = \"/srv/a b/x\"\n" +
		"[IncludeIf \"onbranch:release/**\"]\n\tpath = ~/.gitconfig-release\n" +
		"[includeIf \"gitdir:~/single/repo/.git\"]\n\tpath = ~/.gitconfig-single\n" +
		"[include]\n\tpath = ~/.gitconfig-common\n"

	got, _ := env.userRules(context.Background(), config.Config{}, text)

	home := w.home
	want := []generate.UserRule{
		{Condition: "gitdir:~/oss/", Path: filepath.Join(home, ".gitconfig-oss"), Dir: filepath.Join(home, "oss")},
		{Condition: "gitdir:" + home + "/rel/**", Path: "/srv/a b/x", Dir: filepath.Join(home, "rel")},
		{Condition: "onbranch:release/**", Path: "~/.gitconfig-release"},
		{Condition: "gitdir:~/single/repo/.git", Path: "~/.gitconfig-single"},
	}
	if len(got) != len(want) {
		t.Fatalf("userRules() = %+v\nwant %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("rule %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// keptRuleWorld is a home whose ~/special rule does more than a profile can.
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

func TestWhoamiSaysWhenTheUsersOwnRuleSetsTheAuthor(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[includeIf \"gitdir:~/Projects/acme/side/\"]\n\tpath = ~/.gitconfig-side\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gitconfig-side"), "[user]\n\temail = side@demo.example\n", 0o600)
	repo := filepath.Join(w.home, "Projects", "acme", "side")
	runGit(t, w.home, "init", "-q", repo)
	env, out := w.env(t, repo)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	if err := WhoamiWith(context.Background(), env, WhoamiOptions{Check: true}); err != nil {
		t.Fatalf("whoami --check where the user's own rule applies: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Git uses side@demo.example here, from your own includeIf rule (~/.gitconfig-side)") {
		t.Errorf("whoami should explain the user's rule:\n%s", out.String())
	}
}

func TestRemovingOldRulesRegeneratesTheRules(t *testing.T) {
	w := newWorld(t)
	gitconfig := filepath.Join(w.home, ".gitconfig")
	mustWrite(t, gitconfig, "[includeIf \"gitdir:~/Projects/personal/\"]\n\tpath = ~/.gitconfig-personal\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gitconfig-personal"), "[user]\n\temail = 1000001+octo-alex@users.noreply.github.com\n", 0o600)
	env, _ := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	rules := filepath.Join(env.ConfigDir, generate.RulesFile)
	if !strings.Contains(readFile(t, rules), "gitdir:~/Projects/personal/") {
		t.Fatalf("the user's rule should be included again:\n%s", readFile(t, rules))
	}

	if err := RemoveOldRules(context.Background(), env); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, rules); strings.Contains(got, "Your own rule") {
		t.Errorf("a removed rule must not stay in GitPersona's rules:\n%s", got)
	}
	if changes, err := planApply(context.Background(), env); err != nil || len(changes) != 0 {
		t.Errorf("after removing, everything should be up to date: %v %+v", err, changes)
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

func TestUserRulePathsFollowGitThroughASymlinkedGitConfig(t *testing.T) {
	w := newWorld(t)
	gitconfig := filepath.Join(w.home, ".gitconfig")
	mustWrite(t, filepath.Join(w.home, "dot", "gitconfig"), "", 0o600)
	if err := os.Remove(gitconfig); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(w.home, "dot", "gitconfig"), gitconfig); err != nil {
		t.Fatal(err)
	}
	env, _ := w.env(t, w.home)

	got, _ := env.userRules(context.Background(), config.Config{},
		"[includeIf \"gitdir:./rel/\"]\n\tpath = w.inc\n[includeIf \"gitdir:work/\"]\n\tpath = ~/w2.inc\n")

	realDot := filepath.Join(paths.ResolveSymlinks(w.home), "dot")
	if len(got) != 2 || got[0].Path != filepath.Join(w.home, "w.inc") || got[0].Condition != "gitdir:"+realDot+"/rel/" {
		t.Errorf("rule = %+v; Git reads a relative path next to ~/.gitconfig as named, and ./ next to its real file", got)
	}
	if len(got) == 2 && got[1].Dir != "/" {
		t.Errorf("an unanchored pattern = %+v, want it least specific", got[1])
	}
}

func TestRulesThatCannotBeIncludedTwiceAreLeftAndReported(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[includeIf \"gitdir:~/Projects/acme/side/\"]\n\tpath = ~/.gitconfig-side\n"+
		"[includeIf \"gitdir:~/loop/\"]\n\tpath = ~/.config/gitpersona/rules.gitconfig\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gitconfig-side"), "[user]\n\temail = side@demo.example\n"+
		"[http]\n\textraHeader = X-Demo: 1\n", 0o600)
	env, out := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	Doctor(context.Background(), env)

	rules := readFile(t, filepath.Join(env.ConfigDir, generate.RulesFile))
	if strings.Contains(rules, "Your own rule") {
		t.Errorf("neither rule may be included again:\n%s", rules)
	}
	for _, want := range []string{
		`GitPersona's rules override your includeIf rule "gitdir:~/Projects/acme/side/": it sets http.extraheader, which would be added twice`,
		`GitPersona's rules override your includeIf rule "gitdir:~/loop/": it includes GitPersona's own configuration`,
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("doctor should say %q:\n%s", want, out.String())
		}
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
