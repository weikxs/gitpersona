package app

import (
	"context"
	"github.com/weikxs/gitpersona/internal/discover"
	"os"
	"path/filepath"
	"slices"
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
	if err := AuthStatus(context.Background(), env, AuthOptions{}); err != nil || !strings.Contains(out.String(), "✓ globex: the credential store has a token for globex-alex") ||
		strings.Contains(out.String(), "demo-token") {
		t.Errorf("auth status: %v\n%s", err, out.String())
	}

	if err := AuthLogout(context.Background(), env, AuthOptions{Profile: "globex"}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	err := AuthStatus(context.Background(), env, AuthOptions{Profile: "globex"})
	if !strings.Contains(out.String(), "✗ globex: no token for globex-alex") || err == nil {
		t.Errorf("after logout: %v\n%s", err, out.String())
	}
	if err := AuthStatus(context.Background(), env, AuthOptions{Profile: "personal"}); err == nil || !strings.Contains(err.Error(), "gh auth status") {
		t.Errorf("auth status for a GitHub profile = %v", err)
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

func TestURLMatchesLikeGit(t *testing.T) {
	for pattern, want := range map[string]bool{
		"https://git.demo.example":         true,
		"https://git.demo.example/":        true,
		"https://*.demo.example":           true,
		"https://GIT.demo.example":         true,
		"HTTPS://git.demo.example":         true,
		"https://git.demo.example:443":     true, // the default port
		"git.demo.example":                 true, // a partial URL
		"https://alice@git.demo.example":   true, // the account itself
		"https://bob@git.demo.example":     false,
		"https://Alice@git.demo.example":   false, // user names are exact
		"https://git.demo.example:8443":    false,
		"https://*.example":                false, // * is one label
		"https://git.demo.example/team":    false, // some repositories only
		"http://git.demo.example":          false,
		"https://other.demo.example.extra": false,
	} {
		if got := urlMatches(pattern, "git.demo.example", "alice"); got != want {
			t.Errorf("urlMatches(%q) = %v, want %v", pattern, got, want)
		}
	}
	if !urlMatches("https://git.demo.example:8443", "git.demo.example:8443", "alice") || !urlMatches("https://[::1]:8443", "[::1]:8443", "alice") {
		t.Error("the same port matches, IPv6 included")
	}
}

func TestHelpersAreReadPerAccountInGitsOrder(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	appendConfig(t, w, "[credential]\n\thelper = osxkeychain\n"+
		"[credential \"https://alice@gitlab.globex.example\"]\n\thelper = store\n"+
		"[credential \"https://other.example\"]\n\thelper =\n"+
		"[credential \"https://gitlab.globex.example\"]\n\txhelper = ignored\n")

	alice, err1 := env.hostHelpers(context.Background(), "gitlab.globex.example", "alice")
	bob, err2 := env.hostHelpers(context.Background(), "gitlab.globex.example", "bob")

	if err1 != nil || err2 != nil || !slices.Equal(alice, []string{"osxkeychain", "store"}) || !slices.Equal(bob, []string{"osxkeychain"}) {
		t.Errorf("alice %v, bob %v (%v, %v); a user's helper and another host's reset apply to nobody else", alice, bob, err1, err2)
	}
}

func TestDoctorWithoutAnyHelperSaysSo(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	_ = Doctor(context.Background(), env)

	if !strings.Contains(out.String(), "no credential helper would keep the token for gitlab.globex.example") ||
		strings.Contains(out.String(), "(cache)") {
		t.Errorf("doctor without a helper:\n%s", out.String())
	}
}

func TestDoctorFindsHelpersForAWildcardURLAndWarnsAboutCache(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	appendConfig(t, w, "[credential \"https://*.globex.example\"]\n\thelper = /tmp/demo/git-credential-demo\n")

	_ = Doctor(context.Background(), env)

	if !strings.Contains(out.String(), "credentials for globex-alex on gitlab.globex.example come from /tmp/demo/git-credential-demo") {
		t.Errorf("doctor should find the helper set for *.globex.example:\n%s", out.String())
	}
	out.Reset()
	appendConfig(t, w, "[credential]\n\thelper =\n\thelper = cache\n")

	_ = Doctor(context.Background(), env)

	if !strings.Contains(out.String(), "(cache) forgets tokens after a while") {
		t.Errorf("doctor should warn about the cache helper:\n%s", out.String())
	}
}

func TestAuthStatusRefusesAnUnknownProfile(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)

	if err := AuthStatus(context.Background(), env, AuthOptions{Profile: "nope"}); err == nil || !strings.Contains(err.Error(), `unknown profile "nope"`) {
		t.Errorf("auth status --profile nope = %v", err)
	}
}

func TestGitConfigRegexpKeepsNewlinesAndSkipsNamesWithoutValue(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	appendConfig(t, w, "[credential]\n\thelper = \"one\\ntwo\"\n\thelper\n\thelper = three\n")

	got, err := env.gitConfigRegexp(context.Background(), `^credential\.helper$`)

	want := [][2]string{{"credential.helper", "one\ntwo"}, {"credential.helper", "three"}}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("gitConfigRegexp = %q, %v; want %q", got, err, want)
	}
}

func TestAllKeptComparesFoldersAsTheDiskDoes(t *testing.T) {
	p := SetupProposal{KeptRules: []KeptRule{{Dir: "~/Work"}}}
	u := LeftOut{Identity: discover.Identity{Dirs: []string{"~/work"}}}

	if !allKept(p, u, strings.ToLower) || allKept(p, u, func(s string) string { return s }) {
		t.Error("allKept should follow the comparison key")
	}
}
