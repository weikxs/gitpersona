package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestCheckSSHComparesTheGreetedAccountWithTheProfile(t *testing.T) {
	w := newWorld(t)
	// A fake ssh that greets as the host would, by the key it is given.
	mustWrite(t, filepath.Join(filepath.Dir(w.home), "bin", "ssh"), `#!/bin/sh
for a; do case "$a" in *id_globex) who="gitlab:globex-alex";; *id_acme) who="github:someone-else";; esac; done
case "$who" in
	gitlab:*) echo "Welcome to GitLab, @${who#gitlab:}!";;
	github:*) echo "Hi ${who#github:}! You've successfully authenticated, but GitHub does not provide shell access." >&2; exit 1;;
esac
`, 0o700)
	cfg := strings.Replace(demoConfig, `email    = "alex@globex.example"`, `email    = "alex@globex.example"`+"\n"+`ssh_key  = "~/.ssh/id_globex"`, 1)
	cfg = strings.Replace(cfg, `email   = "1000002+acme-alex@users.noreply.github.com"`,
		`email   = "1000002+acme-alex@users.noreply.github.com"`+"\n"+`ssh_key = "~/.ssh/id_acme"`, 1)
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"), cfg, 0o600)
	for _, k := range []string{"id_globex", "id_acme"} {
		// A FIFO: opening it would hang, so the test proves GitPersona never reads the key.
		path := filepath.Join(w.home, ".ssh", k)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := mkfifo(path); err != nil {
			t.Fatal(err)
		}
	}
	env, out := w.env(t, w.home)

	err := CheckSSH(context.Background(), env, CheckSSHOptions{})

	for _, want := range []string{
		"✓ globex: the key signs in to gitlab.globex.example as globex-alex",
		"✗ acme: the key signs in to github.com as someone-else, not acme-alex",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("check-ssh should say %q:\n%s", want, out.String())
		}
	}
	if err == nil || !strings.Contains(err.Error(), "1 of 2 SSH keys") {
		t.Errorf("check-ssh error = %v", err)
	}
}

func mkfifo(path string) error { return syscall.Mkfifo(path, 0o600) }

func TestCheckSSHReadsEachHostsAnswer(t *testing.T) {
	for name, tc := range map[string]struct {
		answer, want string
		ok           bool
	}{
		"bitbucket": {"authenticated via ssh key.\n\nYou can use git to connect to Bitbucket. Shell access is disabled\n" +
			"logged in as globex-alex.\n", "✓ globex: the key signs in to gitlab.globex.example as globex-alex", true},
		"deploy key": {"Hi acme-corp/billing! You've successfully authenticated, but GitHub does not provide shell access.\n",
			"the key is a deploy key for acme-corp/billing", false},
		"another key": {"debug1: Server accepts key: /tmp/other/id_default ED25519 SHA256:demo explicit\n" +
			"Welcome to GitLab, @globex-alex!\n", "ssh signed in with /tmp/other/id_default from your SSH settings instead", false},
		"refused": {"debug1: Next authentication method: publickey\ngit@gitlab.globex.example: Permission denied (publickey).\n",
			"did not sign in to gitlab.globex.example: git@gitlab.globex.example: Permission denied (publickey).", false},
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			answer := filepath.Join(filepath.Dir(w.home), "answer")
			mustWrite(t, answer, tc.answer, 0o600)
			mustWrite(t, filepath.Join(filepath.Dir(w.home), "bin", "ssh"), "#!/bin/sh\ncat '"+answer+"' >&2\nexit 1\n", 0o700)
			cfg := strings.Replace(demoConfig, `email    = "alex@globex.example"`, `email    = "alex@globex.example"`+"\n"+`ssh_key  = "~/.ssh/id_globex"`, 1)
			mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"), cfg, 0o600)
			mustWrite(t, filepath.Join(w.home, ".ssh", "id_globex"), "KEY PLACEHOLDER (never read)\n", 0o600)
			env, out := w.env(t, w.home)

			err := CheckSSH(context.Background(), env, CheckSSHOptions{Profile: "globex"})

			if !strings.Contains(out.String(), tc.want) {
				t.Errorf("check-ssh should say %q:\n%s", tc.want, out.String())
			}
			if (err == nil) != tc.ok {
				t.Errorf("check-ssh error = %v, want ok=%v", err, tc.ok)
			}
		})
	}
}

func TestCheckSSHRefusesAMissingKeyWithoutRunningSSH(t *testing.T) {
	w := newWorld(t)
	ran := filepath.Join(filepath.Dir(w.home), "ssh-ran")
	mustWrite(t, filepath.Join(filepath.Dir(w.home), "bin", "ssh"), "#!/bin/sh\ntouch '"+ran+"'\n", 0o700)
	cfg := strings.Replace(demoConfig, `email    = "alex@globex.example"`, `email    = "alex@globex.example"`+"\n"+`ssh_key  = "~/.ssh/id_missing"`, 1)
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"), cfg, 0o600)
	env, out := w.env(t, w.home)

	err := CheckSSH(context.Background(), env, CheckSSHOptions{Profile: "globex"})

	if err == nil || !strings.Contains(out.String(), "~/.ssh/id_missing does not exist") {
		t.Errorf("check-ssh with a missing key = %v:\n%s", err, out.String())
	}
	if _, err := os.Stat(ran); err == nil {
		t.Error("ssh ran although the key does not exist; it would sign in with another key")
	}
}
