package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/config"
)

func TestProviderOfTellsGitLabAndBitbucketFromOtherHosts(t *testing.T) {
	for host, want := range map[string]string{
		"gitlab.com":          config.ProviderGitLab,
		"GitLab.Demo.example": config.ProviderGitLab,
		"code.gitlab.example": config.ProviderGitLab,
		"bitbucket.org":       config.ProviderBitbucket,
		"git.demo.example":    config.ProviderGeneric,
		"mygitlabish.example": config.ProviderGeneric,
	} {
		if got := providerOf(host); got != want {
			t.Errorf("providerOf(%q) = %q, want %q", host, got, want)
		}
	}
}

func TestUniqueIDAddsTheHostWhenANameIsTaken(t *testing.T) {
	taken := map[string]bool{"demo": true}
	if got := uniqueID("demo", "gitlab.com", taken); got != "demo-gitlab" {
		t.Errorf("uniqueID = %q, want demo-gitlab", got)
	}
	taken["demo-gitlab"] = true
	if got := uniqueID("demo", "gitlab.com", taken); got != "demo-gitlab-2" {
		t.Errorf("uniqueID = %q, want demo-gitlab-2", got)
	}
	long := strings.Repeat("a", maxIDLen)
	taken[long] = true
	if got := uniqueID(long, "gitlab.com", taken); len(got) > maxIDLen || taken[got] {
		t.Errorf("uniqueID of a long name = %q, want a new name of at most %d characters", got, maxIDLen)
	}
}

// hostWorld adds an existing GitLab identity under ~/lab, set up by hand,
// to the usual demo world.
func hostWorld(t *testing.T, labAccount, labEmail string) (world, Env) {
	t.Helper()
	w := newWorld(t)
	withoutConfig(t, w)
	f, err := os.OpenFile(filepath.Join(w.home, ".gitconfig"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("[includeIf \"gitdir:~/lab/\"]\n\tpath = ~/.gitconfig-lab\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	mustWrite(t, filepath.Join(w.home, ".gitconfig-lab"), "[user]\n\tname = Lab Person\n\temail = "+labEmail+
		"\n[credential \"https://gitlab.com\"]\n\tusername = "+labAccount+"\n", 0o600)
	runGit(t, w.home, "init", "-q", filepath.Join(w.home, "lab", "tool"))
	runGit(t, filepath.Join(w.home, "lab", "tool"), "remote", "add", "origin", "https://gitlab.com/"+labAccount+"/tool.git")
	env, _ := w.env(t, w.home)
	return w, env
}

func profileFor(t *testing.T, p SetupProposal, host string) SetupProfile {
	t.Helper()
	i := slices.IndexFunc(p.Profiles, func(x SetupProfile) bool { return x.Host == host })
	if i < 0 {
		t.Fatalf("no profile for %s in %+v", host, p.Profiles)
	}
	return p.Profiles[i]
}

func TestSetupTurnsAnExistingGitLabIdentityIntoAProfile(t *testing.T) {
	w, env := hostWorld(t, "demo-lab", "lab@demo.example")

	proposal, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	lab := profileFor(t, proposal, "gitlab.com")
	if lab.ID != "demo-lab" || lab.Provider != config.ProviderGitLab || lab.Account != "demo-lab" ||
		lab.Email != "lab@demo.example" || !slices.Equal(lab.Dirs, []string{"~/lab"}) {
		t.Errorf("GitLab profile = %+v", lab)
	}
	if !slices.Contains(proposal.OldRules, "~/lab") {
		t.Errorf("old rules = %v, want the ~/lab rule taken over", proposal.OldRules)
	}
	if err := ApplySetup(context.Background(), env, proposal.Plan()); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(w.home, "lab", "tool")
	if got := runGit(t, repo, "config", "credential.https://gitlab.com.username"); got != "demo-lab" {
		t.Errorf("the GitLab repository pushes as %q, want demo-lab", got)
	}
	if got := runGit(t, repo, "config", "user.email"); got != "lab@demo.example" {
		t.Errorf("the GitLab repository commits as %q", got)
	}
	cfg := readFile(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"))
	if !strings.Contains(cfg, "provider = \"gitlab\"\nhost    = \"gitlab.com\"\n") {
		t.Errorf("the configuration should name the provider and host:\n%s", cfg)
	}
}

func TestSetupWorksWithoutGhForOtherHosts(t *testing.T) {
	w, env := hostWorld(t, "demo-lab", "lab@demo.example")
	t.Setenv("PATH", w.pathWithoutGH)

	proposal, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatalf("setup without gh should still offer the GitLab profile: %v", err)
	}

	if len(proposal.Profiles) != 1 || proposal.Profiles[0].Host != "gitlab.com" {
		t.Errorf("profiles = %+v, want only the GitLab one", proposal.Profiles)
	}
}

func TestSetupWithoutGhOrOtherHostsStillAsksForGh(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	t.Setenv("PATH", w.pathWithoutGH)
	env, _ := w.env(t, w.home)

	if _, err := ProposeSetup(context.Background(), env); err == nil || !strings.Contains(err.Error(), "GitHub CLI") {
		t.Errorf("ProposeSetup() = %v, want the gh install hint", err)
	}
}

func TestAGitLabAccountNamedLikeAGitHubOneGetsItsOwnProfile(t *testing.T) {
	// octo-alex's public GitHub email is alex@octo.example; the GitLab rule
	// uses it too, but names its GitLab account, so it stays GitLab's.
	_, env := hostWorld(t, "octo-alex", "alex@octo.example")

	proposal, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	lab := profileFor(t, proposal, "gitlab.com")
	if lab.ID != "octo-alex-gitlab" || lab.Account != "octo-alex" {
		t.Errorf("GitLab profile = %+v, want octo-alex-gitlab", lab)
	}
	if err := ApplySetup(context.Background(), env, proposal.Plan()); err != nil {
		t.Fatal(err)
	}
}

func TestSetupReadsTheAccountFromRemotesOnACompanyServer(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	f, _ := os.OpenFile(filepath.Join(w.home, ".gitconfig"), os.O_APPEND|os.O_WRONLY, 0)
	_, _ = f.WriteString("[includeIf \"gitdir:~/corp/\"]\n\tpath = ~/.gitconfig-corp\n")
	f.Close()
	mustWrite(t, filepath.Join(w.home, ".gitconfig-corp"), "[user]\n\tname = Corp Person\n\temail = me@corp.example\n", 0o600)
	for _, name := range []string{"api", "web"} {
		dir := filepath.Join(w.home, "corp", name)
		runGit(t, w.home, "init", "-q", dir)
		runGit(t, dir, "remote", "add", "origin", "https://demo-corp@git.corp.example/team/"+name+".git")
	}
	env, _ := w.env(t, w.home)

	proposal, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	corp := profileFor(t, proposal, "git.corp.example")
	if corp.Provider != config.ProviderGeneric || corp.Account != "demo-corp" || corp.Email != "me@corp.example" {
		t.Errorf("company profile = %+v", corp)
	}
}

func TestRemotesOnTwoHostsMakeNoProfile(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	f, _ := os.OpenFile(filepath.Join(w.home, ".gitconfig"), os.O_APPEND|os.O_WRONLY, 0)
	_, _ = f.WriteString("[includeIf \"gitdir:~/mixed/\"]\n\tpath = ~/.gitconfig-mixed\n")
	f.Close()
	mustWrite(t, filepath.Join(w.home, ".gitconfig-mixed"), "[user]\n\temail = mixed@demo.example\n", 0o600)
	for name, url := range map[string]string{"a": "https://demo@git.one.example/a.git", "b": "https://demo@git.two.example/b.git"} {
		dir := filepath.Join(w.home, "mixed", name)
		runGit(t, w.home, "init", "-q", dir)
		runGit(t, dir, "remote", "add", "origin", url)
	}
	env, _ := w.env(t, w.home)

	proposal, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range proposal.Profiles {
		if !p.onGitHub() {
			t.Errorf("an identity used on two hosts should not become a profile: %+v", p)
		}
	}
}

// appendRule adds a hand-made includeIf rule for ~/<dir> with the given file
// content to the test home.
func appendRule(t *testing.T, w world, dir, content string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(w.home, ".gitconfig"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString("[includeIf \"gitdir:~/" + dir + "/\"]\n\tpath = ~/.gitconfig-" + dir + "\n"); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(w.home, ".gitconfig-"+dir), content, 0o600)
}

func TestOneEmailForGitHubAndGitLabKeepsEachFolderWithItsHost(t *testing.T) {
	hub := "[user]\n\tname = Demo\n\temail = me@demo.example\n[credential \"https://github.com\"]\n\tusername = acme-alex\n"
	lab := "[user]\n\tname = Demo\n\temail = me@demo.example\n[credential \"https://gitlab.com\"]\n\tusername = demo-lab\n"
	for _, order := range [][2]string{{"hub", "lab"}, {"lab", "hub"}} {
		w := newWorld(t)
		withoutConfig(t, w)
		files := map[string]string{"hub": hub, "lab": lab}
		appendRule(t, w, order[0], files[order[0]])
		appendRule(t, w, order[1], files[order[1]])
		env, _ := w.env(t, w.home)

		proposal, err := ProposeSetup(context.Background(), env)
		if err != nil {
			t.Fatal(err)
		}

		labProfile := profileFor(t, proposal, "gitlab.com")
		acme := proposal.Profiles[slices.IndexFunc(proposal.Profiles, func(p SetupProfile) bool { return p.ID == "acme-alex" })]
		if !slices.Equal(labProfile.Dirs, []string{"~/lab"}) || !slices.Equal(acme.Dirs, []string{"~/hub"}) {
			t.Errorf("order %v: GitLab dirs %v, acme-alex dirs %v; each folder belongs to its own host", order, labProfile.Dirs, acme.Dirs)
		}
	}
}

func TestAnAccountTheConfigurationCannotHoldStaysTheUsersRule(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	appendRule(t, w, "azure", "[user]\n\temail = me@corp.example\n[credential \"https://dev.azure.example\"]\n\tusername = me@corp.example\n")
	env, _ := w.env(t, w.home)

	proposal, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range proposal.Profiles {
		if !p.onGitHub() {
			t.Errorf("an email as the account cannot be a profile: %+v", p)
		}
	}
	if slices.Contains(proposal.OldRules, "~/azure") {
		t.Error("the rule no profile takes over must stay")
	}
	if err := ApplySetup(context.Background(), env, proposal.Plan()); err != nil {
		t.Errorf("setup must still apply: %v", err)
	}
}

func TestARemoteOnAnotherPortKeepsThePort(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	appendRule(t, w, "corp", "[user]\n\tname = Corp\n\temail = me@corp.example\n")
	runGit(t, w.home, "init", "-q", filepath.Join(w.home, "corp", "api"))
	runGit(t, filepath.Join(w.home, "corp", "api"), "remote", "add", "origin", "https://demo-corp@git.corp.example:8443/team/api.git")
	env, _ := w.env(t, w.home)

	proposal, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	profileFor(t, proposal, "git.corp.example:8443")
	if err := ApplySetup(context.Background(), env, proposal.Plan()); err != nil {
		t.Fatal(err)
	}

	got := runGit(t, filepath.Join(w.home, "corp", "api"), "config", "--get-urlmatch", "credential.username", "https://git.corp.example:8443/team/api.git")
	if got != "demo-corp" {
		t.Errorf("pushes to the port push as %q, want demo-corp", got)
	}
}

func TestAHostRuleThatDoesMoreThanAProfileMakesNoEmptyProfile(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	appendRule(t, w, "lab", "[user]\n\temail = lab@demo.example\n[credential \"https://gitlab.com\"]\n\tusername = demo-lab\n"+
		"[url \"git@gitlab-work:\"]\n\tinsteadOf = https://gitlab.com/\n")
	env, _ := w.env(t, w.home)

	proposal, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range proposal.Profiles {
		if !p.onGitHub() {
			t.Errorf("a profile without folders would do nothing: %+v", p)
		}
	}
}

func TestSetupWithoutGhSaysWhyThereAreNoGitHubProfiles(t *testing.T) {
	w, env := hostWorld(t, "demo-lab", "lab@demo.example")
	t.Setenv("PATH", w.pathWithoutGH)
	var out strings.Builder
	env.Stdout = &out

	proposal, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	env.printProposal(proposal)

	if !strings.Contains(proposal.GitHubUnavailable, "GitHub CLI") || !strings.Contains(out.String(), "No GitHub profiles: setup needs the GitHub CLI") {
		t.Errorf("the proposal should say why: %q\n%s", proposal.GitHubUnavailable, out.String())
	}
	if !strings.Contains(out.String(), "account demo-lab on gitlab.com (gitlab)") {
		t.Errorf("the GitLab profile should show its host:\n%s", out.String())
	}
}
