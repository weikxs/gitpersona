package generate

import (
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/config"
)

func resetsDemoConfig() config.Config {
	return config.Config{Version: config.CurrentVersion, Default: "acme", Profiles: []config.Profile{
		{ID: "acme", Provider: config.ProviderGitHub, Host: "github.com", Account: "acme-alex", Name: "Alex (Acme)",
			Email: "acme-alex@demo.example", Dirs: []string{"~/Projects/acme"}},
		{ID: "personal", Provider: config.ProviderGitHub, Host: "github.com", Account: "octo-alex", Name: "Alex Octo",
			Email: "octo-alex@demo.example", Dirs: []string{"~/Projects/personal"}},
	}}
}

func generatedFile(t *testing.T, files []File, name string) string {
	t.Helper()
	for _, f := range files {
		if f.Name == name {
			return f.Content
		}
	}
	t.Fatalf("no generated file %q", name)
	return ""
}

func TestDefaultProfileResetsKeysUnderEveryKindOfUserRule(t *testing.T) {
	opts := demoOptions()
	opts.KeyRules = []KeyRule{
		{Condition: "onbranch:main", Signing: true, SSH: true, Identity: true},
		{Dir: "/tmp/home/corp", Signing: true, Identity: true},
		{Signing: true, Identity: true},
		{Dir: "/tmp/home/keys-only", SSH: true}, // sets no identity: the default does not override it
	}

	files, err := Generate(resetsDemoConfig(), opts)

	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	rules := generatedFile(t, files, RulesFile)
	signing := "path = /tmp/home/.config/gitpersona/" + ResetFile("acme", "signing")
	ssh := "path = /tmp/home/.config/gitpersona/" + ResetFile("acme", "ssh")
	for _, want := range []string{
		"[includeIf \"onbranch:main\"]\n\t" + signing + "\n\t" + ssh + "\n",
		"[includeIf \"gitdir/i:/tmp/home/corp/\"]\n\t" + signing + "\n",
		"[include]\n\t" + signing + "\n",
	} {
		if !strings.Contains(rules, want) {
			t.Errorf("rules lack %q:\n%s", want, rules)
		}
	}
	if strings.Contains(rules, "keys-only") {
		t.Errorf("a keys-only rule is not overridden by the default, so it needs no reset:\n%s", rules)
	}
	generatedFile(t, files, ResetFile("acme", "signing"))
	generatedFile(t, files, ResetFile("acme", "ssh"))
}

func TestAUserRuleOverlappingAProfilesFolderMakesItResetKeys(t *testing.T) {
	opts := demoOptions()
	opts.KeyRules = []KeyRule{
		{Dir: "/tmp/home/Projects/personal/client", Signing: true, SSH: true},
		{Signing: true}, // can apply anywhere
	}

	files, err := Generate(resetsDemoConfig(), opts)

	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(generatedFile(t, files, ResetFile("personal", "ssh")), "sshCommand") {
		t.Error("personal should reset the SSH command a rule inside its folder sets")
	}
	generatedFile(t, files, ResetFile("personal", "signing"))
}

func TestContainsIsFalseWhenEitherPathCannotBeNormalized(t *testing.T) {
	o := demoOptions()

	if o.contains("relative/dir", "~/Projects/acme", false) || o.contains("~/Projects", "relative/dir", false) {
		t.Fatal("contains() accepted a relative path, want false")
	}
}
