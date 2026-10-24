package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/config"
)

func TestLabelMatchesTreatsStarAsAnyRunOfCharacters(t *testing.T) {
	for _, tc := range []struct {
		pattern, label string
		want           bool
	}{
		{"demo", "demo", true},
		{"demo", "demo2", false},
		{"*", "anything", true},
		{"*", "", true},
		{"git*", "gitlab", true},
		{"git*", "hub", false},
		{"*lab", "gitlab", true},
		{"*lab", "labs", false},
		{"g*b", "gitlab", true},
		{"g*b", "gitlabs", false},
		{"a*b*c", "a-x-b-y-c", true},
		{"a*b*c", "a-x-c-y-b", false},
		{"a*b*c", "ac", false},
	} {
		if got := labelMatches(tc.pattern, tc.label); got != tc.want {
			t.Errorf("labelMatches(%q, %q) = %v, want %v", tc.pattern, tc.label, got, tc.want)
		}
	}
}

func TestHelperAdviceNamesAHelperThatIsInstalled(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	env.LookPath = func(name string) (string, error) {
		if name == "git-credential-manager" {
			return "/demo/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
	want := "manager"
	if runtime.GOOS == "darwin" {
		want = "osxkeychain" // Git on macOS always has it, and it comes first
	}

	if advice := env.helperAdvice(context.Background()); !strings.Contains(advice, "credential.helper "+want) {
		t.Errorf("helperAdvice() = %q, want it to name %s", advice, want)
	}
}

func TestReadTokenTakesTheFirstLineOfStandardInput(t *testing.T) {
	for name, tc := range map[string]struct {
		input   string
		want    string
		wantErr string
	}{
		"a line with a newline":    {input: "demo-token\n", want: "demo-token"},
		"a line without a newline": {input: "demo-token", want: "demo-token"},
		"surrounding blanks":       {input: "  demo-token \r\n", want: "demo-token"},
		"only the first line":      {input: "first\nsecond\n", want: "first"},
		"nothing at all":           {input: "", wantErr: "no token given"},
		"only blanks":              {input: "   \n", wantErr: "empty or not on one line"},
		"a NUL byte":               {input: "abc\x00def\n", wantErr: "empty or not on one line"},
	} {
		t.Run(name, func(t *testing.T) {
			env := Env{Stdin: strings.NewReader(tc.input), Stderr: &strings.Builder{}}

			got, err := env.readToken("Token: ")

			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("readToken() error = %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("readToken() = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestReadTokenNeverEchoesWhatItRead(t *testing.T) {
	var stderr strings.Builder
	env := Env{Stdin: strings.NewReader("demo-secret\n"), Stderr: &stderr}

	if _, err := env.readToken("Token: "); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(stderr.String(), "demo-secret") {
		t.Errorf("readToken wrote the token to stderr: %q", stderr.String())
	}
}

func TestWithPinAddsANewPinWithoutChangingTheOriginal(t *testing.T) {
	cfg := config.Config{Pins: []config.Pin{{Repo: "~/code/one", Profile: "personal"}}}
	opts := config.EditOptions{Home: "/tmp/home"}

	got := withPin(cfg, "~/code/two", "acme", opts)

	if len(got.Pins) != 2 || got.Pins[1] != (config.Pin{Repo: "~/code/two", Profile: "acme"}) {
		t.Fatalf("pins = %+v, want the new pin appended", got.Pins)
	}
	if len(cfg.Pins) != 1 {
		t.Errorf("the original configuration was changed: %+v", cfg.Pins)
	}
}

func TestWithPinMovesAnExistingPinToAnotherProfileWithoutChangingTheOriginal(t *testing.T) {
	cfg := config.Config{Pins: []config.Pin{
		{Repo: "~/code/one", Profile: "personal"},
		{Repo: "~/code/two", Profile: "personal"},
	}}
	opts := config.EditOptions{Home: "/tmp/home"}

	got := withPin(cfg, "/tmp/home/code/two", "acme", opts)

	if len(got.Pins) != 2 || got.Pins[0].Profile != "personal" || got.Pins[1].Profile != "acme" {
		t.Fatalf("pins = %+v, want only the second pin moved to acme", got.Pins)
	}
	if cfg.Pins[1].Profile != "personal" {
		t.Errorf("the original configuration was changed: %+v", cfg.Pins)
	}
}

func TestAuthProfileResolvesTheProfileByNameOrByDirectory(t *testing.T) {
	w := newWorld(t)

	byName, _ := w.env(t, w.home)
	gotByName, errByName := byName.authProfile(AuthOptions{Profile: "globex"})
	byDir, _ := w.env(t, w.repos["hotfix"])
	gotByDir, errByDir := byDir.authProfile(AuthOptions{})

	if errByName != nil || gotByName.ID != "globex" {
		t.Errorf("by name = %q, %v; want globex", gotByName.ID, errByName)
	}
	if errByDir != nil || gotByDir.ID != "globex" {
		t.Errorf("by directory (a pinned repository) = %q, %v; want globex", gotByDir.ID, errByDir)
	}
}

func TestAuthProfileRefusesWhatItCannotUse(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"), `version = 1

[[profile]]
id      = "globex"
provider = "gitlab"
host    = "gitlab.globex.example"
account = "globex-alex"
name    = "Alex"
email   = "alex@globex.example"
dirs    = ["~/work/globex"]

[[profile]]
id      = "personal"
account = "octo-alex"
name    = "Alex Octo"
email   = "alex@octo.example"
dirs    = ["~/work/personal"]
`, 0o600)
	elsewhere := filepath.Join(w.home, "elsewhere")
	if err := os.MkdirAll(elsewhere, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		workDir string
		opts    AuthOptions
		want    string
	}{
		"an unknown profile":           {w.home, AuthOptions{Profile: "nope"}, `no profile "nope"; the profiles are globex, personal`},
		"no profile for the directory": {elsewhere, AuthOptions{}, "no profile applies here"},
		"a GitHub profile":             {w.home, AuthOptions{Profile: "personal"}, "is on GitHub"},
	} {
		t.Run(name, func(t *testing.T) {
			env, _ := w.env(t, tc.workDir)

			_, err := env.authProfile(tc.opts)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("authProfile() error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestAuthProfileFailsWithoutAConfiguration(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	env, _ := w.env(t, w.home)

	if _, err := env.authProfile(AuthOptions{Profile: "globex"}); err == nil {
		t.Fatal("authProfile() succeeded without a configuration, want an error")
	}
}
