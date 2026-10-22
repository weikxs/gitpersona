package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/block"
	"github.com/weikxs/gitpersona/internal/credential"
)

func TestConfigDirFollowsXDG(t *testing.T) {
	tests := []struct {
		name string
		xdg  string
		want string
	}{
		{name: "unset uses ~/.config", xdg: "", want: "/tmp/home/.config/gitpersona"},
		{name: "absolute XDG_CONFIG_HOME", xdg: "/tmp/xdg", want: "/tmp/xdg/gitpersona"},
		{name: "relative XDG_CONFIG_HOME is ignored per the spec", xdg: "relative/xdg", want: "/tmp/home/.config/gitpersona"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", tc.xdg)

			if got := configDir("/tmp/home"); got != tc.want {
				t.Fatalf("configDir() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestFromOSDescribesTheRunningProcess(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	env, err := FromOS(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})

	if err != nil {
		t.Fatalf("FromOS() error: %v", err)
	}
	if env.Home != home || env.ConfigDir != filepath.Join(home, ".config", "gitpersona") {
		t.Errorf("FromOS() home/config = %s, %s", env.Home, env.ConfigDir)
	}
	if env.WorkDir == "" || !filepath.IsAbs(env.Executable) || env.Now == nil || env.Run == nil || env.LookPath == nil {
		t.Errorf("FromOS() left fields empty: %+v", env)
	}
}

func TestFromOSFailsWithoutHome(t *testing.T) {
	t.Setenv("HOME", "")

	if _, err := FromOS(nil, nil, nil); err == nil {
		t.Fatal("FromOS() without HOME succeeded, want an error")
	}
}

func TestCredentialInProcess(t *testing.T) {
	w := newWorld(t)
	target := credential.Target{Provider: "github", Host: "github.com", Account: "acme-alex"}

	answered, _ := w.env(t, w.home)
	answered.Stdin = strings.NewReader("protocol=https\nhost=github.com\n\n")
	var out bytes.Buffer
	answered.Stdout = &out
	if err := Credential(context.Background(), answered, "get", target); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "password=token-for-acme-alex") {
		t.Errorf("Credential() output = %q, want the token from gh", out.String())
	}

	noGH, _ := w.env(t, w.home)
	noGH.Stdin = strings.NewReader("protocol=https\nhost=github.com\n\n")
	var silent bytes.Buffer
	noGH.Stdout = &silent
	noGH.LookPath = func(string) (string, error) { return "", errors.New("not found") }
	if err := Credential(context.Background(), noGH, "get", target); err != nil || silent.Len() != 0 {
		t.Errorf("Credential() without gh = %v, %q; want silence", err, silent.String())
	}

	if err := Credential(context.Background(), noGH, "approve", target); err == nil {
		t.Error("Credential() with an unknown operation succeeded, want an error")
	}
}

func TestDoctorWithoutGH(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.home)
	env.LookPath = func(string) (string, error) { return "", errors.New("not found") }

	if Doctor(context.Background(), env) {
		t.Fatal("Doctor() passed without gh")
	}
	if !strings.Contains(out.String(), "https://cli.github.com") {
		t.Errorf("doctor did not explain how to get gh:\n%s", out.String())
	}
}

func TestDoctorAccountNotSignedIn(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"),
		strings.Replace(demoConfig, `account = "acme-alex"`, `account = "nobody"`, 1), 0o600)
	failingGH := filepath.Join(t.TempDir(), "gh")
	mustWrite(t, failingGH, "#!/bin/sh\nfor a; do [ \"$a\" = nobody ] && exit 1; done\necho token\n", 0o700)
	env, out := w.env(t, w.home)
	env.LookPath = func(string) (string, error) { return failingGH, nil }

	if Doctor(context.Background(), env) {
		t.Fatal("Doctor() passed with an account that is not signed in")
	}
	if !strings.Contains(out.String(), "nobody is not signed in") || !strings.Contains(out.String(), "gh auth login --hostname github.com") {
		t.Errorf("doctor output:\n%s", out.String())
	}
}

func TestDoctorSkipsGHWhenNoGitHubProfiles(t *testing.T) {
	w := newWorld(t)
	gitlabOnly := `version = 1
[[profile]]
id = "globex"
provider = "gitlab"
host = "gitlab.globex.example"
account = "globex-alex"
name = "Alex"
email = "alex@globex.example"
dirs = ["~/Projects/clients/globex"]
`
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"), gitlabOnly, 0o600)
	env, out := w.env(t, w.home)
	if err := Apply(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	if !Doctor(context.Background(), env) || strings.Contains(out.String(), "GitHub CLI") {
		t.Fatalf("doctor with only GitLab profiles should pass without checking gh:\n%s", out.String())
	}
}

func TestApplyAndDoctorReportBrokenMarkers(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), block.Begin+"\n[user]\n", 0o644)
	env, out := w.env(t, w.home)

	err := Apply(context.Background(), env)

	if err == nil || !strings.Contains(err.Error(), ".gitconfig") {
		t.Fatalf("Apply() error = %v, want it to name ~/.gitconfig", err)
	}
	if Doctor(context.Background(), env) || !strings.Contains(out.String(), "generated Git configuration:") {
		t.Fatalf("doctor should report the broken markers:\n%s", out.String())
	}
}

func TestApplyWithoutConfigExplainsWhatIsMissing(t *testing.T) {
	w := newWorld(t)
	if err := os.Remove(filepath.Join(w.home, ".config", "gitpersona", "config.toml")); err != nil {
		t.Fatal(err)
	}
	env, _ := w.env(t, w.home)

	err := Apply(context.Background(), env)

	if err == nil || !strings.Contains(err.Error(), "config.toml") {
		t.Fatalf("Apply() error = %v, want it to name the missing config file", err)
	}
}

func TestWhoamiOutsideARepositoryUsesTheDirectory(t *testing.T) {
	w := newWorld(t)
	plain := filepath.Join(w.home, "Projects", "personal", "notes")
	if err := os.MkdirAll(plain, 0o700); err != nil {
		t.Fatal(err)
	}
	env, out := w.env(t, plain)

	if err := Whoami(context.Background(), env, true); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), `"in_repository": false`) || !strings.Contains(out.String(), `"profile": "personal"`) {
		t.Fatalf("whoami --json outside a repository:\n%s", out.String())
	}
}

func TestDisplayKeepsPathsOutsideHome(t *testing.T) {
	env := Env{Home: "/tmp/home"}

	if got := env.display("/srv/repos/x"); got != "/srv/repos/x" {
		t.Fatalf("display() = %s, want the absolute path unchanged", got)
	}
	if got := env.display("/tmp/home/Projects/x"); got != "~/Projects/x" {
		t.Fatalf("display() = %s, want ~/Projects/x", got)
	}
}

func TestReadOptionalReportsUnreadableFiles(t *testing.T) {
	if _, err := readOptional(t.TempDir()); err == nil {
		t.Fatal("readOptional() of a directory succeeded, want an error")
	}
	if got, err := readOptional(filepath.Join(t.TempDir(), "absent")); err != nil || got != "" {
		t.Fatalf("readOptional() of a missing file = %q, %v; want empty and no error", got, err)
	}
}
