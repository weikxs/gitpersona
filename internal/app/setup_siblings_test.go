package app

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/shell"
)

// teamWorld has two rules of the user's for acme-alex: ~/work commits as
// "Alex Acme", the team folder inside it as "alex_team".
func teamWorld(t *testing.T, teamName string) (world, Env) {
	t.Helper()
	w := newWorld(t)
	withoutConfig(t, w)
	identity := func(name string) string {
		return "[user]\n\tname = " + name + "\n\temail = acme@demo.example\n[credential \"https://github.com\"]\n\tusername = acme-alex\n"
	}
	appendRule(t, w, "work", identity("Alex Acme"))
	appendRule(t, w, "work/team", identity(teamName))
	for _, dir := range []string{"work/app", "work/team/infra"} {
		runGit(t, w.home, "init", "-q", filepath.Join(w.home, dir))
	}
	env, _ := w.env(t, w.home)
	return w, env
}

func TestARuleForTheSameAccountWithAnotherAuthorBecomesItsOwnProfile(t *testing.T) {
	w, env := teamWorld(t, "alex_team")
	before := runGit(t, filepath.Join(w.home, "work/team/infra"), "config", "user.name")

	proposal, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	i := slices.IndexFunc(proposal.Profiles, func(p SetupProfile) bool { return p.ID == "acme-alex-team" })
	if i < 0 {
		t.Fatalf("no profile for the team folder: %+v", proposal.Profiles)
	}
	team := proposal.Profiles[i]
	if team.Account != "acme-alex" || team.Name != "alex_team" || !slices.Equal(team.Dirs, []string{"~/work/team"}) {
		t.Errorf("team profile = %+v", team)
	}
	var printed strings.Builder
	shown := env
	shown.Stdout = &printed
	shown.printProposal(proposal)
	if !strings.HasPrefix(printed.String(), "GitPersona found 2 GitHub accounts") {
		t.Errorf("two accounts, three profiles:\n%s", printed.String())
	}
	for _, u := range proposal.Unmatched {
		if u.OverriddenBy != "" {
			t.Errorf("no identity should be overridden now: %+v", u)
		}
	}
	if err := ApplySetup(context.Background(), env, proposal.Plan()); err != nil {
		t.Fatal(err)
	}
	if got := runGit(t, filepath.Join(w.home, "work/team/infra"), "config", "user.name"); got != before || got != "alex_team" {
		t.Errorf("the team folder commits as %q after setup, %q before", got, before)
	}
	if got := runGit(t, filepath.Join(w.home, "work/app"), "config", "user.name"); got != "Alex Acme" {
		t.Errorf("the rest of ~/work commits as %q", got)
	}
}

func TestARuleForTheSameAccountAndAuthorOnlyAddsItsFolder(t *testing.T) {
	_, env := teamWorld(t, "Alex Acme")

	proposal, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	i := slices.IndexFunc(proposal.Profiles, func(p SetupProfile) bool { return p.ID == "acme-alex" })
	if i < 0 || len(proposal.Profiles) != 2 || !slices.Contains(proposal.Profiles[i].Dirs, "~/work/team") {
		t.Errorf("the same author should only add its folder: %+v", proposal.Profiles)
	}
}

func TestSiblingID(t *testing.T) {
	for dirs, want := range map[string]string{"~/work/team": "acme-team", "": "acme-2", "~/x/" + "a-very-long-folder-name-for-the-team": "acme-a-very-long-folder-name"} {
		var d []string
		if dirs != "" {
			d = []string{dirs}
		}
		if got := siblingID("acme", d); got != want {
			t.Errorf("siblingID(%q) = %q, want %q", dirs, got, want)
		}
	}
}

// remoteRuleWorld gives acme-alex ~/work, and adds a rule of the user's for
// repositories whose remote uses the github-side alias.
func remoteRuleWorld(t *testing.T, aliasRepo bool, extra ...string) SetupProposal {
	t.Helper()
	w := newWorld(t)
	withoutConfig(t, w)
	appendRule(t, w, "work", "[user]\n\tname = Alex Acme\n\temail = acme@demo.example\n[credential \"https://github.com\"]\n\tusername = acme-alex\n")
	f := filepath.Join(w.home, ".gitconfig")
	mustWrite(t, f, readFile(t, f)+"[includeIf \"hasconfig:remote.*.url:git@github-side:*/**\"]\n\tpath = ~/.gitconfig-side\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gitconfig-side"), "[user]\n\tname = Side\n\temail = side@demo.example\n", 0o600)
	for _, rule := range extra { // rules of the user's after the remote rule
		appendRule(t, w, rule, "[user]\n\tname = Later\n\temail = later@demo.example\n")
	}
	runGit(t, w.home, "init", "-q", filepath.Join(w.home, "work/app"))
	runGit(t, filepath.Join(w.home, "work/app"), "remote", "add", "origin", "https://github.com/acme-alex/app.git")
	if aliasRepo {
		runGit(t, w.home, "init", "-q", filepath.Join(w.home, "work/side"))
		runGit(t, filepath.Join(w.home, "work/side"), "remote", "add", "origin", "git@github-side:demo-org/side.git")
	}
	env, _ := w.env(t, w.home)
	proposal, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	return proposal
}

func TestARemoteRuleIsMentionedOnlyWhenRepositoriesMatchIt(t *testing.T) {
	quiet := remoteRuleWorld(t, false)
	for _, s := range quiet.Shadowed {
		if strings.Contains(s, "github-side") {
			t.Errorf("no repository in ~/work uses github-side, so nothing changes for it: %q", s)
		}
	}

	loud := remoteRuleWorld(t, true)
	if !slices.ContainsFunc(loud.Shadowed, func(s string) bool {
		return strings.Contains(s, "github-side") && strings.Contains(s, "1 repository there matches it, ~/work/side")
	}) {
		t.Errorf("the repository that uses github-side should be named: %q", loud.Shadowed)
	}
}

func TestSetupAndDoctorNoticeAGhSwitcherOfTheUsersOwn(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".config", "fish", "conf.d", "gh-by-dir.fish"), `function gh
    set -l account
    switch $PWD
        case "$HOME/fun/*"
            set account demo-fun
    end
    GH_TOKEN=(command gh auth token -u $account) command gh $argv
end
`, 0o600)
	mustWrite(t, filepath.Join(w.home, ".zshrc"), "export EDITOR=vi\n", 0o600) // no gh function: not listed
	env, out := w.env(t, w.home)

	if got := env.ghSwitchers(); !slices.Equal(got, []string{"~/.config/fish/conf.d/gh-by-dir.fish"}) {
		t.Errorf("ghSwitchers = %q", got)
	}
	_ = Doctor(context.Background(), env)
	if strings.Contains(out.String(), "you already switch gh accounts") {
		t.Errorf("without GitPersona's integration there is nothing to compare:\n%s", out.String())
	}
	if err := InstallShell(context.Background(), env, []shell.Kind{shell.Fish}, false); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	_ = Doctor(context.Background(), env)
	if !strings.Contains(out.String(), "you already switch gh accounts in ~/.config/fish/conf.d/gh-by-dir.fish") {
		t.Errorf("doctor should mention it:\n%s", out.String())
	}
}

func TestOutermostListsEachScannedFolderOnce(t *testing.T) {
	got := outermost([]string{"~/work/team", "~/code", "~/work", "~/Code", "~/workshop"}, strings.ToLower)
	if want := []string{"~/code", "~/work", "~/workshop"}; !slices.Equal(got, want) {
		t.Errorf("outermost = %q, want %q", got, want)
	}
}

func TestARemoteRuleALaterRuleOverridesChangesNothing(t *testing.T) {
	proposal := remoteRuleWorld(t, true, "work/side")

	for _, s := range proposal.Shadowed {
		if strings.Contains(s, "github-side") {
			t.Errorf("~/work/side commits as Later today, not by the remote rule: %q", s)
		}
	}
}

func TestAHookThatExportsGhTokenIsNoticed(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".config", "fish", "conf.d", "token-by-dir.fish"), `function __token_by_dir --on-variable PWD
    set -l token (command gh auth token -h github.com -u demo-fun)
    set -gx GH_TOKEN $token
end
`, 0o600)
	mustWrite(t, filepath.Join(w.home, ".bashrc"), "# gh auth token is mentioned here only\nexport EDITOR=vi\n", 0o600)
	env, _ := w.env(t, w.home)

	if got := env.ghSwitchers(); !slices.Equal(got, []string{"~/.config/fish/conf.d/token-by-dir.fish"}) {
		t.Errorf("ghSwitchers = %q", got)
	}
}

func TestOnlyRealGhSwitchersAreNoticed(t *testing.T) {
	w := newWorld(t)
	fish := filepath.Join(w.home, ".config", "fish")
	mustWrite(t, filepath.Join(fish, "conf.d", "whoami.fish"), "function gh-whoami\n    command gh auth status\nend\n", 0o600)
	mustWrite(t, filepath.Join(fish, "conf.d", "alias.fish"), "alias gh 'GH_TOKEN=(command gh auth token -u demo-fun) command gh'\n", 0o600)
	mustWrite(t, filepath.Join(fish, "functions", "gh.fish"), "function gh --wraps gh\n    GH_TOKEN=(command gh auth token -u demo-fun) command gh $argv\nend\n", 0o600)
	env, _ := w.env(t, w.home)

	got := env.ghSwitchers()

	want := []string{"~/.config/fish/conf.d/alias.fish", "~/.config/fish/functions/gh.fish"}
	if !slices.Equal(got, want) {
		t.Errorf("ghSwitchers = %q, want %q (gh-whoami is another command)", got, want)
	}
}

func TestAnAccountsFoldersGoToItsFirstProfileOnly(t *testing.T) {
	w, env := teamWorld(t, "alex_team")
	for _, name := range []string{"a", "b"} {
		dir := filepath.Join(w.home, "code", name)
		runGit(t, w.home, "init", "-q", dir)
		runGit(t, dir, "remote", "add", "origin", "https://github.com/acme-alex/"+name+".git")
	}

	proposal, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range proposal.Profiles {
		if p.ID == "acme-alex-team" && len(p.Found) > 0 {
			t.Errorf("the second profile of acme-alex got the account's folders: %+v", p.Found)
		}
	}
	if err := ApplySetup(context.Background(), env, proposal.Plan()); err != nil {
		t.Errorf("the proposal as it is must apply: %v", err)
	}
}

func TestARuleThatDoesNotApplyTodayMakesNoSecondProfile(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	identity := func(name string) string {
		return "[user]\n\tname = " + name + "\n\temail = acme@demo.example\n[credential \"https://github.com\"]\n\tusername = acme-alex\n"
	}
	// The team rule comes first, so the later ~/work rule wins inside it today.
	appendRule(t, w, "work/team", identity("alex_team"))
	appendRule(t, w, "work", identity("Alex Acme"))
	runGit(t, w.home, "init", "-q", filepath.Join(w.home, "work/team/infra"))
	before := runGit(t, filepath.Join(w.home, "work/team/infra"), "config", "user.name")
	env, _ := w.env(t, w.home)

	proposal, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplySetup(context.Background(), env, proposal.Plan()); err != nil {
		t.Fatal(err)
	}

	if got := runGit(t, filepath.Join(w.home, "work/team/infra"), "config", "user.name"); got != before {
		t.Errorf("the team folder committed as %q before setup and as %q after", before, got)
	}
}

func TestARuleThatDoesMoreMakesNoEmptySecondProfile(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	identity := func(name, extra string) string {
		return "[user]\n\tname = " + name + "\n\temail = acme@demo.example\n[credential \"https://github.com\"]\n\tusername = acme-alex\n" + extra
	}
	appendRule(t, w, "work", identity("Alex Acme", ""))
	appendRule(t, w, "work/team", identity("alex_team", "[core]\n\tautocrlf = input\n"))
	env, _ := w.env(t, w.home)

	proposal, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	// The team rule sets more than a profile can, so it stays the user's:
	// no second profile for it (one without folders would do nothing).
	if len(proposal.Profiles) != 2 {
		t.Errorf("profiles = %+v, want only the two accounts'", proposal.Profiles)
	}
}

func TestARemoteRuleMatchesAnyRemoteAndHostGlobs(t *testing.T) {
	for name, pattern := range map[string]string{"another remote": "git@github-side:*/**", "a host glob": "git@github-*:demo-org/**"} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			withoutConfig(t, w)
			appendRule(t, w, "work", "[user]\n\tname = Alex Acme\n\temail = acme@demo.example\n[credential \"https://github.com\"]\n\tusername = acme-alex\n")
			f := filepath.Join(w.home, ".gitconfig")
			mustWrite(t, f, readFile(t, f)+"[includeIf \"hasconfig:remote.*.url:"+pattern+"\"]\n\tpath = ~/.gitconfig-side\n", 0o600)
			mustWrite(t, filepath.Join(w.home, ".gitconfig-side"), "[user]\n\tname = Side\n\temail = side@demo.example\n", 0o600)
			dir := filepath.Join(w.home, "work/fork")
			runGit(t, w.home, "init", "-q", dir)
			runGit(t, dir, "remote", "add", "origin", "https://github.com/acme-alex/fork.git")
			runGit(t, dir, "remote", "add", "upstream", "git@github-side:demo-org/fork.git")
			env, _ := w.env(t, w.home)

			proposal, err := ProposeSetup(context.Background(), env)
			if err != nil {
				t.Fatal(err)
			}

			if !slices.ContainsFunc(proposal.Shadowed, func(s string) bool { return strings.Contains(s, "~/work/fork") }) {
				t.Errorf("the rule applies to ~/work/fork today through its upstream remote: %q", proposal.Shadowed)
			}
		})
	}
}

func TestDoctorMentionsARemoteRuleOnlyWhenRepositoriesMatchIt(t *testing.T) {
	for _, aliasRepo := range []bool{false, true} {
		w := newWorld(t)
		withoutConfig(t, w)
		appendRule(t, w, "work", "[user]\n\tname = Alex Acme\n\temail = acme@demo.example\n[credential \"https://github.com\"]\n\tusername = acme-alex\n")
		f := filepath.Join(w.home, ".gitconfig")
		mustWrite(t, f, readFile(t, f)+"[includeIf \"hasconfig:remote.*.url:git@github-side:*/**\"]\n\tpath = ~/.gitconfig-side\n", 0o600)
		mustWrite(t, filepath.Join(w.home, ".gitconfig-side"), "[user]\n\tname = Side\n\temail = side@demo.example\n", 0o600)
		runGit(t, w.home, "init", "-q", filepath.Join(w.home, "work/app"))
		if aliasRepo {
			runGit(t, w.home, "init", "-q", filepath.Join(w.home, "work/side"))
			runGit(t, filepath.Join(w.home, "work/side"), "remote", "add", "origin", "git@github-side:demo-org/side.git")
		}
		env, out := w.env(t, w.home)
		proposal, err := ProposeSetup(context.Background(), env)
		if err != nil {
			t.Fatal(err)
		}
		if err := ApplySetup(context.Background(), env, proposal.Plan()); err != nil {
			t.Fatal(err)
		}
		out.Reset()

		_ = Doctor(context.Background(), env)

		mentioned := strings.Contains(out.String(), "git@github-side")
		switch {
		case !aliasRepo && mentioned:
			t.Errorf("no repository in ~/work follows the rule, so doctor should not warn:\n%s", out.String())
		case aliasRepo && !strings.Contains(out.String(), "1 of their repositories match it, such as ~/work/side"):
			t.Errorf("doctor should name ~/work/side:\n%s", out.String())
		}
	}
}
