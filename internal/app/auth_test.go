package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeKeychain configures, in the test home, a credential helper that keeps
// one token per host and account in a folder, as a system keychain does.
func fakeKeychain(t *testing.T, w world) string {
	t.Helper()
	store := filepath.Join(w.home, "keychain")
	helper := filepath.Join(w.home, "bin", "git-credential-demo")
	mustWrite(t, helper, `#!/bin/sh
store="`+store+`"
mkdir -p "$store"
host=; user=; pass=
while IFS='=' read -r k v; do
	[ -z "$k" ] && break
	case "$k" in host) host=$v;; username) user=$v;; password) pass=$v;; esac
done
f="$store/$host@$user"
case "$1" in
	get) [ -f "$f" ] && printf 'password=%s\n' "$(cat "$f")";;
	store) printf '%s' "$pass" > "$f";;
	erase) rm -f "$f";;
esac
`, 0o700)
	appendConfig(t, w, "[credential]\n\thelper = "+helper+"\n")
	return store
}

func TestAuthKeepsOneTokenPerAccountInTheUsersOwnStore(t *testing.T) {
	w := newWorld(t)
	store := fakeKeychain(t, w)
	env, out := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	env.Stdin = strings.NewReader("demo-token-for-globex\n")

	if err := AuthLogin(context.Background(), env, AuthOptions{Profile: "globex"}); err != nil {
		t.Fatalf("auth login: %v\n%s", err, out.String())
	}

	if data, err := os.ReadFile(filepath.Join(store, "gitlab.globex.example@globex-alex")); err != nil || string(data) != "demo-token-for-globex" {
		t.Fatalf("the user's store should hold the token under the account: %q, %v", data, err)
	}
	// Git, in a repository of the profile, gets that account's token.
	answer := runGitInput(t, w.repos["hotfix"], "protocol=https\nhost=gitlab.globex.example\n\n", "credential", "fill")
	if !strings.Contains(answer, "username=globex-alex") || !strings.Contains(answer, "password=demo-token-for-globex") {
		t.Errorf("git credential fill in the globex repository =\n%s", answer)
	}
	for _, f := range []string{".config/gitpersona/config.toml", ".config/gitpersona/profiles/globex.gitconfig", ".gitconfig"} {
		if strings.Contains(readFile(t, filepath.Join(w.home, f)), "demo-token-for-globex") {
			t.Errorf("%s holds the token; GitPersona must keep no copy", f)
		}
	}
	out.Reset()
	if err := AuthStatus(context.Background(), env); err != nil || !strings.Contains(out.String(), "✓ globex: the credential store has a token for globex-alex") ||
		strings.Contains(out.String(), "demo-token") {
		t.Errorf("auth status: %v\n%s", err, out.String())
	}

	if err := AuthLogout(context.Background(), env, AuthOptions{Profile: "globex"}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	_ = AuthStatus(context.Background(), env)
	if !strings.Contains(out.String(), "✗ globex: no token for globex-alex") {
		t.Errorf("after logout:\n%s", out.String())
	}
}

func TestAuthRefusesGitHubProfilesAndUnsafeHelpers(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	env.Stdin = strings.NewReader("demo-token\n")

	if err := AuthLogin(context.Background(), env, AuthOptions{Profile: "acme"}); err == nil || !strings.Contains(err.Error(), "gh auth login") {
		t.Errorf("a GitHub profile: %v", err)
	}
	if err := AuthLogin(context.Background(), env, AuthOptions{Profile: "globex"}); err == nil || !strings.Contains(err.Error(), "no credential helper") {
		t.Errorf("without a helper: %v", err)
	}
	appendConfig(t, w, "[credential]\n\thelper = store\n")
	if err := AuthLogin(context.Background(), env, AuthOptions{Profile: "globex"}); err == nil || !strings.Contains(err.Error(), "clear text") {
		t.Errorf("with the store helper: %v", err)
	}
	if _, err := os.Stat(filepath.Join(w.home, ".git-credentials")); err == nil {
		t.Error("a token was written in clear text")
	}
}

func TestHelperKinds(t *testing.T) {
	for helper, want := range map[string]string{
		"osxkeychain": "secure", "manager": "secure", "/usr/lib/git-core/git-credential-libsecret": "secure",
		"store --file ~/x": "plain", "cache --timeout 60": "memory", "!glab auth git-credential": "other",
	} {
		if got := helperKind(helper); got != want {
			t.Errorf("helperKind(%q) = %q, want %q", helper, got, want)
		}
	}
}

// appendConfig adds text to the test home's global Git configuration.
func appendConfig(t *testing.T, w world, text string) {
	t.Helper()
	gitconfig := filepath.Join(w.home, ".gitconfig")
	current, _ := os.ReadFile(gitconfig)
	mustWrite(t, gitconfig, string(current)+text, 0o600)
}
