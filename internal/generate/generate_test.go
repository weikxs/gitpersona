package generate

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/config"
)

var update = flag.Bool("update", false, "rewrite golden files in testdata/")

const (
	home      = "/tmp/home"
	configDir = "/tmp/home/.config/gitpersona"
	exe       = "/opt/tools/gitpersona"
)

func demoConfig() config.Config {
	return config.Config{
		Version: config.CurrentVersion,
		Default: "acme",
		Profiles: []config.Profile{
			{ID: "personal", Provider: config.ProviderGitHub, Host: "github.com", Account: "octo-alex", Name: "Alex Octo",
				Email:   "1000001+octo-alex@users.noreply.github.com",
				Signing: config.Signing{Method: config.SigningSSH, Key: "~/.ssh/id_ed25519_personal.pub"},
				Dirs:    []string{"~/Projects/personal", "~/Projects/acme/oss"}},
			{ID: "acme", Provider: config.ProviderGitHub, Host: "github.com", Account: "acme-alex", Name: "Alex (Acme)",
				Email: "1000002+acme-alex@users.noreply.github.com", Dirs: []string{"~/Projects/acme"}},
			{ID: "globex", Provider: config.ProviderGitLab, Host: "gitlab.globex.example", Account: "globex-alex", Name: "Alex",
				Email: "alex@globex.example", SSHKey: "~/.ssh/id_ed25519_globex", Dirs: []string{"~/Projects/clients/globex"}},
		},
		Pins: []config.Pin{{Repo: "~/Downloads/globex-hotfix", Profile: "globex"}},
	}
}

func demoOptions() Options {
	return Options{
		Home:            home,
		ConfigDir:       configDir,
		Executable:      exe,
		CaseInsensitive: true,
		FallbackHelpers: map[string][]string{
			"https://github.com": {"!/usr/local/bin/gh auth git-credential"},
		},
		FallbackSSHCommand: "",
	}
}

func TestGenerateMatchesGoldenFiles(t *testing.T) {
	files, err := Generate(demoConfig(), demoOptions())
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	wantNames := []string{"profiles/acme.gitconfig", "profiles/globex.gitconfig", "profiles/personal.gitconfig", "rules.gitconfig"}
	if len(files) != len(wantNames) {
		t.Fatalf("Generate() returned %d files, want %d", len(files), len(wantNames))
	}
	for i, f := range files {
		if f.Name != wantNames[i] {
			t.Fatalf("file %d = %s, want %s (files must be sorted)", i, f.Name, wantNames[i])
		}
		compareGolden(t, f.Name, f.Content)
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	first, _ := Generate(demoConfig(), demoOptions())
	second, _ := Generate(demoConfig(), demoOptions())

	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("Generate() is not deterministic for %s", first[i].Name)
		}
	}
}

func TestGenerateCaseSensitiveUsesPlainGitdir(t *testing.T) {
	opts := demoOptions()
	opts.CaseInsensitive = false

	files, _ := Generate(demoConfig(), opts)
	rules := files[len(files)-1].Content

	if strings.Contains(rules, "gitdir/i:") || !strings.Contains(rules, `[includeIf "gitdir:/tmp/home/Projects/acme/"]`) {
		t.Fatalf("case-sensitive rules should use gitdir: without /i:\n%s", rules)
	}
}

func TestGenerateWithoutSigningOrKeysLeavesThoseSettingsAlone(t *testing.T) {
	cfg := demoConfig()
	cfg.Profiles[0].Signing = config.Signing{}
	cfg.Profiles[2].SSHKey = ""

	files, err := Generate(cfg, demoOptions())
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range files {
		if strings.Contains(f.Content, "gpgsign") || strings.Contains(f.Content, "sshCommand") {
			t.Fatalf("%s mentions signing or sshCommand although no profile uses them:\n%s", f.Name, f.Content)
		}
	}
}

func TestGenerateRejectsInvalidConfig(t *testing.T) {
	cfg := demoConfig()
	cfg.Default = "missing"

	if _, err := Generate(cfg, demoOptions()); err == nil {
		t.Fatal("Generate() with an invalid config succeeded, want an error")
	}
}

func TestQuoteValue(t *testing.T) {
	tests := map[string]string{
		"plain value":     "plain value",
		"":                `""`,
		" leading":        `" leading"`,
		"has # hash":      `"has # hash"`,
		"has ; semicolon": `"has ; semicolon"`,
		`back\slash`:      `"back\\slash"`,
		`say "hi"`:        `"say \"hi\""`,
	}
	for in, want := range tests {
		if got := quoteValue(in); got != want {
			t.Errorf("quoteValue(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestShellQuote(t *testing.T) {
	if got, want := shellQuote("/opt/my tools/it's"), `'/opt/my tools/it'\''s'`; got != want {
		t.Fatalf("shellQuote() = %s, want %s", got, want)
	}
}

func compareGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", strings.ReplaceAll(name, "/", "_")+".golden")
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file (run go test ./internal/generate -update): %v", err)
	}
	if got != string(want) {
		t.Errorf("%s differs from %s\n--- got ---\n%s\n--- want ---\n%s", name, path, got, want)
	}
}

func TestIncludeBody(t *testing.T) {
	got := IncludeBody("/tmp/home/.config/gitpersona/rules.gitconfig")

	if !strings.HasSuffix(got, "[include]\n\tpath = /tmp/home/.config/gitpersona/rules.gitconfig\n") {
		t.Fatalf("IncludeBody() = %q", got)
	}
}

func TestGenerateAddsRealPathWhenRuleGoesThroughSymlink(t *testing.T) {
	opts := demoOptions()
	opts.Canonicalize = func(p string) string { return strings.Replace(p, "/tmp/", "/private/tmp/", 1) }

	files, err := Generate(demoConfig(), opts)
	if err != nil {
		t.Fatal(err)
	}
	rules := files[len(files)-1].Content

	for _, want := range []string{
		`[includeIf "gitdir/i:/tmp/home/Projects/acme/"]`,
		`[includeIf "gitdir/i:/private/tmp/home/Projects/acme/"]`,
		`[includeIf "gitdir/i:/private/tmp/home/Downloads/globex-hotfix/.git"]`,
	} {
		if !strings.Contains(rules, want) {
			t.Errorf("rules are missing %s\n%s", want, rules)
		}
	}
}
