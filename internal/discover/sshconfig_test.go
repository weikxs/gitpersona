package discover

import (
	"path/filepath"
	"testing"
)

func TestSSHAliasesForGitHub(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".ssh", "config"), `# demo
Host github-work
  HostName github.com
  User git
  IdentityFile ~/.ssh/id_work
  IdentitiesOnly yes

Host github-fun gh-fun
    hostname=GitHub.com
    IdentityFile "~/.ssh/id fun"

Host github.com
  IdentityFile ~/.ssh/id_default

Host *.example.com gitlab-work
  HostName gitlab.com
  IdentityFile ~/.ssh/id_gitlab

Host *
  IdentityFile ~/.ssh/id_%h

Include conf.d/*

Match host github.com exec "true"
  IdentityFile ~/.ssh/ignored
`)
	write(t, filepath.Join(home, ".ssh", "conf.d", "extra"), "Host github-extra\n  HostName github.com\n  IdentityFile ~/.ssh/id_extra\n")

	got := SSHAliases(filepath.Join(home, ".ssh", "config"), home)

	want := []SSHAlias{
		{Alias: "github-work", IdentityFile: "~/.ssh/id_work"},
		{Alias: "github-fun", IdentityFile: "~/.ssh/id fun"},
		{Alias: "gh-fun", IdentityFile: "~/.ssh/id fun"},
		{Alias: "github.com", IdentityFile: "~/.ssh/id_default"},
		{Alias: "github-extra", IdentityFile: "~/.ssh/id_extra"},
	}
	if len(got) != len(want) {
		t.Fatalf("SSHAliases() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("alias %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSSHAliasesWithoutAFileFindsNothing(t *testing.T) {
	if got := SSHAliases(filepath.Join(t.TempDir(), "missing"), t.TempDir()); len(got) != 0 {
		t.Fatalf("SSHAliases() = %+v", got)
	}
}

func TestSSHAliasesFollowIncludesLikeSSH(t *testing.T) {
	home := t.TempDir()
	ssh := filepath.Join(home, ".ssh")
	write(t, filepath.Join(ssh, "config"), `Host github-work
  Include common/*
  HostName github.com

Host github-split
  HostName github.com
Host github-split
  IdentityFile ~/.ssh/id_split

Host github-pct
  HostName github.com
  IdentityFile ~/.ssh/id_%r

Match host foo
  Include matched

Include config
Include ~/.ssh/*
`)
	write(t, filepath.Join(ssh, "common", "a"), "Host *\n  ServerAliveInterval 30\n")
	write(t, filepath.Join(ssh, "matched"), "Host github-matched\n  HostName github.com\n")
	// Key files must never be opened, even when a glob names them.
	write(t, filepath.Join(ssh, "id_evil"), "Host github-from-a-key\n  HostName github.com\n")
	write(t, filepath.Join(ssh, "id_evil.pub"), "Host github-from-a-pub\n  HostName github.com\n")

	got := SSHAliases(filepath.Join(ssh, "config"), home)

	want := []SSHAlias{
		{Alias: "github-work"},
		{Alias: "github-split", IdentityFile: "~/.ssh/id_split"},
		{Alias: "github-pct"},
	}
	if len(got) != len(want) {
		t.Fatalf("SSHAliases() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("alias %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
