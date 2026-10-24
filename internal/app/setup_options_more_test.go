package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/shell"
)

// setupWorld is a demo home with no configuration yet, so setup can run.
func setupWorld(t *testing.T) (world, Env, *strings.Builder) {
	t.Helper()
	w := newWorld(t)
	if err := os.Remove(filepath.Join(w.home, ".config", "gitpersona", "config.toml")); err != nil {
		t.Fatal(err)
	}
	env, _ := w.env(t, w.home)
	var out strings.Builder
	env.Stdout = &out
	return w, env, &out
}

func TestSetupRefusesBothADefaultAndNoDefault(t *testing.T) {
	_, env, _ := setupWorld(t)

	err := Setup(context.Background(), env, SetupOptions{Yes: true, NoDefault: true, Default: "octo-alex"})

	if err == nil || !strings.Contains(err.Error(), "either --default or --no-default") {
		t.Errorf("Setup = %v", err)
	}
}

func TestSetupTellsToOpenANewTerminalWhenItInstalledAShellIntegration(t *testing.T) {
	w, env, out := setupWorld(t)
	env.LoginShell = func(context.Context) shell.Detected {
		return shell.Detected{Path: "/opt/homebrew/bin/fish", Kind: shell.Fish, Source: "dscl"}
	}

	if err := Setup(context.Background(), env, SetupOptions{Yes: true, KeepOldRules: true}); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}

	if !strings.Contains(out.String(), "Open a new terminal") {
		t.Errorf("setup should say to open a new terminal:\n%s", out.String())
	}
	if _, err := os.Stat(fishFile(w)); err != nil {
		t.Errorf("the fish integration was not installed: %v", err)
	}
}

func TestSetupStopsWhenNothingCanBeBackedUpAndSaysSo(t *testing.T) {
	w, env, out := setupWorld(t)
	blockBackups(t, w)

	err := Setup(context.Background(), env, SetupOptions{Yes: true})

	if err == nil {
		t.Errorf("Setup with an unwritable backup folder succeeded:\n%s", out.String())
	}
	if _, statErr := os.Stat(filepath.Join(w.home, ".config", "gitpersona", "config.toml")); statErr == nil {
		t.Error("a configuration was written without a backup")
	}
}
