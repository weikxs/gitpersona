package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/discover"
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
		"https://git.demo.example:443":     true,  // the default port
		"git.demo.example":                 true,  // a bare host name
		"*.demo.example":                   false, // no globs without a scheme
		"git.demo.example:443":             false,
		"alice@git.demo.example":           true,
		"bob@git.demo.example":             false,
		"GIT.demo.example":                 false, // compared exactly without a scheme
		"git.demo.example/team":            false,
		"https://":                         true, // no host: every host
		"https://alice@":                   true,
		"alice@":                           true,
		"bob@":                             false,
		"https://git.demo.example.":        true,  // a trailing dot
		"https://[git.demo.example]":       false, // brackets are for IPv6
		"https://*":                        false, // one label only
		"https://alice@git.demo.example":   true,  // the account itself
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
	push, err2 := env.hostHelpers(context.Background(), "gitlab.globex.example", "")

	if err1 != nil || err2 != nil || !slices.Equal(alice, []string{"osxkeychain", "store"}) || !slices.Equal(push, []string{"osxkeychain"}) {
		t.Errorf("alice %v, push %v (%v, %v); a user's helper applies only to requests naming that user", alice, push, err1, err2)
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

// Git matches https://user@host settings with the user written in the
// remote's URL only; a push from https://host/... never reaches them.
func TestAHelperForOneUserIsNotWhatAPushUses(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	appendConfig(t, w, "[credential \"https://globex-alex@gitlab.globex.example\"]\n\thelper = /tmp/demo/git-credential-demo\n")
	env.Stdin = strings.NewReader("demo-token\n")

	err := AuthLogin(context.Background(), env, AuthOptions{Profile: "globex"})

	if err == nil || !strings.Contains(err.Error(), "no credential helper would keep the token") {
		t.Errorf("auth login with a helper only for the user = %v", err)
	}
	_ = Doctor(context.Background(), env)
	if !strings.Contains(out.String(), "profile globex: no credential helper") {
		t.Errorf("doctor:\n%s", out.String())
	}
}

func TestAuthLoginRefusesAStoreHelperForTheAccount(t *testing.T) {
	w := newWorld(t)
	fakeKeychain(t, w)
	env, _ := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	appendConfig(t, w, "[credential \"https://globex-alex@gitlab.globex.example\"]\n\thelper = store\n")
	env.Stdin = strings.NewReader("demo-token\n")

	err := AuthLogin(context.Background(), env, AuthOptions{Profile: "globex"})

	if err == nil || !strings.Contains(err.Error(), "clear text") {
		t.Errorf("auth login would also save with store: %v", err)
	}
}

func TestAuthRefusesAnInvalidConfiguration(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	cfgPath := filepath.Join(w.home, ".config", "gitpersona", "config.toml")
	mustWrite(t, cfgPath, strings.Replace(readFile(t, cfgPath), `host     = "gitlab.globex.example"`, "", 1), 0o600)
	env.Stdin = strings.NewReader("demo-token\n")

	err := AuthLogin(context.Background(), env, AuthOptions{Profile: "globex"})

	if err == nil || !strings.Contains(err.Error(), "profile globex needs a valid host") {
		t.Errorf("auth login with no host = %v", err)
	}
}

func TestAuthLogoutWorksDespiteMistakesInOtherProfiles(t *testing.T) {
	w := newWorld(t)
	fakeKeychain(t, w)
	env, _ := w.env(t, w.home)
	cfgPath := filepath.Join(w.home, ".config", "gitpersona", "config.toml")
	mustWrite(t, cfgPath, strings.Replace(readFile(t, cfgPath), `email   = "1000002+acme-alex@users.noreply.github.com"`, `email   = "not an email"`, 1), 0o600)

	if err := AuthLogout(context.Background(), env, AuthOptions{Profile: "globex"}); err != nil {
		t.Errorf("auth logout with another profile's mistake = %v", err)
	}
}

// A setting for account@host that resets the helpers sends the token where
// a push never looks.
func TestAuthLoginRefusesHelpersAPushDoesNotAsk(t *testing.T) {
	w := newWorld(t)
	fakeKeychain(t, w)
	env, _ := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	appendConfig(t, w, "[credential \"https://globex-alex@gitlab.globex.example\"]\n\thelper =\n\thelper = /tmp/demo/other-helper\n")
	env.Stdin = strings.NewReader("demo-token\n")

	err := AuthLogin(context.Background(), env, AuthOptions{Profile: "globex"})

	if err == nil || !strings.Contains(err.Error(), "which a push to gitlab.globex.example does not ask") {
		t.Errorf("auth login = %v", err)
	}
}

func TestURLMatchesNormalizesPortsAsGitDoes(t *testing.T) {
	if !urlMatches("https://git.demo.example:08443", "git.demo.example:8443", "") {
		t.Error("leading zeros in a port do not count")
	}
	if urlMatches("https://[git.demo.example]:8443", "git.demo.example:8443", "") {
		t.Error("a host name in brackets is not a host")
	}
}

func TestHelperKindKnowsTheShellForms(t *testing.T) {
	for helper, want := range map[string]string{
		"store": "plain", "!git credential-store --file x": "plain", "/usr/bin/git-credential-store": "plain",
		"!git credential-cache": "memory", "osxkeychain": "secure", "!/opt/demo/helper": "other",
		`"/opt/a b/git-credential-store"`: "plain", "!/usr/bin/env git-credential-store": "plain",
		"!git -c x=y credential-store": "plain", `!f() { git credential-store "$@"; }; f`: "plain",
		`C:/Program Files/Git/git.exe credential-store`: "plain", "manager-core": "secure",
	} {
		if got := helperKind(helper); got != want {
			t.Errorf("helperKind(%q) = %q, want %q", helper, got, want)
		}
	}
}

func TestAStoreHelperForEveryHostIsRefused(t *testing.T) {
	w := newWorld(t)
	fakeKeychain(t, w)
	env, _ := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	appendConfig(t, w, "[credential \"https://\"]\n\thelper = !git credential-store\n")
	env.Stdin = strings.NewReader("demo-token\n")

	err := AuthLogin(context.Background(), env, AuthOptions{Profile: "globex"})

	if err == nil || !strings.Contains(err.Error(), "clear text") {
		t.Errorf("auth login with store for every host = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(w.home, ".git-credentials")); statErr == nil {
		t.Error("the token went to a file in clear text")
	}
}

func TestATokenPerRepositoryIsRefused(t *testing.T) {
	w := newWorld(t)
	fakeKeychain(t, w)
	env, out := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	appendConfig(t, w, "[credential]\n\tuseHttpPath = true\n")
	env.Stdin = strings.NewReader("demo-token\n")

	err := AuthLogin(context.Background(), env, AuthOptions{Profile: "globex"})
	_ = Doctor(context.Background(), env)

	if err == nil || !strings.Contains(err.Error(), "credential.useHttpPath is on for gitlab.globex.example") {
		t.Errorf("auth login with useHttpPath = %v", err)
	}
	if !strings.Contains(out.String(), "turn it off where you set it") {
		t.Errorf("doctor should say how to turn it off:\n%s", out.String())
	}
}

func TestAuthStatusTellsInvalidProfilesFromMissingTokens(t *testing.T) {
	w := newWorld(t)
	fakeKeychain(t, w)
	env, _ := w.env(t, w.home)
	cfgPath := filepath.Join(w.home, ".config", "gitpersona", "config.toml")
	mustWrite(t, cfgPath, strings.Replace(readFile(t, cfgPath), `host     = "gitlab.globex.example"`, "", 1), 0o600)

	err := AuthStatus(context.Background(), env, AuthOptions{})

	if err == nil || err.Error() != "1 of 1 profiles need a valid host and account" {
		t.Errorf("auth status = %v", err)
	}
}

func TestURLMatchesKnowsOnlyStarAndDecodesUsers(t *testing.T) {
	if urlMatches("https://g?t.demo.example", "git.demo.example", "") || urlMatches("https://[gh]it.demo.example", "git.demo.example", "") {
		t.Error("Git knows only * in host names")
	}
	if !urlMatches("https://g*t.demo.example", "git.demo.example", "") || !urlMatches("https://al%69ce@git.demo.example", "git.demo.example", "alice") {
		t.Error("* within a label, and a percent-escaped user, match")
	}
}

func TestPerRepositoryTokensForSomeRepositoriesAreFound(t *testing.T) {
	w := newWorld(t)
	fakeKeychain(t, w)
	env, _ := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	appendConfig(t, w, "[credential \"https://gitlab.globex.example/team\"]\n\tuseHttpPath = 1k\n")
	env.Stdin = strings.NewReader("demo-token\n")

	err := AuthLogin(context.Background(), env, AuthOptions{Profile: "globex"})

	if err == nil || !strings.Contains(err.Error(), "on for some repositories of gitlab.globex.example") {
		t.Errorf("auth login with useHttpPath for a path = %v", err)
	}
}

// A repository inside one profile's folder that names another signed-in
// account for pushes gets that account's token, not none and not the
// profile's.
func TestARepositoryNamingAnotherAccountPushesAsIt(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	runGit(t, w.repos["billing"], "config", "credential.https://github.com.username", "octo-alex")

	answer := runGitInput(t, w.repos["billing"], "protocol=https\nhost=github.com\n\n", "credential", "fill")

	if !strings.Contains(answer, "username=octo-alex") || !strings.Contains(answer, "password=token-for-octo-alex") {
		t.Errorf("git credential fill in an acme repository naming octo-alex =\n%s", answer)
	}
}
