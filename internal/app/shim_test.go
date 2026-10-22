package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// execCall records what the gh shim would exec.
type execCall struct {
	path string
	argv []string
	env  []string
}

func shimEnv(t *testing.T, w world, dir string) (Env, *strings.Builder, *execCall) {
	t.Helper()
	env, _ := w.env(t, dir)
	var stderr strings.Builder
	env.Stderr = &stderr
	call := &execCall{}
	env.Exec = func(path string, argv, environ []string) error {
		call.path, call.argv, call.env = path, argv, environ
		return nil
	}
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	// The shim directory comes first on PATH, as the shell integration arranges.
	t.Setenv("PATH", env.shimDir()+string(os.PathListSeparator)+os.Getenv("PATH"))
	return env, &stderr, call
}

func envValue(environ []string, key string) (string, bool) {
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			return v, true
		}
	}
	return "", false
}

func TestApplyCreatesTheGHEntryAndUninstallRemovesIt(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	target, err := os.Readlink(filepath.Join(env.shimDir(), "gh"))
	if err != nil || target != binary {
		t.Fatalf("gh entry -> %q (%v), want a link to %s", target, err, binary)
	}
	if err := Uninstall(context.Background(), env, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(env.shimDir(), "gh")); !os.IsNotExist(err) {
		t.Fatal("uninstall should remove the gh entry")
	}
}

func TestGHShimRunsRealGHWithTheDirectorysAccount(t *testing.T) {
	w := newWorld(t)
	env, stderr, call := shimEnv(t, w, w.repos["sdk"])

	code := GHShim(context.Background(), env, []string{"pr", "create"})

	if code != 0 || filepath.Base(call.path) != "gh" || strings.HasPrefix(call.path, env.shimDir()) {
		t.Fatalf("exec %q (code %d), want the real gh outside %s", call.path, code, env.shimDir())
	}
	if strings.Join(call.argv, " ") != "gh pr create" {
		t.Fatalf("argv = %v", call.argv)
	}
	if token, ok := envValue(call.env, "GH_TOKEN"); !ok || token != "token-for-octo-alex" {
		t.Fatalf("GH_TOKEN for gh in ~/Projects/acme/oss = %q, want the personal account's token", token)
	}
	if strings.Contains(stderr.String(), "token-for-") {
		t.Fatalf("the shim printed a token:\n%s", stderr.String())
	}
}

func TestGHShimLeavesGHAloneWhenItShould(t *testing.T) {
	w := newWorld(t)
	cases := map[string]func(t *testing.T) (Env, *execCall){
		"user already set GH_TOKEN": func(t *testing.T) (Env, *execCall) {
			env, _, call := shimEnv(t, w, w.repos["billing"])
			t.Setenv("GH_TOKEN", "users-own-token")
			return env, call
		},
		"another host via GH_HOST": func(t *testing.T) (Env, *execCall) {
			env, _, call := shimEnv(t, w, w.repos["billing"])
			t.Setenv("GH_HOST", "github.example.com")
			return env, call
		},
		"GitLab profile": func(t *testing.T) (Env, *execCall) {
			env, _, call := shimEnv(t, w, w.repos["hotfix"])
			return env, call
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			env, call := setup(t)

			GHShim(context.Background(), env, []string{"repo", "view"})

			token, _ := envValue(call.env, "GH_TOKEN")
			if strings.HasPrefix(token, "token-for-") {
				t.Fatalf("GH_TOKEN = %q, want the shim to leave it alone", token)
			}
		})
	}
}

func TestGHShimRefusesWhenTheProfileAccountIsNotSignedIn(t *testing.T) {
	w := newWorld(t)
	env, stderr, call := shimEnv(t, w, w.repos["billing"])
	t.Setenv("FAKE_GH_ACCOUNTS", "octo-alex")

	code := GHShim(context.Background(), env, []string{"pr", "create"})

	if code == 0 || call.path != "" {
		t.Fatalf("gh ran (code %d, exec %q) although acme-alex is not signed in; it must not act as another account", code, call.path)
	}
	if !strings.Contains(stderr.String(), "acme-alex is not signed in to gh") || !strings.Contains(stderr.String(), "gh auth login") {
		t.Fatalf("the refusal should explain how to sign in:\n%s", stderr.String())
	}
}

func TestGHShimPassesAuthCommandsThroughUntouched(t *testing.T) {
	w := newWorld(t)
	env, _, call := shimEnv(t, w, w.repos["billing"])
	t.Setenv("FAKE_GH_ACCOUNTS", "octo-alex") // even when the account is not signed in yet
	t.Setenv("GH_LOG", filepath.Join(t.TempDir(), "gh.log"))

	code := GHShim(context.Background(), env, []string{"auth", "login"})

	if code != 0 || strings.Join(call.argv, " ") != "gh auth login" {
		t.Fatalf("gh auth login should run as is (code %d, argv %v)", code, call.argv)
	}
	if _, ok := envValue(call.env, "GH_TOKEN"); ok {
		t.Fatal("gh auth commands must not get a token")
	}
	if log, _ := os.ReadFile(os.Getenv("GH_LOG")); strings.Contains(string(log), "auth token") {
		t.Fatalf("no token may be fetched for gh auth commands:\n%s", log)
	}
}

func TestGHShimOutsideARepositoryUsesTheDirectoryRule(t *testing.T) {
	w := newWorld(t)
	personalDir := filepath.Join(w.home, "Projects", "personal") // not a repository; the default profile is acme
	env, _, call := shimEnv(t, w, personalDir)

	GHShim(context.Background(), env, []string{"repo", "clone", "octo-alex/tool"})

	if token, _ := envValue(call.env, "GH_TOKEN"); token != "token-for-octo-alex" {
		t.Fatalf("gh repo clone in ~/Projects/personal got %q, want the personal account (the clone will be a personal repository)", token)
	}
}

func TestGHShimFailsClearlyWithoutRealGH(t *testing.T) {
	w := newWorld(t)
	env, stderr, _ := shimEnv(t, w, w.repos["billing"])
	t.Setenv("PATH", env.shimDir())

	if code := GHShim(context.Background(), env, []string{"pr", "list"}); code == 0 || !strings.Contains(stderr.String(), "could not find the real gh") {
		t.Fatalf("code %d, stderr %q", code, stderr.String())
	}
}

func TestGHShimReportsExecFailure(t *testing.T) {
	w := newWorld(t)
	env, stderr, _ := shimEnv(t, w, w.repos["billing"])
	env.Exec = func(string, []string, []string) error { return errors.New("boom") }

	if code := GHShim(context.Background(), env, nil); code == 0 || !strings.Contains(stderr.String(), "boom") {
		t.Fatalf("code %d, stderr %q", code, stderr.String())
	}
}

// TestRealBinaryAsGHFollowsTheDirectory runs the built binary through the gh
// entry, as a shell with the integration would.
func TestRealBinaryAsGHFollowsTheDirectory(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", env.shimDir()+string(os.PathListSeparator)+os.Getenv("PATH"))

	for repo, want := range map[string]string{"sdk": "gh ran as octo-alex", "billing": "gh ran as acme-alex"} {
		cmd := exec.Command("gh", "pr", "list")
		cmd.Dir = w.repos[repo]
		got, err := cmd.CombinedOutput()

		if err != nil || strings.TrimSpace(string(got)) != want {
			t.Errorf("gh in %s = %q, %v; want %q", repo, got, err, want)
		}
		if strings.Contains(string(got), "token-for-") {
			t.Fatalf("a token reached the output:\n%s", got)
		}
	}
}
