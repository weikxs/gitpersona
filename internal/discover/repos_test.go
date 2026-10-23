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

	got := Repos(context.Background(), []string{root}, ScanLimits{MaxDepth: 4, Timeout: time.Second})

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
	for url, want := range map[string][2]string{
		"https://github.com/demo-work/api.git":       {"github.com", "demo-work"},
		"https://token@github.com/demo-work/api.git": {"github.com", "demo-work"},
		"ssh://git@github.com:22/demo-work/api.git":  {"github.com", "demo-work"},
		"git@github-work:demo-org/api.git":           {"github-work", "demo-org"},
		"github-work:demo-org/api.git":               {"github-work", "demo-org"},
		"/srv/git/api.git":                           {"", ""},
		"file:///srv/git/api.git":                    {"", ""},
	} {
		host, owner := parseRemote(url)
		if host != want[0] || owner != want[1] {
			t.Errorf("parseRemote(%q) = %q, %q, want %q, %q", url, host, owner, want[0], want[1])
		}
	}
}
