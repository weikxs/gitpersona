package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/block"
	"github.com/weikxs/gitpersona/internal/discover"
	"github.com/weikxs/gitpersona/internal/generate"
)

func TestInferLayoutUsesOwnersAliasesAndRepositoryEmails(t *testing.T) {
	profiles := []SetupProfile{
		{ID: "octo-alex", Account: "octo-alex", Email: "alex@octo.example", Dirs: []string{"~/old/personal"}},
		{ID: "acme-alex", Account: "acme-alex", Email: "1000002+acme-alex@users.noreply.github.com"},
	}
	repos := []discover.Repo{
		{Dir: "~/code/mine/blog", Host: "github.com", Owner: "octo-alex"},
		{Dir: "~/code/mine/dots", Host: "github.com", Owner: "octo-alex"},
		// An organization reached through acme's SSH alias.
		{Dir: "~/code/acme/billing", Host: "github-acme", Owner: "acme-corp"},
		{Dir: "~/code/acme/web", Host: "github-acme", Owner: "acme-corp"},
		{Dir: "~/code/acme/tools", Host: "github-acme", Owner: "acme-alex"},
		// Inside acme's folder, but the repository sets octo's email.
		{Dir: "~/code/acme/side", Host: "github.com", Owner: "octo-alex", Email: "Alex@Octo.example"},
		// An organization, but the repositories set octo's email.
		{Dir: "~/code/oss/lib", Host: "github.com", Owner: "some-org", Email: "alex@octo.example"},
		// Nobody's: the user must choose.
		{Dir: "~/code/other/x", Host: "github.com", Owner: "other-org"},
		{Dir: "~/code/other/y", Host: "github.com", Owner: "other-org"},
		// Already covered by the existing setup.
		{Dir: "~/old/personal/notes", Host: "github.com", Owner: "octo-alex"},
		// Not on GitHub.
		{Dir: "~/code/lab/tool", Host: "gitlab.com", Owner: "octo-alex"},
		{Dir: "~/code/local/scratch"},
	}
	aliases := []discover.SSHAlias{{Alias: "github-acme", IdentityFile: "~/.ssh/id_acme"}, {Alias: "github.com", IdentityFile: "~/.ssh/id"}}

	got := inferLayout(profiles, repos, aliases, testKey, nil)

	wantFound := map[string][]DirSuggestion{
		"octo-alex": {
			{Dir: "~/code/mine", Why: "all 2 repositories belong to octo-alex"},
			{Dir: "~/code/oss", Why: "its one repository belongs to octo-alex; it sets alex@octo.example"},
		},
		"acme-alex": {{Dir: "~/code/acme", Why: "3 of 4 repositories belong to acme-alex; all use acme-alex's SSH alias github-acme"}},
	}
	if !reflect.DeepEqual(got.found, wantFound) {
		t.Errorf("found = %+v\nwant  %+v", got.found, wantFound)
	}
	wantPins := []PinSuggestion{{Repo: "~/code/acme/side", Profile: "octo-alex", Why: "the repository sets alex@octo.example"}}
	if !reflect.DeepEqual(got.pins, wantPins) {
		t.Errorf("pins = %+v, want %+v", got.pins, wantPins)
	}
	wantUnassigned := []UnassignedDir{{Dir: "~/code/other", Owner: "other-org", Why: "all 2 repositories"}}
	if !reflect.DeepEqual(got.unassigned, wantUnassigned) {
		t.Errorf("unassigned = %+v, want %+v", got.unassigned, wantUnassigned)
	}
	if got.aliasAccount["github-acme"] != "acme-alex" || got.aliasAccount["github.com"] != "" {
		t.Errorf("aliases = %+v, want github-acme for acme-alex only", got.aliasAccount)
	}
}

// testKey compares paths like a case-insensitive disk with home /tmp/home.
func testKey(p string) string {
	return strings.ToLower(strings.Replace(p, "~", "/tmp/home", 1))
}

func TestInferLayoutNeverGivesAFolderTwoProfiles(t *testing.T) {
	profiles := []SetupProfile{
		{ID: "a", Account: "a", Email: "a@demo.example", Dirs: []string{"/tmp/home/Work"}},
		{ID: "b", Account: "b", Email: "b@demo.example"},
	}
	repos := []discover.Repo{
		{Dir: "~/work/x", Host: "github.com", Owner: "b"},
		{Dir: "~/work/y", Host: "github.com", Owner: "b"},
		{Dir: "~/work/z", Host: "github.com", Owner: "b", Email: "b@demo.example"},
	}

	got := inferLayout(profiles, repos, nil, testKey, nil)

	if len(got.found) != 0 {
		t.Errorf("found = %+v; ~/work already belongs to a (written /tmp/home/Work)", got.found)
	}
	if len(got.pins) != 1 || got.pins[0].Repo != "~/work/z" || got.pins[0].Profile != "b" {
		t.Errorf("pins = %+v, want only the repository that sets b's email", got.pins)
	}
}

func TestInferLayoutHandlesAliasesItCannotTieToAnAccount(t *testing.T) {
	profiles := []SetupProfile{
		{ID: "a", Account: "a", Email: "a@demo.example", SSHKey: "~/.ssh/id_a"},
		{ID: "b", Account: "b", Email: "b@demo.example"},
	}
	repos := []discover.Repo{
		// Behind an alias nobody's repositories explain: the folder is asked about.
		{Dir: "~/client/x", Host: "github-client", Owner: "client-org"},
		{Dir: "~/client/y", Host: "github-client", Owner: "client-org"},
		// The alias is written in another case in ~/.ssh/config, and uses a's key.
		{Dir: "~/work/x", Host: "github-work", Owner: "work-org"},
	}
	aliases := []discover.SSHAlias{{Alias: "github-client"}, {Alias: "GitHub-Work", IdentityFile: "~/.ssh/id_a"}}

	got := inferLayout(profiles, repos, aliases, testKey, nil)

	if len(got.unassigned) != 1 || got.unassigned[0].Dir != "~/client" || got.unassigned[0].Owner != "client-org" {
		t.Errorf("unassigned = %+v, want ~/client to be asked about", got.unassigned)
	}
	if f := got.found["a"]; len(f) != 1 || f[0].Dir != "~/work" {
		t.Errorf("found = %+v, want ~/work for a through its SSH key", got.found)
	}
}

func TestInferLayoutWithoutRepositoriesSuggestsNothing(t *testing.T) {
	got := inferLayout([]SetupProfile{{ID: "a", Account: "a", Email: "a@x.example"}}, nil, nil, testKey, nil)
	if len(got.found) != 0 || len(got.pins) != 0 || len(got.unassigned) != 0 {
		t.Fatalf("inferLayout() = %+v", got)
	}
}

// autoWorld is a home without a configuration whose repositories, SSH
// configuration and keys tell which account each folder belongs to.
func autoWorld(t *testing.T) (world, Env) {
	t.Helper()
	w := newWorld(t)
	withoutConfig(t, w)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[user]\n\tname = Demo Person\n\temail = person@demo.example\n", 0o600)
	for dir, url := range map[string]string{
		"code/mine/blog":    "https://github.com/octo-alex/blog.git",
		"code/acme/billing": "git@github-acme:acme-corp/billing.git",
		"code/acme/tools":   "git@github-acme:acme-alex/tools.git",
		"code/other/x":      "https://github.com/other-org/x.git",
	} {
		repoDir := filepath.Join(w.home, dir)
		runGit(t, w.home, "init", "-q", repoDir)
		runGit(t, repoDir, "remote", "add", "origin", url)
	}
	runGit(t, w.home, "init", "-q", filepath.Join(w.home, "code/acme/side"))
	runGit(t, filepath.Join(w.home, "code/acme/side"), "remote", "add", "origin", "https://github.com/octo-alex/side.git")
	runGit(t, filepath.Join(w.home, "code/acme/side"), "config", "user.email", "alex@octo.example")
	mustWrite(t, filepath.Join(w.home, ".ssh", "config"), "Host github-acme\n  HostName github.com\n  IdentityFile ~/.ssh/id_acme\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".ssh", "id_acme"), "KEY PLACEHOLDER (never read)\n", 0o600)
	env, _ := w.env(t, w.home)
	return w, env
}

func TestProposeSetupOrganizesEverythingItFinds(t *testing.T) {
	_, env := autoWorld(t)

	got, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	octo, acme := got.Profiles[0], got.Profiles[1]
	if len(octo.Found) != 1 || octo.Found[0].Dir != "~/code/mine" {
		t.Errorf("octo-alex found = %+v, want ~/code/mine", octo.Found)
	}
	if len(acme.Found) != 1 || acme.Found[0].Dir != "~/code/acme" || acme.SSHKey != "~/.ssh/id_acme" {
		t.Errorf("acme-alex = %+v, want ~/code/acme and the SSH alias's key", acme)
	}
	if len(got.Pins) != 1 || got.Pins[0].Repo != "~/code/acme/side" || got.Pins[0].Profile != "octo-alex" {
		t.Errorf("pins = %+v", got.Pins)
	}
	if len(got.Unassigned) != 1 || got.Unassigned[0].Dir != "~/code/other" {
		t.Errorf("unassigned = %+v", got.Unassigned)
	}
	if !slices.Contains(got.Roots, "~/code") || slices.Contains(got.Roots, "~/src") {
		t.Errorf("roots = %+v, want only the folders that exist", got.Roots)
	}
}

func TestTheProposalAppliesAsIsAndGitFollowsIt(t *testing.T) {
	w, env := autoWorld(t)
	proposal, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	if err := ApplySetup(context.Background(), env, proposal.Plan()); err != nil {
		t.Fatal(err)
	}

	for dir, want := range map[string]string{
		"code/mine/blog":    "alex@octo.example",
		"code/acme/billing": "1000002+acme-alex@users.noreply.github.com",
	} {
		if got := runGit(t, filepath.Join(w.home, dir), "config", "user.email"); got != want {
			t.Errorf("%s commits as %q, want %q", dir, got, want)
		}
	}
	// The pinned repository sets its own email; the pin shows in the account it pushes as.
	if got := runGit(t, filepath.Join(w.home, "code/acme/side"), "config", "credential.https://github.com.username"); got != "octo-alex" {
		t.Errorf("the pinned repository pushes as %q, want octo-alex", got)
	}
	if got := runGit(t, filepath.Join(w.home, "code/acme/billing"), "config", "core.sshCommand"); !strings.Contains(got, "id_acme") {
		t.Errorf("acme's repositories should use acme's SSH key, got %q", got)
	}
}

func TestSetupAddsTheChosenShellsInTheSameBackup(t *testing.T) {
	w, env := autoWorld(t)
	proposal, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	plan := proposal.Plan()
	plan.Shells = []string{"tcsh"}
	if err := ApplySetup(context.Background(), env, plan); err == nil || !strings.Contains(err.Error(), "tcsh") {
		t.Errorf("an unknown shell should be refused, got %v", err)
	}
	plan.Shells = []string{"zsh"}

	if err := ApplySetup(context.Background(), env, plan); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(readFile(t, filepath.Join(w.home, ".zshrc")), "gitpersona") {
		t.Error("~/.zshrc should load GitPersona")
	}
	if n, _ := backupCount(t, env); n != 1 {
		t.Errorf("setup with shells made %d backups, want 1", n)
	}
}

func TestSetupRefusesAKeyThatDoesNotExist(t *testing.T) {
	_, env := autoWorld(t)
	proposal, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	plan := proposal.Plan()
	plan.Profiles[0].SSHKey = "~/.ssh/missing"

	if err := ApplySetup(context.Background(), env, plan); err == nil || !strings.Contains(err.Error(), "~/.ssh/missing does not exist") {
		t.Fatalf("ApplySetup() with a missing key = %v", err)
	}
}

func TestSetupRetiresTheOldRulesItTakesOverAndKeepsTheRest(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w) // octo-alex's rule is taken over; the clients rule belongs to no account
	env, _ := w.env(t, w.home)
	proposal, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(proposal.OldRules, []string{"~/Projects/personal"}) {
		t.Fatalf("old rules = %v, want only the one taken over", proposal.OldRules)
	}

	if err := ApplySetup(context.Background(), env, proposal.Plan()); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, filepath.Join(w.home, ".gitconfig"))
	if strings.Contains(got, "gitdir:~/Projects/personal/") {
		t.Errorf("the rule GitPersona took over should be gone:\n%s", got)
	}
	if !strings.Contains(got, "gitdir:~/Projects/clients/") || !strings.Contains(got, "rules.gitconfig") {
		t.Errorf("the unclaimed rule and GitPersona's include should stay:\n%s", got)
	}
	if n, _ := backupCount(t, env); n != 1 {
		t.Errorf("setup made %d backups, want 1", n)
	}
	if got := runGit(t, w.repos["blog"], "config", "user.email"); got != "alex.personal@example.com" {
		t.Errorf("the taken-over directory commits as %q", got)
	}
}

func TestSetupKeepsAnOldRuleThatDoesMoreThanAProfile(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	mustWrite(t, filepath.Join(w.home, ".gitconfig-personal"), "[user]\n\tname = Alex Personal\n\temail = alex.personal@example.com\n"+
		"[credential \"https://github.com\"]\n\tusername = octo-alex\n[url \"git@github-personal:\"]\n\tinsteadOf = https://github.com/\n", 0o600)
	env, out := w.env(t, w.home)

	if err := Setup(context.Background(), env, SetupOptions{Yes: true, NoShell: true}); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, filepath.Join(w.home, ".gitconfig")); !strings.Contains(got, "gitdir:~/Projects/personal/") {
		t.Errorf("a rule that also rewrites URLs must stay:\n%s", got)
	}
	if !strings.Contains(out.String(), "Kept your includeIf rule for ~/Projects/personal: it also sets url.git@github-personal:.insteadof") {
		t.Errorf("setup should say why the rule stays:\n%s", out.String())
	}
}

func TestTwoRootsReachingTheSameFolderCountItsRepositoriesOnce(t *testing.T) {
	w, env := autoWorld(t)
	if err := os.Symlink(filepath.Join(w.home, "code", "mine"), filepath.Join(w.home, "work")); err != nil {
		t.Fatal(err)
	}

	for range 5 {
		got, err := ProposeSetup(context.Background(), env)
		if err != nil {
			t.Fatal(err)
		}
		if f := got.Profiles[0].Found; len(f) != 1 || f[0].Dir != "~/code/mine" || f[0].Why != "its one repository belongs to octo-alex" {
			t.Fatalf("found = %+v, want ~/code/mine once, with its one repository", f)
		}
	}
}

func TestSetupRetiresOldRulesWhenTheIncludeIsAlreadyThere(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	env, _ := w.env(t, w.home)
	gitconfig := filepath.Join(w.home, ".gitconfig")
	mustWrite(t, gitconfig, readFile(t, gitconfig)+block.Render(generate.IncludeBody(env.includePath())), 0o600)
	proposal, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	if err := ApplySetup(context.Background(), env, proposal.Plan()); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, gitconfig); strings.Contains(got, "gitdir:~/Projects/personal/") {
		t.Errorf("the rule setup said it removes is still there:\n%s", got)
	}
}
