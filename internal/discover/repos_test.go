package discover

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestReposReportRemoteHostOwnerAndTheirOwnEmail(t *testing.T) {
	root := t.TempDir()
	repo(t, filepath.Join(root, "work", "api"), "git@github-work:demo-org/api.git")
	write(t, filepath.Join(root, "work", "api", ".git", "config"), `[core]
	bare = false
[remote "upstream"]
	url = https://github.com/upstream-org/api.git
[remote "origin"]
	url = git@github-work:demo-org/api.git
[user]
	email = Worker@Demo.example
	name = Demo Worker
`)
	repo(t, filepath.Join(root, "fun", "game"), "https://github.com/demo-personal/game")
	repo(t, filepath.Join(root, "local"), "")

	got, complete := Repos(context.Background(), []string{root}, ScanLimits{MaxDepth: 4, Timeout: time.Second})

	if !complete {
		t.Error("a small scan should complete")
	}
	if len(got) != 3 {
		t.Fatalf("Repos() = %+v, want three repositories", got)
	}
	byDir := map[string]Repo{}
	for _, r := range got {
		byDir[r.Dir] = r
	}
	if r := byDir[filepath.Join(root, "work", "api")]; r.Host != "github-work" || r.Owner != "demo-org" || r.Email != "Worker@Demo.example" {
		t.Errorf("work/api = %+v, want origin's host alias and owner, and the repository's own email", r)
	}
	if r := byDir[filepath.Join(root, "fun", "game")]; r.Host != "github.com" || r.Owner != "demo-personal" || r.Email != "" {
		t.Errorf("fun/game = %+v", r)
	}
	if r := byDir[filepath.Join(root, "local")]; r.Host != "" || r.Owner != "" {
		t.Errorf("a repository without a remote = %+v", r)
	}
}

func TestParseRemote(t *testing.T) {
	for url, want := range map[string][3]string{
		"https://github.com/demo-work/api.git":                 {"github.com", "demo-work", ""},
		"https://demo-personal@github.com/demo-work/api.git":   {"github.com", "demo-work", "demo-personal"},
		"https://demo-personal:secret@github.com/demo/api.git": {"github.com", "demo", "demo-personal"},
		"ssh://git@github.com:22/demo-work/api.git":            {"github.com", "demo-work", ""},
		"git@github-work:demo-org/api.git":                     {"github-work", "demo-org", ""},
		"github-work:demo-org/api.git":                         {"github-work", "demo-org", ""},
		"/srv/git/api.git":                                     {"", "", ""},
		"file:///srv/git/api.git":                              {"", "", ""},
	} {
		host, owner, user := parseRemote(url)
		if host != want[0] || owner != want[1] || user != want[2] {
			t.Errorf("parseRemote(%q) = %q, %q, %q, want %q", url, host, owner, user, want)
		}
	}
}

func TestReposInsideRepositoriesAndAScannedRepositoryAreLone(t *testing.T) {
	root := t.TempDir()
	repo(t, root, "https://github.com/demo-personal/src")
	repo(t, filepath.Join(root, "outer"), "https://github.com/demo-personal/outer")
	repo(t, filepath.Join(root, "outer", "inner"), "git@github-work:demo-work/inner.git")
	write(t, filepath.Join(root, "outer", "sub", ".git"), "gitdir: ../.git/modules/sub\n") // a submodule

	got, _ := Repos(context.Background(), []string{root}, ScanLimits{MaxDepth: 4, Timeout: time.Second})

	lone := map[string]bool{}
	for _, r := range got {
		rel, _ := filepath.Rel(root, r.Dir)
		lone[rel] = r.Lone
	}
	want := map[string]bool{".": true, "outer": true, filepath.Join("outer", "inner"): true}
	if len(lone) != len(want) {
		t.Fatalf("Repos() found %v, want %v", lone, want)
	}
	for dir, l := range want {
		if got, ok := lone[dir]; !ok || got != l {
			t.Errorf("%s: lone = %v (found %v), want %v", dir, got, ok, l)
		}
	}
}
