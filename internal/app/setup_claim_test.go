package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/discover"
	"github.com/weikxs/gitpersona/internal/shell"
)

func TestInferLayoutPinsRepositoriesOfAnotherAccountInSubfolders(t *testing.T) {
	profiles := []SetupProfile{
		{ID: "work", Account: "demo-work", Email: "worker@demo.example"},
		{ID: "personal", Account: "demo-personal", Email: "person@demo.example"},
	}
	repos := []discover.Repo{
		{Dir: "~/code/app", Host: "github.com", Owner: "demo-work"},
		// Two each in a subfolder: no rule of its own, so ~/code's rule applies.
		{Dir: "~/code/mix/z", Host: "github.com", Owner: "demo-personal", User: "demo-personal"},
		{Dir: "~/code/mix/w", Host: "github.com", Owner: "demo-work"},
		{Dir: "~/code/mix/y", Host: "github.com", Owner: "demo-personal"},
		{Dir: "~/code/mix/x", Host: "github.com", Owner: "demo-work"},
	}

	got := inferLayout(profiles, repos, nil, testKey, layoutHints{})

	wantPins := []PinSuggestion{
		{Repo: "~/code/mix/y", Profile: "personal", Why: "the repository belongs to demo-personal"},
		{Repo: "~/code/mix/z", Profile: "personal", Why: "the repository signs in as demo-personal"},
	}
	if f := got.found["demo-work"]; len(f) != 1 || f[0].Dir != "~/code" {
		t.Fatalf("found = %+v, want ~/code for demo-work", got.found)
	}
	if !reflect.DeepEqual(got.pins, wantPins) {
		t.Errorf("pins = %+v\nwant %+v", got.pins, wantPins)
	}
}

func TestInferLayoutPinsLoneRepositoriesInsteadOfMakingFolderRules(t *testing.T) {
	profiles := []SetupProfile{
		{ID: "work", Account: "demo-work", Email: "worker@demo.example"},
		{ID: "personal", Account: "demo-personal", Email: "person@demo.example"},
	}
	repos := []discover.Repo{
		// A scanned folder that is a repository itself: its parent is the home.
		{Dir: "~/src", Host: "github.com", Owner: "demo-personal", Lone: true},
		{Dir: "~/Projects/outer", Host: "github.com", Owner: "demo-personal"},
		{Dir: "~/Projects/other", Host: "github.com", Owner: "demo-personal"},
		// A repository kept inside another one.
		{Dir: "~/Projects/outer/inner", Host: "github-work", Owner: "demo-work", Lone: true},
	}
	aliases := []discover.SSHAlias{{Alias: "github-work"}}

	got := inferLayout(profiles, repos, aliases, testKey, layoutHints{})

	if len(got.found["demo-work"]) != 0 || len(got.found["demo-personal"]) != 1 || got.found["demo-personal"][0].Dir != "~/Projects" {
		t.Errorf("found = %+v; neither ~ nor a repository may become a folder rule", got.found)
	}
	wantPins := []PinSuggestion{
		{Repo: "~/Projects/outer/inner", Profile: "work", Why: "the repository uses demo-work's SSH alias github-work"},
		{Repo: "~/src", Profile: "personal", Why: "the repository belongs to demo-personal", Outside: true},
	}
	if !reflect.DeepEqual(got.pins, wantPins) {
		t.Errorf("pins = %+v\nwant %+v", got.pins, wantPins)
	}
}

func TestReasonsNameEveryAliasTheRepositoriesUse(t *testing.T) {
	profiles := []SetupProfile{{ID: "work", Account: "demo-work", Email: "worker@demo.example", SSHKey: "~/.ssh/id_work"}}
	repos := []discover.Repo{
		{Dir: "~/code/r1", Host: "gh-w", Owner: "demo-work"},
		{Dir: "~/code/r2", Host: "github-missing", Owner: "demo-work"},
		{Dir: "~/code/r3", Host: "github.com", Owner: "demo-work"},
	}
	aliases := []discover.SSHAlias{{Alias: "gh-w", IdentityFile: "~/.ssh/id_work"}, {Alias: "github-missing", IdentityFile: "~/.ssh/id_work"}}

	got := inferLayout(profiles, repos, aliases, testKey, layoutHints{})

	want := "all 3 repositories belong to demo-work; 1 uses demo-work's SSH alias gh-w, 1 uses demo-work's SSH alias github-missing"
	if f := got.found["demo-work"]; len(f) != 1 || f[0].Why != want {
		t.Errorf("found = %+v, want %q", f, want)
	}
}

// identityWorld is a home with a hand-made includeIf identity for ~/oss that
// names no account: only its SSH key and its repositories tie it to one.
func identityWorld(t *testing.T, sshConfig string) (world, Env) {
	t.Helper()
	w := newWorld(t)
	withoutConfig(t, w)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[user]\n\tname = Global Demo\n\temail = global@demo.example\n"+
		"[includeIf \"gitdir/i:~/oss/\"]\n\tpath = ~/.gitconfig-oss\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gitconfig-oss"), "[user]\n\tname = Demo Personal\n\temail = personal@demo.example\n"+
		"[core]\n\tsshCommand = ssh -i ~/.ssh/id_personal -o IdentitiesOnly=yes\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".ssh", "id_personal"), "KEY PLACEHOLDER (never read)\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".ssh", "config"), sshConfig, 0o600)
	env, _ := w.env(t, w.home)
	return w, env
}

func TestAnIdentityIsTiedToTheAccountWhoseSSHAliasUsesItsKey(t *testing.T) {
	_, env := identityWorld(t, "Host github-personal\n  HostName github.com\n  IdentityFile ~/.ssh/id_personal\n")
	// No repository says whose github-personal is; the key it shares with the
	// identity, and a repository in ~/work using the alias, do.
	w := world{home: env.Home}
	repo := filepath.Join(w.home, "work", "odd")
	runGit(t, w.home, "init", "-q", repo)
	runGit(t, repo, "remote", "add", "origin", "git@github-personal:octo-alex/odd.git")

	got, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	octo := got.Profiles[0]
	if octo.Account != "octo-alex" || octo.Email != "personal@demo.example" || !slices.Contains(octo.Dirs, "~/oss") || octo.SSHKey != "~/.ssh/id_personal" {
		t.Errorf("octo-alex = %+v, want the ~/oss identity with its key", octo)
	}
	if len(got.Unmatched) != 0 || !slices.Equal(got.OldRules, []string{"~/oss"}) {
		t.Errorf("unmatched = %+v, old rules = %v; the ~/oss rule is taken over", got.Unmatched, got.OldRules)
	}
}

func TestAnIdentityOnlyItsRepositoriesExplainIsTiedThroughThem(t *testing.T) {
	w, env := identityWorld(t, "")
	for _, name := range []string{"tool", "site"} {
		repo := filepath.Join(w.home, "oss", name)
		runGit(t, w.home, "init", "-q", repo)
		runGit(t, repo, "remote", "add", "origin", "https://github.com/octo-alex/"+name+".git")
	}

	got, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Contains(got.Roots, "~/oss") {
		t.Errorf("roots = %v; the folders of existing rules are scanned too", got.Roots)
	}
	if octo := got.Profiles[0]; octo.Email != "personal@demo.example" || !slices.Contains(octo.Dirs, "~/oss") {
		t.Errorf("octo-alex = %+v, want the ~/oss identity", octo)
	}
}

func TestAnIdentityNothingTiesToAnAccountKeepsWorkingInItsFolder(t *testing.T) {
	w, env := identityWorld(t, "")
	repo := filepath.Join(w.home, "oss", "notes")
	runGit(t, w.home, "init", "-q", repo)
	out := &strings.Builder{}
	env.Stdout = out

	got, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Unmatched) != 1 || got.Unmatched[0].OverriddenBy != "" || got.Default != "" || got.SuggestedDefault == "" ||
		!slices.Equal(got.Unmatched[0].Replaceable, []string{"~/oss"}) || len(got.OldRules) != 0 {
		t.Fatalf("proposal = %+v; want ~/oss left to its own rule and no default", got)
	}
	if err := Setup(context.Background(), env, SetupOptions{Yes: true, NoShell: true}); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"Not taken over: Demo Personal <personal@demo.example> for ~/oss",
		"your rule keeps applying there",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("setup should say %q:\n%s", want, out.String())
		}
	}
	if got := runGit(t, repo, "config", "user.email"); got != "personal@demo.example" {
		t.Errorf("~/oss/notes commits as %q after setup, want personal@demo.example as before", got)
	}
}

func TestADefaultChosenAnywayIsAnnouncedAsOverridingYourRules(t *testing.T) {
	_, env := identityWorld(t, "")
	out := &strings.Builder{}
	env.Stdout = out
	env.Stdin = strings.NewReader("n\n")

	if err := Setup(context.Background(), env, SetupOptions{Default: "acme-alex"}); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"! The default profile acme-alex overrides your own includeIf rules everywhere",
		"! Replaced: Demo Personal <personal@demo.example> for ~/oss"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("setup --default should say %q:\n%s", want, out.String())
		}
	}
	if err := Setup(context.Background(), env, SetupOptions{Default: "nobody", Print: true}); err == nil {
		t.Error("an unknown --default must be refused")
	}
}

func TestAMissingKeyOfAnExistingIdentityIsLeftOutWithANote(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	mustWrite(t, filepath.Join(w.home, ".gitconfig-personal"), "[user]\n\tname = Alex Personal\n\temail = alex.personal@example.com\n"+
		"[credential \"https://github.com\"]\n\tusername = octo-alex\n[core]\n\tsshCommand = ssh -i ~/.ssh/id_absent\n", 0o600)
	env, _ := w.env(t, w.home)

	got, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	octo := got.Profiles[0]
	if octo.SSHKey != "" || len(octo.Notes) != 1 || !strings.Contains(octo.Notes[0], "~/.ssh/id_absent") {
		t.Errorf("octo-alex = %+v, want the missing key left out with a note", octo)
	}
}

func TestADeclinedShellIsRememberedAndDoctorDoesNotPushIt(t *testing.T) {
	_, env := autoWorld(t)
	proposal, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	proposal.Shells, proposal.NoShell = []string{}, true // the zsh offered was unticked

	if err := ApplySetup(context.Background(), env, proposal.Plan()); err != nil {
		t.Fatal(err)
	}

	if cfg := readFile(t, env.configFile()); !strings.Contains(cfg, "shell_integration = false") {
		t.Errorf("the configuration should record that the shell was declined:\n%s", cfg)
	}
	out := &strings.Builder{}
	env.Stdout = out
	env.LoginShell = func(context.Context) shell.Detected { return shell.Detected{Kind: shell.Zsh} }
	DoctorWith(context.Background(), env, DoctorOptions{Fix: true})
	zshrc, _ := os.ReadFile(filepath.Join(env.Home, ".zshrc"))
	if strings.Contains(out.String(), "integration is not installed") || strings.Contains(string(zshrc), "gitpersona") {
		t.Errorf("doctor must not push the shell integration the user declined:\n%s", out.String())
	}
}

func TestOneClonedRepositoryDoesNotTieAnIdentityToItsOwner(t *testing.T) {
	w, env := identityWorld(t, "")
	repo := filepath.Join(w.home, "oss", "clone")
	runGit(t, w.home, "init", "-q", repo)
	runGit(t, repo, "remote", "add", "origin", "https://github.com/acme-alex/clone.git")

	got, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range got.Profiles {
		if p.Email == "personal@demo.example" {
			t.Errorf("%s took the ~/oss identity from a single clone it owns", p.ID)
		}
	}
	if len(got.Unmatched) != 1 {
		t.Errorf("unmatched = %+v, want the ~/oss identity left for the user to choose", got.Unmatched)
	}
}

func TestTheStrongestEvidenceClaimsAnAccountFirst(t *testing.T) {
	profiles := []SetupProfile{{ID: "octo", Account: "octo", Email: "octo@demo.example"}}
	ids := []discover.Identity{
		{Email: "weak@demo.example", Dirs: []string{"~/a"}},
		{Email: "strong@demo.example", Dirs: []string{"~/b"}, SSHKey: "~/.ssh/id_octo"},
	}
	repos := []discover.Repo{
		{Dir: "~/a/x", Host: "github.com", Owner: "octo"},
		{Dir: "~/a/y", Host: "github.com", Owner: "octo"},
		{Dir: "~/b/z", Host: "github-octo", Owner: "octo"},
	}
	aliases := []discover.SSHAlias{{Alias: "github-octo", IdentityFile: "~/.ssh/id_octo"}}
	in := newInference(profiles, repos, aliases, testKey)

	got := claimIdentities(in, ids, []bool{false, false}, repos, aliases)

	if !reflect.DeepEqual(got, map[int]string{1: "octo"}) {
		t.Errorf("claims = %v, want the identity using octo's key (1), not the first one listed", got)
	}
}

func TestARepositoryOfAnotherAccountInsideARuleIsPinned(t *testing.T) {
	profiles := []SetupProfile{
		{ID: "work", Account: "demo-work", Email: "worker@demo.example", Dirs: []string{"~/work"}},
		{ID: "personal", Account: "demo-personal", Email: "person@demo.example"},
	}
	repos := []discover.Repo{
		{Dir: "~/work/team/a", Host: "github.com", Owner: "demo-work"},
		{Dir: "~/work/team/b", Host: "github.com", Owner: "demo-work"},
		{Dir: "~/work/team/fork", Host: "github.com", Owner: "demo-personal"},
	}

	got := inferLayout(profiles, repos, nil, testKey, layoutHints{})

	// Without the pin, pushing to demo-personal's fork would use work's credentials.
	if len(got.pins) != 1 || got.pins[0].Repo != "~/work/team/fork" || got.pins[0].Profile != "personal" {
		t.Errorf("pins = %+v, want the fork pinned to personal", got.pins)
	}
}

func TestPinsOutsideEveryRuleFollowTheDefault(t *testing.T) {
	p := SetupProposal{Default: "work", Pins: []PinSuggestion{
		{Repo: "~/src", Profile: "personal", Outside: true},
		{Repo: "~/scratch", Profile: "work", Outside: true},
		{Repo: "~/code/x", Profile: "work"},
	}}

	repos := func(plan SetupPlan) []string {
		var out []string
		for _, pin := range plan.Pins {
			out = append(out, pin.Repo)
		}
		return out
	}
	if got := repos(p.Plan()); !slices.Equal(got, []string{"~/src", "~/code/x"}) {
		t.Errorf("with default work, pins = %v", got)
	}
	p.Default = ""
	if got := repos(p.Plan()); !slices.Equal(got, []string{"~/src", "~/scratch", "~/code/x"}) {
		t.Errorf("without a default, pins = %v; Git's own identity would apply to them", got)
	}
}

func TestAnAliasUsedForTwoAccountsIsNobodys(t *testing.T) {
	profiles := []SetupProfile{
		{ID: "work", Account: "demo-work", Email: "worker@demo.example"},
		{ID: "personal", Account: "demo-personal", Email: "person@demo.example"},
	}
	repos := []discover.Repo{
		{Dir: "~/code/a", Host: "gh-w", Owner: "demo-work"},
		{Dir: "~/code/b", Host: "gh-w", Owner: "demo-work"},
		{Dir: "~/src/p", Host: "gh-w", Owner: "demo-personal"},
		{Dir: "~/src/q", Host: "gh-w", Owner: "demo-personal"},
	}

	got := inferLayout(profiles, repos, []discover.SSHAlias{{Alias: "gh-w", IdentityFile: "~/.ssh/id_w"}}, testKey, layoutHints{})

	if got.aliasAccount["gh-w"] != "" {
		t.Errorf("gh-w was tied to %q; its repositories belong to both accounts", got.aliasAccount["gh-w"])
	}
	if f := got.found["demo-work"]; len(f) != 1 || f[0].Dir != "~/code" || f[0].Why != "all 2 repositories belong to demo-work" {
		t.Errorf("found = %+v, want ~/code for demo-work by its owners", got.found)
	}
}

func TestNestedRulesForTheSameProfileAreMerged(t *testing.T) {
	profiles := []SetupProfile{
		{ID: "personal", Account: "demo-personal", Email: "person@demo.example"},
		{ID: "work", Account: "demo-work", Email: "worker@demo.example"},
	}
	repos := []discover.Repo{
		{Dir: "~/code/l1", Host: "github.com", Owner: "demo-personal"},
		{Dir: "~/code/a/l2", Host: "github.com", Owner: "demo-personal"},
		{Dir: "~/code/a/w/x", Host: "github.com", Owner: "demo-work"},
		{Dir: "~/code/a/w/y/l3", Host: "github.com", Owner: "demo-personal"},
	}

	got := inferLayout(profiles, repos, nil, testKey, layoutHints{})

	var dirs []string
	for _, f := range got.found["demo-personal"] {
		dirs = append(dirs, f.Dir)
	}
	// ~/code/a adds nothing to ~/code; ~/code/a/w/y sits inside work's ~/code/a/w and stays.
	if !slices.Equal(dirs, []string{"~/code", "~/code/a/w/y"}) {
		t.Errorf("personal's rules = %v", dirs)
	}
}

func TestALeftOutIdentitysFolderGetsNoRuleFromOneRepository(t *testing.T) {
	w, env := identityWorld(t, "")
	repo := filepath.Join(w.home, "oss", "one")
	runGit(t, w.home, "init", "-q", repo)
	runGit(t, repo, "remote", "add", "origin", "https://github.com/octo-alex/one.git")

	got, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range got.Profiles {
		for _, f := range p.Found {
			if strings.HasPrefix(f.Dir, "~/oss") {
				t.Errorf("%s got %s from one repository, overriding the identity left out there", p.ID, f.Dir)
			}
		}
	}
	for _, pin := range got.Pins {
		if strings.HasPrefix(pin.Repo, "~/oss") {
			t.Errorf("pin %+v overrides the identity left out there", pin)
		}
	}
}

func TestAnIdentityWithoutANameKeepsTheGlobalName(t *testing.T) {
	_, env := identityWorld(t, "Host github-personal\n  HostName github.com\n  IdentityFile ~/.ssh/id_personal\n")
	mustWrite(t, filepath.Join(env.Home, ".gitconfig-oss"), "[user]\n\temail = personal@demo.example\n"+
		"[core]\n\tsshCommand = ssh -i ~/.ssh/id_personal -o IdentitiesOnly=yes\n", 0o600)
	repo := filepath.Join(env.Home, "work", "odd")
	runGit(t, env.Home, "init", "-q", repo)
	runGit(t, repo, "remote", "add", "origin", "git@github-personal:octo-alex/odd.git")

	got, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	if octo := got.Profiles[0]; octo.Name != "Global Demo" || octo.NameSource != "global" {
		t.Errorf("octo-alex = %+v, want the global name Git used there before", octo)
	}
}

func TestSetupPrintsWhatItLeavesOutAndCanSkipTheDefault(t *testing.T) {
	_, env := identityWorld(t, "")
	out := &strings.Builder{}
	env.Stdout = out

	if err := Setup(context.Background(), env, SetupOptions{Print: true, NoDefault: true}); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "# Left out: Demo Personal <personal@demo.example> for ~/oss") {
		t.Errorf("--print should say what it leaves out:\n%s", out.String())
	}
	if strings.Contains(out.String(), "default =") {
		t.Errorf("--no-default should set no default:\n%s", out.String())
	}
}

func TestAFolderInsideALeftOutIdentitysFolderStillGetsItsRule(t *testing.T) {
	profiles := []SetupProfile{{ID: "work", Account: "demo-work", Email: "worker@demo.example"}}
	repos := []discover.Repo{
		{Dir: "~/code/one", Host: "github.com", Owner: "demo-work"},
		{Dir: "~/code/work/a", Host: "github.com", Owner: "demo-work"},
		{Dir: "~/code/work/b", Host: "github.com", Owner: "demo-work"},
	}

	got := inferLayout(profiles, repos, nil, testKey, layoutHints{reserved: []string{"~/code"}})

	if f := got.found["demo-work"]; len(f) != 1 || f[0].Dir != "~/code/work" {
		t.Errorf("found = %+v; ~/code keeps the user's rule, ~/code/work clearly belongs to work", got.found)
	}
	if len(got.pins) != 0 {
		t.Errorf("pins = %+v; ~/code/one keeps the user's rule", got.pins)
	}
}

func TestOneRepositoryDirectlyInAScannedFolderIsPinnedNotARule(t *testing.T) {
	profiles := []SetupProfile{
		{ID: "work", Account: "demo-work", Email: "worker@demo.example"},
		{ID: "personal", Account: "demo-personal", Email: "person@demo.example"},
	}
	repos := []discover.Repo{{Dir: "~/code/lone", Host: "github.com", Owner: "demo-personal"}}

	got := inferLayout(profiles, repos, nil, testKey, layoutHints{roots: []string{"~/code"}})

	if len(got.found) != 0 {
		t.Errorf("found = %+v; one repository does not make all of ~/code personal", got.found)
	}
	if len(got.pins) != 1 || got.pins[0].Repo != "~/code/lone" || !got.pins[0].Outside {
		t.Errorf("pins = %+v, want ~/code/lone pinned while the default is another profile", got.pins)
	}
}
