package app

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestParseRepoConfigReadsScopeOriginKeyAndValue(t *testing.T) {
	out := "global\x00file:/tmp/home/.gitconfig\x00user.email\na@x\x00" +
		"local\x00file:.git/config\x00user.email\nb@y\x00" +
		"local\x00file:.git/config\x00core.bare2\x00" +
		"local\x00file:.git/config\x00url.git@github.com:.insteadOf\nhttps://github.com/\x00"

	cfg := parseRepoConfig(out)

	if len(cfg) != 4 {
		t.Fatalf("parsed %d entries, want 4: %+v", len(cfg), cfg)
	}
	if got := cfg.get("User.Email"); got != "b@y" {
		t.Fatalf("get(user.email) = %q, want the last value b@y", got)
	}
	if got := cfg.getIn("global", "user.email"); got != "a@x" {
		t.Fatalf("getIn(global, user.email) = %q, want a@x", got)
	}
	if got := cfg.getIn("local", "core.sshCommand"); got != "" {
		t.Fatalf("getIn(local, core.sshCommand) = %q, want empty", got)
	}
	if got := cfg.originFile("user.email", "/repo"); got != filepath.Join("/repo", ".git/config") {
		t.Fatalf("originFile = %q, want the repository's config made absolute", got)
	}
	if e, ok := cfg.last("core.bare2", ""); !ok || e.value != "" {
		t.Fatalf("a setting without a value = %+v, %v", e, ok)
	}
}

func TestNormalKeyKeepsTheSubsectionCase(t *testing.T) {
	cases := map[string]string{
		"User.Email":                    "user.email",
		"core.sshCommand":               "core.sshcommand",
		"URL.git@GitHub.com:.InsteadOf": "url.git@GitHub.com:.insteadof",
		"plain":                         "plain",
	}
	for in, want := range cases {
		if got := normalKey(in); got != want {
			t.Errorf("normalKey(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestRepoConfigAgreesWithGitConfig checks the dump against the git config
// commands it replaces, in a real repository with an include.
func TestRepoConfigAgreesWithGitConfig(t *testing.T) {
	w := newWorld(t)
	repo := w.repos["blog"]
	included := filepath.Join(w.home, "extra.gitconfig")
	mustWrite(t, included, "[user]\n\temail = included@x\n\tname = Included\n", 0o600)
	runGit(t, w.home, "config", "--global", "user.email", "global@x")
	runGit(t, repo, "config", "include.path", included)
	runGit(t, repo, "config", "core.sshCommand", "ssh -i ~/.ssh/k")
	env, _ := w.env(t, repo)

	cfg, err := env.readRepoConfig(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{"user.email", "user.name", "core.sshCommand"} {
		if got, want := cfg.get(key), runGit(t, repo, "config", key); got != want {
			t.Errorf("get(%s) = %q, git config says %q", key, got, want)
		}
		if got, want := cfg.getLocal(key, repo, filepath.Join(repo, ".git")), runGit(t, repo, "config", "--local", key); got != want {
			t.Errorf("getLocal(%s) = %q, git config --local says %q", key, got, want)
		}
	}
	origin, _, _ := strings.Cut(runGit(t, repo, "config", "--show-origin", "--get", "user.email"), "\t")
	if got := cfg.originFile("user.email", repo); "file:"+got != origin && !strings.HasSuffix(origin, strings.TrimPrefix(got, repo+"/")) {
		t.Errorf("originFile(user.email) = %q, git says %q", got, origin)
	}
}

// countGit wraps env so it counts the git processes started per directory.
func countGit(env Env) (Env, func(dir string) int) {
	var mu sync.Mutex
	counts := map[string]int{}
	base := env.Run
	env.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "git" && len(args) > 1 && args[0] == "-C" {
			mu.Lock()
			counts[args[1]]++
			mu.Unlock()
		}
		return base(ctx, name, args...)
	}
	return env, func(dir string) int {
		mu.Lock()
		defer mu.Unlock()
		return counts[dir]
	}
}

func TestVerifyStartsFewGitProcessesPerRepository(t *testing.T) {
	w := newWorld(t)
	repo := w.repos["blog"]
	runGit(t, repo, "remote", "add", "origin", "https://github.com/octo-alex/blog.git")
	runGit(t, repo, "remote", "add", "mirror", "git@github.com:octo-alex/blog.git")
	env, _ := w.env(t, repo)
	env, count := countGit(env)

	report, err := env.VerifyRepos(context.Background(), []string{repo}, false)
	if err != nil {
		t.Fatal(err)
	}

	var remotes int
	for _, r := range report.Repositories {
		if r.Path == env.display(repo) {
			remotes = len(r.Remotes)
		}
	}
	if remotes != 2 {
		t.Fatalf("verify found %d remotes in the blog, want 2: %+v", remotes, report.Repositories)
	}
	// rev-parse, the config dump, remote -v and one credential lookup for
	// the HTTPS remote; it was 13 before the dump.
	if got := count(repo); got > 5 {
		t.Fatalf("verify started %d git processes for one repository with two remotes, want at most 5", got)
	}
}
