package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	t.Chdir(t.TempDir())
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "version flag prints build info", args: []string{"--version"}, wantCode: 0, wantStdout: "gitpersona dev"},
		{name: "version command prints build info", args: []string{"version"}, wantCode: 0, wantStdout: "gitpersona dev"},
		{name: "help flag prints usage", args: []string{"--help"}, wantCode: 0, wantStdout: "Usage:"},
		{name: "no arguments prints usage", args: nil, wantCode: 0, wantStdout: "Usage:"},
		{name: "unknown command fails with usage hint", args: []string{"bogus"}, wantCode: 2, wantStderr: `unknown command "bogus"`},
		{name: "credential without target is a usage error", args: []string{"credential", "get"}, wantCode: 2, wantStderr: "--provider P --host H --account A"},
		{name: "unknown flag is a usage error", args: []string{"whoami", "--bogus"}, wantCode: 2, wantStderr: "flag provided but not defined"},
		{name: "subcommand help goes to stdout", args: []string{"apply", "--help"}, wantCode: 0, wantStdout: "Usage: gitpersona apply"},
		{name: "help for a command", args: []string{"help", "whoami"}, wantCode: 0, wantStdout: "Usage: gitpersona whoami"},
		{name: "help for an unknown command", args: []string{"help", "bogus"}, wantCode: 2, wantStderr: `unknown command "bogus"`},
		{name: "json is only for whoami", args: []string{"doctor", "--json"}, wantCode: 2, wantStderr: "flag provided but not defined"},
		{name: "extra arguments are rejected", args: []string{"apply", "extra"}, wantCode: 2, wantStderr: "unexpected argument"},
		{name: "version rejects extra arguments", args: []string{"version", "extra"}, wantCode: 2, wantStderr: "unexpected argument"},
		{name: "help rejects extra arguments", args: []string{"help", "apply", "extra"}, wantCode: 2, wantStderr: "unexpected argument"},
		{name: "help shows double-dash flags", args: []string{"help", "apply"}, wantCode: 0, wantStdout: "--dry-run"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code := run(tc.args, strings.NewReader(""), &stdout, &stderr)

			if code != tc.wantCode {
				t.Fatalf("exit code = %d, want %d (stderr: %q)", code, tc.wantCode, stderr.String())
			}
			if !strings.Contains(stdout.String(), tc.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tc.wantStdout)
			}
			if !strings.Contains(stderr.String(), tc.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tc.wantStderr)
			}
		})
	}
}

// demoHome prepares a demo home under the test temp dir (which make test puts
// under /tmp) with one profile and a fake gh, and isolates Git from the user.
func demoHome(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "bin")
	write := func(path, content string, perm os.FileMode) {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), perm); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(bin, "gh"), `#!/bin/sh
case "$1 $2" in
"auth token") echo token ;;
"auth status") echo '{"hosts":{"github.com":[{"login":"acme-alex","state":"success"}]}}' ;;
esac
`, 0o700)
	write(filepath.Join(home, ".config", "gitpersona", "config.toml"), `version = 1
default = "acme"
[[profile]]
id = "acme"
account = "acme-alex"
name = "Alex (Acme)"
email = "1000002+acme-alex@users.noreply.github.com"
dirs = ["~/Projects/acme"]
`, 0o600)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	// Commands act on the current directory; never let them see the real one.
	t.Chdir(home)
	return home
}

func TestCommandsEndToEnd(t *testing.T) {
	demoHome(t)
	steps := []struct {
		args       []string
		stdin      string
		wantCode   int
		wantStdout string
	}{
		{args: []string{"doctor"}, wantCode: 1, wantStdout: "run `gitpersona apply`"},
		{args: []string{"apply"}, wantCode: 0, wantStdout: "Backup saved in"},
		{args: []string{"doctor"}, wantCode: 0, wantStdout: "is up to date"},
		{args: []string{"whoami", "--json"}, wantCode: 0, wantStdout: `"profile": "acme"`},
		{args: []string{"list"}, wantCode: 0, wantStdout: "acme (default)"},
		{args: []string{"use", "ghost"}, wantCode: 1},
		{args: []string{"remove", "~/nowhere"}, wantCode: 1},
		{args: []string{"pin", "acme"}, wantCode: 1},
		{args: []string{"unpin"}, wantCode: 1},
		{args: []string{"rollback"}, wantCode: 0, wantStdout: "(latest)"},
		{args: []string{"init", "fish"}, wantCode: 0, wantStdout: "gitpersona_prompt"},
		{args: []string{"shells"}, wantCode: 0, wantStdout: "installed shells:"},
		{args: []string{"init", "tcsh"}, wantCode: 2},
		{args: []string{"prompt"}, wantCode: 0, wantStdout: "acme-alex"},
		{args: []string{"install-shell", "--shell", "bash", "--dry-run"}, wantCode: 0, wantStdout: "would create"},
		{args: []string{"install-shell", "--shell", "csh"}, wantCode: 2},
		{args: []string{"uninstall"}, wantCode: 1},
		{args: []string{"uninstall", "--yes"}, wantCode: 0, wantStdout: "GitPersona is uninstalled"},
		{args: []string{"credential", "--provider", "github", "--host", "github.com", "--account", "acme-alex", "get"},
			stdin: "protocol=https\nhost=github.com\n\n", wantCode: 0, wantStdout: "password=token"},
		{args: []string{"credential", "--provider", "github", "--host", "github.com", "--account", "acme-alex", "bogus"},
			wantCode: 1},
	}

	for _, s := range steps {
		var stdout, stderr bytes.Buffer

		code := run(s.args, strings.NewReader(s.stdin), &stdout, &stderr)

		if code != s.wantCode || !strings.Contains(stdout.String(), s.wantStdout) {
			t.Fatalf("gitpersona %v: exit %d, stdout %q, stderr %q; want exit %d and stdout containing %q",
				s.args, code, stdout.String(), stderr.String(), s.wantCode, s.wantStdout)
		}
	}
}

func TestCommandFailsWithoutConfig(t *testing.T) {
	home := demoHome(t)
	if err := os.Remove(filepath.Join(home, ".config", "gitpersona", "config.toml")); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer

	code := run([]string{"apply"}, strings.NewReader(""), &stdout, &stderr)

	if code != 1 || !strings.Contains(stderr.String(), "configuration file not found") {
		t.Fatalf("apply without config: exit %d, stderr %q", code, stderr.String())
	}
}

func TestCommandFailsWithoutHome(t *testing.T) {
	t.Setenv("HOME", "")
	var stdout, stderr bytes.Buffer

	if code := run([]string{"whoami"}, strings.NewReader(""), &stdout, &stderr); code != 1 {
		t.Fatalf("whoami without HOME: exit %d, stderr %q; want 1", code, stderr.String())
	}
}
