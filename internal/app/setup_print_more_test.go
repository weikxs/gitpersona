package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/config"
	"github.com/weikxs/gitpersona/internal/discover"
)

func demoSetupProfile(id, account string) SetupProfile {
	return SetupProfile{ID: id, Account: account, Name: "Alex Demo", Email: account + "@demo.example", Dirs: []string{}}
}

func printedProposal(t *testing.T, p SetupProposal) string {
	t.Helper()
	w := newWorld(t)
	env, out := w.env(t, w.home)
	env.printProposal(p)
	return out.String()
}

func TestPrintProposalExplainsAnIncompleteOrTooDeepScan(t *testing.T) {
	p := SetupProposal{
		Profiles:          []SetupProfile{demoSetupProfile("octo-alex", "octo-alex")},
		ScanIncomplete:    true,
		ScanUnfinished:    []string{"~/Projects/huge"},
		ScanTooDeep:       true,
		GitHubUnavailable: "gh is not installed.",
	}

	got := printedProposal(t, p)

	for _, want := range []string{
		"GitHub accounts were not looked up: gh is not installed.",
		"Looking for repositories stopped after",
		"~/Projects/huge",
		"nested more than",
		"no directories yet",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the proposal should say %q:\n%s", want, got)
		}
	}
}

func TestPrintProposalDescribesTheDefaultProfileAndWhatItReplaces(t *testing.T) {
	def := demoSetupProfile("acme-alex", "acme-alex")
	def.Signing = config.Signing{Method: config.SigningSSH, Key: "~/.ssh/id_demo.pub"}
	def.SSHKey = "~/.ssh/id_demo"
	other := demoSetupProfile("octo-alex", "octo-alex")
	other.Dirs = []string{"~/Projects/personal"}
	p := SetupProposal{
		Default:  "acme-alex",
		Profiles: []SetupProfile{def, other},
		Global:   GlobalIdentity{Name: "Global Demo", Email: "global@demo.example"},
		OwnRules: []string{"~/oss (oss@demo.example)"},
		Pins:     []PinSuggestion{{Repo: "~/Desktop/scratch", Profile: "octo-alex", Why: "its own settings name octo-alex"}},
	}

	got := printedProposal(t, p)

	for _, want := range []string{
		"acme-alex (default): Alex Demo <acme-alex@demo.example>",
		"commits will use acme-alex (Alex Demo <acme-alex@demo.example>) instead of Global Demo <global@demo.example>",
		"commits will also be signed (ssh), as acme-alex signs every commit",
		"Git also connects with acme-alex's SSH key ~/.ssh/id_demo",
		"The default profile acme-alex overrides your own includeIf rules everywhere: ~/oss (oss@demo.example)",
		"Pin ~/Desktop/scratch to octo-alex",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the proposal should say %q:\n%s", want, got)
		}
	}
}

func TestPrintProposalSaysWhyThereIsNoDefaultWhileOwnRulesExist(t *testing.T) {
	p := SetupProposal{
		Profiles: []SetupProfile{demoSetupProfile("octo-alex", "octo-alex")},
		OwnRules: []string{"~/oss (oss@demo.example)"},
	}

	got := printedProposal(t, p)

	if !strings.Contains(got, "No default profile") || !strings.Contains(got, "gitpersona setup --default <profile>") {
		t.Errorf("the proposal should explain the missing default:\n%s", got)
	}
}

func TestPrintProposalListsRulesItKeepsReplacesAndLeavesOut(t *testing.T) {
	octo := demoSetupProfile("octo-alex", "octo-alex")
	octo.Dirs = []string{"~/Projects/personal"}
	octo.NameSource = "global"
	octo.Notes = []string{"the SSH key ~/.ssh/id_gone of your includeIf setup does not exist, so it is left out"}
	gitlab := demoSetupProfile("globex-alex", "globex-alex")
	gitlab.Provider, gitlab.Host, gitlab.NameSource = "gitlab", "gitlab.globex.example", "login"
	gitlab.Signing = config.Signing{Method: config.SigningGPG, Key: "ABCD1234"}
	p := SetupProposal{
		Profiles: []SetupProfile{octo, gitlab},
		OldRules: []string{"~/Projects/personal"},
		KeptRules: []KeptRule{
			{Dir: "~/work", Why: "it sets core.sshCommand", OverriddenBy: "octo-alex"},
			{Dir: "~/oss", Why: "it sets commit.gpgsign", ReplacedIn: []string{"~/oss/mine"}},
		},
		Shadowed: []string{"your rule for ~/old never applied"},
		Shells:   []string{"fish"},
		GhSwitchers: []string{
			"~/.config/fish/conf.d/gh-switch.fish",
		},
		Unassigned: []UnassignedDir{{Dir: "~/Projects/clients", Owner: "client-org", Why: "3 repositories"}},
		Unmatched: []LeftOut{
			{Identity: discover.Identity{Name: "Old Demo", Email: "old@demo.example", Dirs: []string{"~/never"}, Source: "~/.gitconfig-old"}, NeverApplied: "a later rule overrides it"},
			{Identity: discover.Identity{Name: "Cover Demo", Email: "cover@demo.example", Dirs: []string{"~/Projects/personal"}, Source: "~/.gitconfig-cover"}, OverriddenBy: "octo-alex"},
			{Identity: discover.Identity{Name: "Free Demo", Email: "free@demo.example", Dirs: []string{"~/free"}, Source: "~/.gitconfig-free"}, ReplacedIn: []string{"~/free/inner"}},
		},
	}

	got := printedProposal(t, p)

	for _, want := range []string{
		"The profiles take over your includeIf rules for ~/Projects/personal",
		"does more than a profile can (it sets core.sshCommand), but profile octo-alex applies there",
		"Kept your includeIf rule for ~/oss",
		"Except in ~/oss/mine, which go to profiles",
		"! Changed: your rule for ~/old never applied.",
		"Shell integration for fish",
		"Owned by someone else",
		"~/Projects/clients  (3 repositories, owned by client-org)",
		"Not taken over: Old Demo <old@demo.example> for ~/never",
		"Replaced: Cover Demo <cover@demo.example> for ~/Projects/personal",
		"Not taken over: Free Demo <free@demo.example> for ~/free",
		"Except in ~/free/inner, which go to profiles.",
		"(your includeIf setup sets no name, so the name is your global Git name)",
		"(the account shows no public name, so the name is its login)",
		"account globex-alex on gitlab.globex.example (gitlab)",
		"signs commits (gpg)",
		"! the SSH key ~/.ssh/id_gone",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the proposal should say %q:\n%s", want, got)
		}
	}
}

func TestPrintedConfigurationListsWhatItLeavesOutAndKeepsAsComments(t *testing.T) {
	p := SetupProposal{
		KeptRules: []KeptRule{{Dir: "~/oss", Why: "it sets commit.gpgsign"}},
		Unmatched: []LeftOut{{Identity: discover.Identity{Name: "Free Demo", Email: "free@demo.example", Dirs: []string{"~/free"}}}},
	}
	var out bytes.Buffer

	printLeftOutComments(&out, p, strings.ToLower)

	for _, want := range []string{
		"# Left out: Free Demo <free@demo.example> for ~/free",
		"# Kept: your includeIf rule for ~/oss (it sets commit.gpgsign).",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output should hold %q:\n%s", want, out.String())
		}
	}
}

func TestRenderPlanWritesSigningNoneAndCountsIdentities(t *testing.T) {
	prof := demoSetupProfile("octo-alex", "octo-alex")
	prof.Signing = config.Signing{Method: config.SigningNone}
	plan := SetupPlan{Profiles: []SetupProfile{prof}}

	text := renderPlan(plan)

	if !strings.Contains(text, `signing = { method = "none" }`) {
		t.Errorf("rendered plan lacks the signing opt-out:\n%s", text)
	}
	if identities(1) != "1 identity" || identities(2) != "2 identities" {
		t.Errorf("identities(1) = %q, identities(2) = %q", identities(1), identities(2))
	}
}

func TestProfileOfAnUnknownAccountIsAStandInNamedAfterTheAccount(t *testing.T) {
	got := profileOf([]SetupProfile{demoSetupProfile("octo-alex", "octo-alex")}, "somebody")

	if got.ID != "somebody" || got.Account != "somebody" {
		t.Errorf("profileOf(unknown) = %+v", got)
	}
}

func TestAnIdentityWithoutANameCommitsAsTheGlobalName(t *testing.T) {
	prof := demoSetupProfile("octo-alex", "octo-alex")
	prof.Name, prof.Email = "Global Demo", "id@demo.example"

	if !sameAuthor(prof, discover.Identity{Email: "id@demo.example"}, "Global Demo") {
		t.Error("an identity without a name should commit as the global name")
	}
	if sameAuthor(prof, discover.Identity{Email: "id@demo.example"}, "Someone Else") {
		t.Error("an identity without a name must not match a profile with another name")
	}
}

func TestAnIdentityNamingTwoHostsHasNoSingleHostAccount(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	id := discover.Identity{HostAccounts: map[string]string{"gitlab.globex.example": "globex-alex", "git.acme.example": "acme-alex"}}

	if _, ok := env.hostAccountOf(id, nil, nil); ok {
		t.Error("an identity naming two hosts should have no host account")
	}
}
