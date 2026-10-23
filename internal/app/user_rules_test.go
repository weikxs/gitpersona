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
		// Git resolves ./ against the real directory of the file.
		{Condition: "gitdir:" + paths.ResolveSymlinks(home) + "/rel/**", Path: "/srv/a b/x", Dir: filepath.Join(paths.ResolveSymlinks(home), "rel")},
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

func TestRulesInIncludedFilesAndTheXDGConfigAreIncludedToo(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".config", "git", "config"), "[includeIf \"gitdir:~/xdg/\"]\n\tpath = ~/.gc-xdg\n", 0o600)
	mustWrite(t, filepath.Join(w.home, "dotfiles", "rules"), "[includeIf \"gitdir:~/inc/\"]\n\tpath = inc.gitconfig\n", 0o600)
	env, _ := w.env(t, w.home)

	got, _ := env.userRules(context.Background(), config.Config{},
		"[include]\n\tpath = dotfiles/rules\n[include]\n\tpath = ~/.config/gitpersona/rules.gitconfig\n")

	if len(got) != 2 || got[0].Condition != "gitdir:~/xdg/" || got[1].Condition != "gitdir:~/inc/" ||
		got[1].Path != filepath.Join(w.home, "dotfiles", "inc.gitconfig") {
		t.Errorf("userRules() = %+v; want ~/.config/git/config's rule first, then the included file's", got)
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
		got := runGit(t, repo, "config", key)
		if key == "commit.gpgsign" && want == "" {
			want = "false" // unset before; GitPersona writes Git's default
		}
		if got != want {
			t.Errorf("in ~/x509 %s = %q after setup, want %q as before", key, got, want)
		}
	}
}

func TestSetupSaysWhenItCannotKeepARuleWorking(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[user]\n\tname = Global Demo\n\temail = global@demo.example\n"+
		"[includeIf \"gitdir:~/hdr/\"]\n\tpath = ~/.gc-hdr\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gc-hdr"), "[user]\n\temail = hdr@demo.example\n[http]\n\textraHeader = X-Demo: 1\n", 0o600)
	env, _ := w.env(t, w.home)
	out := &strings.Builder{}
	env.Stdout = out

	if err := Setup(context.Background(), env, SetupOptions{Print: false, Yes: true, NoShell: true}); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "Replaced: Global Demo <hdr@demo.example> for ~/hdr") ||
		!strings.Contains(out.String(), "it sets http.extraheader, which would be added twice, so commits there will use profile") {
		t.Errorf("setup must not claim the rule keeps working:\n%s", out.String())
	}
}

func TestYourOwnRulesKeepTheirOrder(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[includeIf \"onbranch:main\"]\n\tpath = ~/.gc-main\n"+
		"[includeIf \"gitdir:~/oss/\"]\n\tpath = ~/.gc-oss\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gc-main"), "[user]\n\temail = main@demo.example\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gc-oss"), "[user]\n\temail = oss@demo.example\n", 0o600)
	repo := filepath.Join(w.home, "oss", "r")
	runGit(t, w.home, "init", "-q", "-b", "main", repo)
	before := runGit(t, repo, "config", "user.email")
	env, _ := w.env(t, w.home)

	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	if got := runGit(t, repo, "config", "user.email"); before != "oss@demo.example" || got != before {
		t.Errorf("on main in ~/oss: %q before, %q after; ~/oss comes later in your file, so it wins", before, got)
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

func TestAnInnerRuleBuildsOnItsEnclosingRuleAsBefore(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[user]\n\tname = Global Demo\n\temail = global@demo.example\n"+
		"[includeIf \"gitdir:~/w/team/\"]\n\tpath = ~/.gc-team\n[includeIf \"gitdir:~/w/\"]\n\tpath = ~/.gc-w\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gc-w"), "[user]\n\temail = w@demo.example\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gc-team"), "[core]\n\tsshCommand = ssh -p 2222\n", 0o600)
	repo := filepath.Join(w.home, "w", "team", "r")
	runGit(t, w.home, "init", "-q", repo)
	before := runGit(t, repo, "config", "user.email") + " " + runGit(t, repo, "config", "core.sshCommand")
	env, _ := w.env(t, w.home)

	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	if after := runGit(t, repo, "config", "user.email") + " " + runGit(t, repo, "config", "core.sshCommand"); after != before {
		t.Errorf("in ~/w/team: %q before, %q after", before, after)
	}
}

func TestTheGlobalIdentityIsReadFromDotfilesToo(t *testing.T) {
	w := newWorld(t)
	t.Setenv("HOME", w.home)
	mustWrite(t, filepath.Join(w.home, "dot", "identity"), "[user]\n\tname = Dot Demo\n\temail = dot@demo.example\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[include]\n\tpath = dot/identity\n"+
		"[includeIf \"gitdir:~/side/\"]\n\tpath = ~/.gc-side\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gc-side"), "[core]\n\tautocrlf = input\n", 0o600)
	repo := filepath.Join(w.home, "side", "r")
	runGit(t, w.home, "init", "-q", repo)
	env, out := w.env(t, repo)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	if got := runGit(t, repo, "config", "user.email"); got != "dot@demo.example" {
		t.Errorf("under your own rule the identity is your global one from the dotfile, got %q", got)
	}
	if err := WhoamiWith(context.Background(), env, WhoamiOptions{Check: true}); err != nil ||
		!strings.Contains(out.String(), "your own global identity, which GitPersona keeps under your own includeIf rule") {
		t.Errorf("whoami --check under your rule: %v\n%s", err, out.String())
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
