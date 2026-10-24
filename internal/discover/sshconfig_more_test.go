package discover

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestSSHAliasesReadAFileOnlyOnceAndNeverOpenKeys(t *testing.T) {
	home := t.TempDir()
	ssh := filepath.Join(home, ".ssh")
	write(t, filepath.Join(ssh, "config"), `Include config
Include id_evil
Compression
Include ~/.ssh/extra
Include ~/.ssh/*
Host github-work
  HostName github.com
`)
	write(t, filepath.Join(ssh, "extra"), "Host github-extra\n  HostName github.com\n")
	write(t, filepath.Join(ssh, "id_evil"), "Host github-from-a-key\n  HostName github.com\n")
	// Matched by the wildcard but not named like a configuration file.
	write(t, filepath.Join(ssh, "notes"), "Host github-from-notes\n  HostName github.com\n")

	got := SSHAliases(filepath.Join(ssh, "config"), home)

	if len(got) != 2 || got[0].Alias != "github-extra" || got[1].Alias != "github-work" {
		t.Fatalf("SSHAliases() = %+v, want github-extra and github-work", got)
	}
}

func TestSSHAliasesStopFollowingIncludesAtTheDepthLimit(t *testing.T) {
	home := t.TempDir()
	ssh := filepath.Join(home, ".ssh")
	const chain = maxIncludeDepth + 3
	for i := 0; i < chain; i++ {
		body := fmt.Sprintf("Host github-%d\n  HostName github.com\nInclude link%d\n", i, i+1)
		name := "config"
		if i > 0 {
			name = fmt.Sprintf("link%d", i)
		}
		write(t, filepath.Join(ssh, name), body)
	}

	got := SSHAliases(filepath.Join(ssh, "config"), home)

	if len(got) != maxIncludeDepth+1 {
		t.Fatalf("SSHAliases() found %d aliases, want %d (the depth limit)", len(got), maxIncludeDepth+1)
	}
}
