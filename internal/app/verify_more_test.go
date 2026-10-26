package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// failingWriter is an output that refuses every write.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

// failGit makes every git command whose arguments contain all of the words fail.
func failGit(env Env, words ...string) Env {
	base := env.Run
	env.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "git" {
			joined := " " + strings.Join(args, " ") + " "
			failing := true
			for _, w := range words {
				failing = failing && strings.Contains(joined, " "+w+" ")
			}
			if failing {
				return nil, errors.New("git failed")
			}
		}
		return base(ctx, name, args...)
	}
	return env
}

// sshRemoteWorld is verifyWorld with the personal blog pushing over SSH with
// a key that no profile names.
func sshRemoteWorld(t *testing.T) (world, Env, string) {
	t.Helper()
	w, env, _ := verifyWorld(t)
	key := filepath.Join(w.home, ".ssh", "id_any")
	mustWrite(t, key, "not a real key\n", 0o600)
	runGit(t, w.repos["blog"], "remote", "set-url", "origin", "git@github.com:octo-alex/blog.git")
	return w, env, key
}

func TestVerifyCannotTellWhichAccountAnUnnamedSSHKeyPushesAs(t *testing.T) {
	_, env, key := sshRemoteWorld(t)
	env = withSSH(env, func(string) string { return "hostname github.com\nidentityfile " + key + "\n" })
	var out bytes.Buffer
	env.Stdout = &out

	err := Verify(context.Background(), env, VerifyOptions{})

	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"? ~/Projects/personal/blog",
		"as an account GitPersona cannot tell",
		"1 unconfirmed",
		"`gitpersona verify --ssh` asks the SSH hosts",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output should hold %q:\n%s", want, out.String())
		}
	}
}

func TestVerifyNamesSSHAgentOrDefaultKeysWhenNoKeyFileIsConfigured(t *testing.T) {
	_, env, _ := sshRemoteWorld(t)
	env = withSSH(env, func(string) string { return "hostname github.com\n" })

	c := repoCheck(t, verify(t, env), "~/Projects/personal/blog")

	if c.Remotes[0].Via != "ssh-agent or ssh's default keys" || c.Verdict != VerdictUnconfirmed {
		t.Errorf("check = %+v", c)
	}
}

func TestVerifyReadsWhatTheSSHHostSaysAboutTheKeyThatSignedIn(t *testing.T) {
	const noAccount = "Authenticated to github.com ([192.0.2.1]:22) using \"publickey\".\n"
	for name, tc := range map[string]struct {
		stderr, via string
		exit        int
		wantProblem string
	}{
		"a key file the host accepted": {
			stderr: "debug1: Server accepts key: KEYFILE ED25519 SHA256:demo\nHi octo-alex! You've successfully authenticated, but GitHub does not provide shell access.\n",
			exit:   1, via: "SSH key ~/.ssh/id_any, github.com greets octo-alex",
		},
		"a key from the ssh-agent": {
			stderr: "debug1: Server accepts key: alex@demo.example ED25519 SHA256:demo\nHi octo-alex! You've successfully authenticated, but GitHub does not provide shell access.\n",
			exit:   1, via: "a key from ssh-agent, github.com greets octo-alex",
		},
		"a deploy key": {
			stderr: "Hi acme-corp/billing! You've successfully authenticated, but GitHub does not provide shell access.\n",
			exit:   1, via: ", a deploy key for acme-corp/billing", wantProblem: "do not sign in as an account",
		},
		"a refused key": {
			stderr: "git@github.com: Permission denied (publickey).\n",
			exit:   255, via: ", refused: git@github.com: Permission denied (publickey).", wantProblem: "do not sign in as an account",
		},
		"a host that names no account": {
			stderr: noAccount, via: ", the host did not say which account", wantProblem: "do not sign in as an account",
		},
	} {
		t.Run(name, func(t *testing.T) {
			w, env, key := sshRemoteWorld(t)
			fakeSSH := filepath.Join(w.home, "fake-bin", "ssh")
			script := "#!/bin/sh\ncat >&2 <<'EOF'\n" + strings.ReplaceAll(tc.stderr, "KEYFILE", key) + "EOF\nexit " + strconv.Itoa(tc.exit) + "\n"
			mustWrite(t, fakeSSH, script, 0o700)
			env = withSSH(env, func(string) string { return "hostname github.com\nidentityfile " + key + "\n" })
			env.LookPath = func(name string) (string, error) {
				if name == "ssh" {
					return fakeSSH, nil
				}
				return "", os.ErrNotExist
			}

			r, err := env.VerifyRepos(context.Background(), nil, true)
			if err != nil {
				t.Fatal(err)
			}

			c := repoCheck(t, r, "~/Projects/personal/blog")
			if !strings.Contains(c.Remotes[0].Via, tc.via) {
				t.Errorf("via = %q, want it to contain %q", c.Remotes[0].Via, tc.via)
			}
			if tc.wantProblem != "" && !strings.Contains(findingText(c), tc.wantProblem) {
				t.Errorf("findings = %s, want %q", findingText(c), tc.wantProblem)
			}
		})
	}
}

func TestVerifyNotesWhoChoseTheSSHCommand(t *testing.T) {
	t.Run("an environment variable", func(t *testing.T) {
		_, env, key := sshRemoteWorld(t)
		t.Setenv("GIT_SSH_COMMAND", "ssh -i "+key)
		env = withSSH(env, func(string) string { return "hostname github.com\n" })

		c := repoCheck(t, verify(t, env), "~/Projects/personal/blog")

		if c.Remotes[0].ChosenBy != "GIT_SSH_COMMAND" {
			t.Errorf("chosen by %q", c.Remotes[0].ChosenBy)
		}
	})
	t.Run("the repository", func(t *testing.T) {
		w, env, key := sshRemoteWorld(t)
		runGit(t, w.repos["blog"], "config", "--local", "core.sshCommand", "ssh -i "+key)
		env = withSSH(env, func(string) string { return "hostname github.com\n" })

		c := repoCheck(t, verify(t, env), "~/Projects/personal/blog")

		if c.Remotes[0].ChosenBy != "core.sshCommand in the repository" {
			t.Errorf("chosen by %q", c.Remotes[0].ChosenBy)
		}
	})
}

func TestVerifyIgnoresKeysThatAreNotFilesAndPathsItCannotExpand(t *testing.T) {
	_, env, key := sshRemoteWorld(t)
	env = withSSH(env, func(string) string {
		return "hostname github.com\nidentityfile ~nobody/id_x\nidentityfile " + filepath.Dir(key) + "\nidentityfile " + key + "\n"
	})

	c := repoCheck(t, verify(t, env), "~/Projects/personal/blog")

	if len(c.Remotes[0].Keys) != 1 || c.Remotes[0].Keys[0] != "~/.ssh/id_any" {
		t.Errorf("keys = %v, want only ~/.ssh/id_any", c.Remotes[0].Keys)
	}
}

func TestVerifyLeavesRemotesThatAreNotNetworkAddressesAlone(t *testing.T) {
	w, env, _ := verifyWorld(t)
	runGit(t, w.repos["blog"], "remote", "add", "backup", filepath.Join(w.home, "backup.git"))

	c := repoCheck(t, verify(t, env), "~/Projects/personal/blog")

	if len(c.Remotes) != 2 || c.Verdict != VerdictOK {
		t.Errorf("check = %+v", c)
	}
}

func TestVerifySkipsRemotesGitCannotDescribe(t *testing.T) {
	w, env, _ := verifyWorld(t)
	env = failGit(env, "remote", "-v")

	c := repoCheck(t, verify(t, env), "~/Projects/personal/blog")

	if len(c.Remotes) != 0 {
		t.Errorf("remotes = %+v, want none when git cannot give their URLs", c.Remotes)
	}
	env = failGit(w.mustEnv(t), "remote")
	if c := repoCheck(t, verify(t, env), "~/Projects/personal/blog"); len(c.Remotes) != 0 {
		t.Errorf("remotes = %+v, want none when git cannot list them", c.Remotes)
	}
}

func (w world) mustEnv(t *testing.T) Env {
	t.Helper()
	env, _ := w.env(t, w.home)
	return env
}

func TestVerifyTreatsAGHThatCannotBeAskedAsKnowingNoAccount(t *testing.T) {
	_, env, _ := verifyWorld(t)
	base := env.Run
	env.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name != "git" {
			return nil, errors.New("gh failed")
		}
		return base(ctx, name, args...)
	}

	r := verify(t, env)

	if blog := repoCheck(t, r, "~/Projects/personal/blog"); blog.Verdict != VerdictOK {
		t.Errorf("blog = %+v; a gh that cannot be asked must not raise a problem", blog)
	}
}

func TestVerifyCountsRepositoriesOutsideEveryProfileAndShowsNoAuthor(t *testing.T) {
	w, env, out := verifyWorld(t)
	cfgFile := filepath.Join(w.home, ".config", "gitpersona", "config.toml")
	mustWrite(t, cfgFile, strings.Replace(readFile(t, cfgFile), "default = \"acme\"\n", "", 1), 0o600)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	scratch := w.repos["scratch"]

	if err := Verify(context.Background(), env, VerifyOptions{Dirs: []string{scratch}}); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"- ~/Desktop/scratch  (no profile: Git uses your own settings)",
		"author  no author",
		"1 outside your profiles",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output should hold %q:\n%s", want, out.String())
		}
	}
}

func TestVerifyWarnsWhenTheScanDidNotGoDeepEnough(t *testing.T) {
	w, env, out := verifyWorld(t)
	deep := filepath.Join(w.home, "Projects", "personal", "a", "b", "c", "d", "e", "f")
	runGit(t, w.home, "init", "-q", deep)

	if err := Verify(context.Background(), env, VerifyOptions{}); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "the scan stopped early or did not go deep enough") {
		t.Errorf("output:\n%s", out.String())
	}
}

func TestVerifyRefusesAFolderItCannotExpand(t *testing.T) {
	_, env, _ := verifyWorld(t)

	err := Verify(context.Background(), env, VerifyOptions{Dirs: []string{"~nobody/code"}})

	if err == nil {
		t.Error("Verify with a ~user folder succeeded")
	}
}

func TestVerifyAndWhoamiReportWhenTheirOutputCannotBeWritten(t *testing.T) {
	_, env, _ := verifyWorld(t)
	env.Stdout = failingWriter{}

	if err := Verify(context.Background(), env, VerifyOptions{JSON: true}); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Errorf("Verify = %v", err)
	}
	if err := WhoamiWith(context.Background(), env, WhoamiOptions{JSON: true}); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Errorf("Whoami = %v", err)
	}
	if err := Shells(context.Background(), env, true); err == nil {
		t.Error("Shells with unwritable output succeeded")
	}
}
