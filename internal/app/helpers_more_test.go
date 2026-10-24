package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/config"
)

func TestHelperAdviceNamesNoHelperWhenNoneIsInstalled(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("macOS always has its keychain helper")
	}
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	env = failGit(env, "--exec-path")
	env.LookPath = func(string) (string, error) { return "", os.ErrNotExist }

	got := env.helperAdvice(context.Background())

	if !strings.Contains(got, "install a helper that keeps tokens in the system keychain") {
		t.Errorf("helperAdvice = %q", got)
	}
}

func TestHelperNameFindsTheHelperWhateverTheCommandLooksLike(t *testing.T) {
	for name, tc := range map[string]struct {
		words []string
		want  string
	}{
		"a git-credential program":            {[]string{"!/usr/bin/git-credential-demo"}, "demo"},
		"git with an option that has a value": {[]string{"!git", "-c", "x=y", "credential-demo"}, "demo"},
		"git with a flag":                     {[]string{"!git", "--no-pager", "credential-demo"}, "demo"},
		"git with something else than a name": {[]string{"git", "status"}, "git"},
		"a short name":                        {[]string{"store"}, "store"},
		"nothing":                             {nil, ""},
		"git with only a flag and no helper":  {[]string{"git", "--version"}, "git"},
		"a quoted program with an exe suffix": {[]string{`"C:/bin/git-credential-demo.exe"`}, "demo"},
		"git credential-<name> after options": {[]string{"git", "-C", "/tmp", "credential-demo"}, "demo"},
		"a script that is not a known helper": {[]string{"/opt/scripts/keep-token.sh"}, "keep-token.sh"},
		"a git word later in a shell command": {[]string{"echo", "x", ";", "git", "credential-demo"}, "demo"},
		"the git program given by its path":   {[]string{"/usr/bin/git", "credential-demo"}, "demo"},
		"git with a flag then a helper name":  {[]string{"git", "-p", "credential-demo"}, "demo"},
	} {
		if got := helperName(tc.words); got != tc.want {
			t.Errorf("%s: helperName(%q) = %q, want %q", name, tc.words, got, tc.want)
		}
	}
}

func TestHelperChecksFailClearlyWhenGitCannotReadItsSettings(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[credential\n\thelper = broken\n", 0o600)
	env, out := w.env(t, w.home)
	ctx := context.Background()
	globex := config.Profile{ID: "globex", Provider: "gitlab", Host: "gitlab.globex.example", Account: "globex-alex"}

	if _, err := env.perRepositoryTokens(ctx, globex.Host); err == nil {
		t.Error("perRepositoryTokens with an unreadable setting succeeded")
	}
	if _, _, err := env.helpersProblem(ctx, globex); err == nil {
		t.Error("helpersProblem with an unreadable setting succeeded")
	}
	if ok := Doctor(ctx, env); ok {
		t.Errorf("doctor passed although Git cannot read its settings:\n%s", out.String())
	}
}

func TestHelpersAreCheckedForTheAccountEvenWhenThePushHelpersAreFine(t *testing.T) {
	w := newWorld(t)
	fakeKeychain(t, w)
	env, _ := w.env(t, w.home)
	globex := config.Profile{ID: "globex", Provider: "gitlab", Host: "gitlab.globex.example", Account: "globex-alex"}
	// A broken per-account section makes only the account lookup fail.
	appendConfig(t, w, "[credential \"https://globex-alex@gitlab.globex.example\"\n\thelper = broken\n")

	_, _, err := env.helpersProblem(context.Background(), globex)

	if err == nil {
		t.Error("helpersProblem should fail when the account's settings cannot be read")
	}
}
