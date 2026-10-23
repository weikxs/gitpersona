package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// glabWorld is a demo home with a fake glab, a fake keychain holding the
// globex account's token, and the gh and glab entries applied.
func glabWorld(t *testing.T) (world, Env, *strings.Builder, *execCall) {
	t.Helper()
	w := newWorld(t)
	mustWrite(t, filepath.Join(filepath.Dir(w.home), "bin", "glab"), "#!/bin/sh\nexit 0\n", 0o700)
	store := fakeKeychain(t, w)
	mustWrite(t, filepath.Join(store, "gitlab.globex.example@globex-alex"), "demo-token-for-globex", 0o600)
	env, stderr, call := shimEnv(t, w, w.repos["hotfix"])
	return w, env, stderr, call
}

// envValue returns the value of key in environ, or "".
func envValue(environ []string, key string) string {
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			return v
		}
	}
	return ""
}

func TestGlabRunsAsTheDirectorysGitLabAccount(t *testing.T) {
	_, env, stderr, call := glabWorld(t)

	code := GlabShim(context.Background(), env, []string{"mr", "list"})

	if code != 0 || filepath.Base(call.path) != "glab" || strings.HasPrefix(call.path, env.shimDir()) {
		t.Fatalf("exec %q (code %d): %s", call.path, code, stderr.String())
	}
	if token := envValue(call.env, "GITLAB_TOKEN"); token != "demo-token-for-globex" {
		t.Errorf("GITLAB_TOKEN = %q, want the globex account's token", token)
	}
	if host := envValue(call.env, "GITLAB_HOST"); host != "gitlab.globex.example" {
		t.Errorf("GITLAB_HOST = %q", host)
	}
	if strings.Contains(stderr.String(), "demo-token") {
		t.Error("the token was printed")
	}
}

func TestGlabIsLeftAloneWhereGitPersonaShouldNotChoose(t *testing.T) {
	w, env, _, call := glabWorld(t)

	for name, prepare := range map[string]func(){
		"a GitHub directory":  func() { env.WorkDir = w.repos["billing"] },
		"the user's token":    func() { t.Setenv("GITLAB_TOKEN", "the-users-own") },
		"glab auth":           func() {},
		"another GitLab host": func() { t.Setenv("GITLAB_HOST", "gitlab.com") },
	} {
		t.Run(name, func(t *testing.T) {
			env.WorkDir = w.repos["hotfix"]
			prepare()
			args := []string{"mr", "list"}
			if name == "glab auth" {
				args = []string{"auth", "status"}
			}
			*call = execCall{}

			if code := GlabShim(context.Background(), env, args); code != 0 {
				t.Fatalf("code %d", code)
			}
			if token := envValue(call.env, "GITLAB_TOKEN"); token == "demo-token-for-globex" {
				t.Error("GitPersona chose the account where it should not")
			}
			os.Unsetenv("GITLAB_TOKEN")
			os.Unsetenv("GITLAB_HOST")
		})
	}
}

func TestGlabWithoutATokenRefusesInsteadOfUsingAnotherAccount(t *testing.T) {
	w, env, stderr, call := glabWorld(t)
	if err := os.Remove(filepath.Join(w.home, "keychain", "gitlab.globex.example@globex-alex")); err != nil {
		t.Fatal(err)
	}

	code := GlabShim(context.Background(), env, []string{"mr", "list"})

	if code == 0 || call.path != "" || !strings.Contains(stderr.String(), "gitpersona auth login --profile globex") {
		t.Errorf("code %d, exec %q:\n%s", code, call.path, stderr.String())
	}
}

func TestApplyCreatesTheGlabEntryForGitLabProfiles(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if target, err := os.Readlink(filepath.Join(env.shimDir(), "glab")); err != nil || target != binary {
		t.Fatalf("glab entry -> %q (%v)", target, err)
	}
	if err := Uninstall(context.Background(), env, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(env.shimDir(), "glab")); !os.IsNotExist(err) {
		t.Error("uninstall should remove the glab entry")
	}
}

func TestGlabTakesTheSameHostWrittenAsAURL(t *testing.T) {
	w, env, _, call := glabWorld(t)
	env.WorkDir = w.repos["hotfix"]
	t.Setenv("GITLAB_HOST", "https://GitLab.Globex.example/")

	if code := GlabShim(context.Background(), env, []string{"mr", "list"}); code != 0 {
		t.Fatalf("code %d", code)
	}

	if token := envValue(call.env, "GITLAB_TOKEN"); token != "demo-token-for-globex" {
		t.Errorf("GITLAB_HOST as a URL of the same host should still choose the account, got token %q", token)
	}
}

func TestBareHost(t *testing.T) {
	for in, want := range map[string]string{"gitlab.com": "gitlab.com", "https://gitlab.com/": "gitlab.com", "http://git.demo.example:8080/api/v4": "git.demo.example:8080", "HTTPS://GitLab.com": "gitlab.com", "https://u@gitlab.com/": "gitlab.com", "https://gitlab.com:443": "gitlab.com", "http://gitlab.com:443": "gitlab.com:443"} {
		if got := bareHost(in); got != want {
			t.Errorf("bareHost(%q) = %q, want %q", in, got, want)
		}
	}
}
