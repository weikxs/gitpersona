package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/shell"
)

func TestGlabShimExplainsWhenTheRealGlabIsMissing(t *testing.T) {
	w := newWorld(t)
	env, stderr, call := shimEnv(t, w, w.repos["hotfix"])
	t.Setenv("PATH", env.shimDir())

	code := GlabShim(context.Background(), env, []string{"mr", "list"})

	if code == 0 || !strings.Contains(stderr.String(), "could not find the real glab") || call.path != "" {
		t.Errorf("code %d, stderr %q, exec %q", code, stderr.String(), call.path)
	}
}

func TestGlabShimReportsAnExecFailure(t *testing.T) {
	_, env, stderr, _ := glabWorld(t)
	env.Exec = func(string, []string, []string) error { return errors.New("boom") }

	code := GlabShim(context.Background(), env, []string{"mr", "list"})

	if code == 0 || !strings.Contains(stderr.String(), "boom") {
		t.Errorf("code %d, stderr %q", code, stderr.String())
	}
}

func TestBareHostGivesNothingForAnAddressThatIsNotAURL(t *testing.T) {
	if got := bareHost("http://[::1"); got != "" {
		t.Errorf("bareHost(broken URL) = %q, want nothing", got)
	}
}

func TestTheRealToolIsFoundPastDirectoriesFilesAndGitPersonaItself(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	root := filepath.Dir(w.home)
	notExecutable := filepath.Join(root, "no-exec")
	mustWrite(t, filepath.Join(notExecutable, "glab"), "#!/bin/sh\n", 0o600)
	selfDir := filepath.Join(root, "self")
	if err := os.MkdirAll(selfDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(binary, filepath.Join(selfDir, "glab")); err != nil {
		t.Fatal(err)
	}
	realDir := filepath.Join(root, "real")
	mustWrite(t, filepath.Join(realDir, "glab"), "#!/bin/sh\n", 0o700)
	dirNamedLikeTheTool := filepath.Join(root, "dir-tool")
	if err := os.MkdirAll(filepath.Join(dirNamedLikeTheTool, "glab"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", strings.Join([]string{notExecutable, dirNamedLikeTheTool, selfDir, realDir}, string(os.PathListSeparator)))

	got, err := env.realTool("glab")

	if err != nil || got != filepath.Join(realDir, "glab") {
		t.Errorf("realTool = %q, %v, want the executable that is not GitPersona", got, err)
	}
}

func TestTheShimsRunWithoutAProfileWhenTheConfigurationIsBroken(t *testing.T) {
	w := newWorld(t)
	env, _, call := shimEnv(t, w, w.repos["billing"])
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"), "version = [\n", 0o600)

	code := GHShim(context.Background(), env, []string{"pr", "list"})

	if _, set := ghToken(call.env); code != 0 || set {
		t.Errorf("code %d, token set %v; with a broken configuration gh must run as it is", code, set)
	}
}

func TestShellIntegrationScanSkipsStartupFilesItCannotRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("file permissions do not stop root")
	}
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	switcher := "gh() {\n  GH_TOKEN=$(gh auth token) command gh \"$@\"\n}\n"
	mustWrite(t, filepath.Join(w.home, ".bashrc"), switcher, 0o600)
	mustWrite(t, filepath.Join(w.home, ".zshrc"), switcher, 0o600)
	if err := os.Chmod(filepath.Join(w.home, ".zshrc"), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(w.home, ".zshrc"), 0o600) })

	got := env.ghSwitchers()

	if len(got) != 1 || got[0] != "~/.bashrc" {
		t.Errorf("ghSwitchers = %v, want only ~/.bashrc", got)
	}
}

func TestFromOSDetectsTheShellsOfTheRunningSystem(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	env, err := FromOS(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	login := env.LoginShell(ctx)
	current := env.CurrentShell(ctx)
	installed := env.InstalledShells()

	for _, d := range []shell.Detected{login, current} {
		if d.Kind != shell.Unknown && d.Path == "" {
			t.Errorf("a detected shell has a kind but no path: %+v", d)
		}
	}
	for _, in := range installed {
		if in.Path == "" {
			t.Errorf("an installed shell has no path: %+v", in)
		}
	}
}

func TestGitSettingsOfTheSystemAreReadBeforeTheUsersOwn(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	system := filepath.Join(filepath.Dir(w.home), "system.gitconfig")
	mustWrite(t, system, "[credential]\n\thelper = demo-system\n", 0o600)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "")
	t.Setenv("GIT_CONFIG_SYSTEM", system)
	appendConfig(t, w, "[credential]\n\thelper = demo-user\n")

	entries, err := env.gitConfigRegexp(context.Background(), `^credential\.helper$`)

	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0][1] != "demo-system" || entries[1][1] != "demo-user" {
		t.Errorf("entries = %v, want the system helper first", entries)
	}
}
