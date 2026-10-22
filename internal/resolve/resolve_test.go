package resolve

import (
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/config"
)

const home = "/tmp/home"

// demoConfig mirrors the example in the product requirements.
func demoConfig() config.Config {
	return config.Config{
		Version: config.CurrentVersion,
		Default: "acme",
		Profiles: []config.Profile{
			{ID: "personal", Account: "octo-alex", Dirs: []string{"~/Projects/personal", "~/Projects/acme/oss"}},
			{ID: "acme", Account: "acme-alex", Dirs: []string{"~/Projects/acme"}},
			{ID: "globex", Account: "globex-alex", Dirs: []string{"~/Projects/clients/globex"}},
		},
		Pins: []config.Pin{{Repo: "~/Downloads/globex-hotfix", Profile: "globex"}},
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name        string
		repo        string
		wantProfile string
		wantReason  Reason
		wantMatch   string
	}{
		{name: "directory rule", repo: "~/Projects/personal/blog", wantProfile: "personal", wantReason: ReasonRule, wantMatch: "~/Projects/personal"},
		{name: "rule on the directory itself", repo: "~/Projects/acme", wantProfile: "acme", wantReason: ReasonRule, wantMatch: "~/Projects/acme"},
		{name: "work directory rule", repo: "~/Projects/acme/billing", wantProfile: "acme", wantReason: ReasonRule, wantMatch: "~/Projects/acme"},
		{name: "more specific nested rule wins", repo: "~/Projects/acme/oss/sdk", wantProfile: "personal", wantReason: ReasonRule, wantMatch: "~/Projects/acme/oss"},
		{name: "pin wins over everything", repo: "~/Downloads/globex-hotfix", wantProfile: "globex", wantReason: ReasonPin, wantMatch: "~/Downloads/globex-hotfix"},
		{name: "pin is exact, not a prefix", repo: "~/Downloads/globex-hotfix/vendor/lib", wantProfile: "acme", wantReason: ReasonDefault},
		{name: "unmatched uses default", repo: "~/Desktop/scratch", wantProfile: "acme", wantReason: ReasonDefault},
		{name: "common prefix is not a match", repo: "~/Projects/personal2/demo", wantProfile: "acme", wantReason: ReasonDefault},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Resolve(demoConfig(), expand(tc.repo), Options{Home: home})

			if !got.Found || got.Profile.ID != tc.wantProfile || got.Reason != tc.wantReason || got.Match != tc.wantMatch {
				t.Fatalf("Resolve(%s) = {found:%v profile:%q reason:%q match:%q}, want {profile:%q reason:%q match:%q}",
					tc.repo, got.Found, got.Profile.ID, got.Reason, got.Match, tc.wantProfile, tc.wantReason, tc.wantMatch)
			}
		})
	}
}

func TestResolveWithoutDefaultLeavesGitAlone(t *testing.T) {
	cfg := demoConfig()
	cfg.Default = ""

	got := Resolve(cfg, expand("~/Desktop/scratch"), Options{Home: home})

	if got.Found || got.Reason != ReasonNone {
		t.Fatalf("Resolve() = %+v, want not found with reason none", got)
	}
}

func TestResolveCaseInsensitive(t *testing.T) {
	repo := expand("~/projects/ACME/billing")

	sensitive := Resolve(demoConfig(), repo, Options{Home: home})
	insensitive := Resolve(demoConfig(), repo, Options{Home: home, CaseInsensitive: true})

	if sensitive.Reason != ReasonDefault {
		t.Errorf("case-sensitive reason = %q, want default", sensitive.Reason)
	}
	if insensitive.Reason != ReasonRule || insensitive.Profile.ID != "acme" {
		t.Errorf("case-insensitive = %+v, want rule acme", insensitive)
	}
}

func TestResolveAppliesCanonicalizeToBothSides(t *testing.T) {
	// The repository path arrives through a symlinked directory.
	canon := func(p string) string { return strings.Replace(p, "/tmp/home/link", "/tmp/home/Projects", 1) }

	got := Resolve(demoConfig(), "/tmp/home/link/acme/billing", Options{Home: home, Canonicalize: canon})

	if got.Profile.ID != "acme" || got.Reason != ReasonRule {
		t.Fatalf("Resolve() = %+v, want rule acme after canonicalization", got)
	}
}

func TestResolveIgnoresInvalidEntries(t *testing.T) {
	cfg := config.Config{
		Profiles: []config.Profile{{ID: "personal", Dirs: []string{"relative/bad", "~/Projects/personal"}}},
		Pins:     []config.Pin{{Repo: "", Profile: "personal"}, {Repo: "~/x", Profile: "ghost"}},
	}

	got := Resolve(cfg, expand("~/x"), Options{Home: home})
	rule := Resolve(cfg, expand("~/Projects/personal/a"), Options{Home: home})

	if got.Found {
		t.Errorf("pin to a missing profile resolved to %+v, want not found", got)
	}
	if rule.Profile.ID != "personal" {
		t.Errorf("valid rule next to an invalid one = %+v, want personal", rule)
	}
}

func TestReasonDescribe(t *testing.T) {
	tests := map[Reason]string{
		ReasonPin:     "pinned repository",
		ReasonRule:    "directory rule",
		ReasonDefault: "default profile",
		ReasonNone:    "no rule matches",
	}
	for reason, want := range tests {
		if got := reason.Describe(); got != want {
			t.Errorf("%q.Describe() = %q, want %q", reason, got, want)
		}
	}
}

func expand(p string) string {
	return strings.Replace(p, "~", home, 1)
}

func TestResolveGitDirFollowsGitIncludeIfSemantics(t *testing.T) {
	tests := []struct {
		name        string
		gitDir      string
		wantProfile string
		wantReason  Reason
	}{
		{name: "normal repository", gitDir: "~/Projects/acme/billing/.git", wantProfile: "acme", wantReason: ReasonRule},
		{name: "worktree follows its main repository", gitDir: "~/Projects/personal/blog/.git/worktrees/feature", wantProfile: "personal", wantReason: ReasonRule},
		{name: "pinned repository", gitDir: "~/Downloads/globex-hotfix/.git", wantProfile: "globex", wantReason: ReasonPin},
		{name: "worktree of a pinned repository", gitDir: "~/Downloads/globex-hotfix/.git/worktrees/wt", wantProfile: "globex", wantReason: ReasonPin},
		{name: "repository nested in a pinned one is not pinned", gitDir: "~/Downloads/globex-hotfix/vendor/lib/.git", wantProfile: "acme", wantReason: ReasonDefault},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveGitDir(demoConfig(), expand(tc.gitDir), Options{Home: home})

			if got.Profile.ID != tc.wantProfile || got.Reason != tc.wantReason {
				t.Fatalf("ResolveGitDir(%s) = %q/%q, want %q/%q", tc.gitDir, got.Profile.ID, got.Reason, tc.wantProfile, tc.wantReason)
			}
		})
	}
}

func TestResolveOutsideRepositoryUsesOnlyTheDefault(t *testing.T) {
	cfg := demoConfig()

	got := ResolveOutsideRepository(cfg)
	cfg.Default = ""
	none := ResolveOutsideRepository(cfg)

	if got.Profile.ID != "acme" || got.Reason != ReasonDefault {
		t.Errorf("outside a repository = %+v, want the default profile", got)
	}
	if none.Found || none.Reason != ReasonNone {
		t.Errorf("outside a repository without default = %+v, want nothing", none)
	}
}
