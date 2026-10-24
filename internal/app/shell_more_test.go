package app

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/block"
	"github.com/weikxs/gitpersona/internal/shell"
)

func TestInstallShellAsksForAShellWhenTheLoginShellIsUnknown(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)

	err := InstallShell(context.Background(), env, nil, false)

	if err == nil || !strings.Contains(err.Error(), "--shell fish, zsh or bash") {
		t.Errorf("InstallShell with an unknown login shell = %v", err)
	}
}

func TestShellsCallsAnUndetectedShellUnknown(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.home)

	if err := Shells(context.Background(), env, false); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "default  unknown") || !strings.Contains(out.String(), "current  unknown") {
		t.Errorf("output =\n%s", out.String())
	}
}

func TestShellCommandsRefuseAStartupFileWithABrokenGitPersonaBlock(t *testing.T) {
	w := newWorld(t)
	env, _ := shellEnv(t, w)
	mustWrite(t, fishFile(w), block.Begin+"\nset -x DEMO 1\n", 0o644)

	cases := map[string]error{
		"install-shell": InstallShell(context.Background(), env, []shell.Kind{shell.Fish}, false),
	}
	_, previewErr := PreviewShells(env, []shell.Kind{shell.Fish})
	cases["preview"] = previewErr

	for name, err := range cases {
		if err == nil || !strings.Contains(err.Error(), "gitpersona.fish") {
			t.Errorf("%s with a broken block = %v, want an error naming the file", name, err)
		}
	}
}

func TestShellCommandsReportAnUnreadableStartupFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("file permissions do not stop root")
	}
	w := newWorld(t)
	env, _ := shellEnv(t, w)
	mustWrite(t, fishFile(w), "set -x DEMO 1\n", 0o644)
	if err := os.Chmod(fishFile(w), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(fishFile(w), 0o644) })

	if err := InstallShell(context.Background(), env, []shell.Kind{shell.Fish}, false); err == nil {
		t.Error("InstallShell with an unreadable startup file succeeded")
	}
	if _, err := PreviewShells(env, []shell.Kind{shell.Fish}); err == nil {
		t.Error("PreviewShells with an unreadable startup file succeeded")
	}
}

func TestInstallShellStopsWhenTheBackupCannotBeMade(t *testing.T) {
	w := newWorld(t)
	env, _ := shellEnv(t, w)
	blockBackups(t, w)

	err := InstallShell(context.Background(), env, []shell.Kind{shell.Fish}, false)

	if err == nil {
		t.Error("InstallShell with an unwritable backup folder succeeded")
	}
	if _, statErr := os.Stat(fishFile(w)); statErr == nil {
		t.Error("the startup file was written without a backup")
	}
}

func TestDoctorConfirmsGHGoesThroughGitPersonaWhenThePathFindsItsEntry(t *testing.T) {
	w := newWorld(t)
	env, out := shellEnv(t, w)
	ctx := context.Background()
	if err := Apply(ctx, env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := InstallShell(ctx, env, []shell.Kind{shell.Fish}, false); err != nil {
		t.Fatal(err)
	}
	env.LookPath = func(name string) (string, error) {
		if name == "gh" {
			return env.ghEntry(), nil
		}
		return "", os.ErrNotExist
	}
	out.Reset()

	Doctor(ctx, env)

	if !strings.Contains(out.String(), "gh in the shell that started gitpersona goes through GitPersona") {
		t.Errorf("doctor should confirm the routing:\n%s", out.String())
	}
}
