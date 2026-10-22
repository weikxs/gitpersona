package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStateGathersEverythingThePageNeeds(t *testing.T) {
	w := newWorld(t)
	env, _ := shellEnv(t, w)
	ghLog := filepath.Join(t.TempDir(), "gh.log")
	t.Setenv("GH_LOG", ghLog)

	got := State(context.Background(), env)

	if !got.ConfigExists || got.ConfigError != "" || len(got.Config.Profiles) != 3 || got.Config.Default != "acme" {
		t.Fatalf("config part = %+v", got)
	}
	if len(got.Accounts) != 2 || got.Accounts[0].Login != "octo-alex" || !got.Accounts[0].Active {
		t.Errorf("accounts = %+v", got.Accounts)
	}
	if got.Shells.Default.Kind != "fish" || len(got.Doctor.Checks) == 0 {
		t.Errorf("shells/doctor = %+v / %+v", got.Shells, got.Doctor)
	}
	if calls, _ := os.ReadFile(ghLog); strings.Contains(string(calls), "auth token") {
		t.Fatalf("the page state must never fetch a token; gh calls:\n%s", calls)
	}
}

func TestStateWithoutConfig(t *testing.T) {
	w := newWorld(t)
	if err := os.Remove(filepath.Join(w.home, ".config", "gitpersona", "config.toml")); err != nil {
		t.Fatal(err)
	}
	env, _ := w.env(t, w.home)

	got := State(context.Background(), env)

	if got.ConfigExists || got.ConfigError != "" {
		t.Fatalf("missing config should be reported as not existing, got %+v", got)
	}
}

func TestPreviewListsDiffsWithoutWriting(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)

	changes, err := Preview(context.Background(), env)

	if err != nil || len(changes) < 5 {
		t.Fatalf("Preview() = %+v, %v", changes, err)
	}
	var gitconfig FileChange
	for _, c := range changes {
		if c.Path == "~/.gitconfig" {
			gitconfig = c
		}
	}
	if gitconfig.Action != "create" || !strings.Contains(gitconfig.Diff, "+# >>> gitpersona >>>") {
		t.Fatalf("~/.gitconfig preview = %+v", gitconfig)
	}
	if _, err := os.Stat(filepath.Join(w.home, ".gitconfig")); !os.IsNotExist(err) {
		t.Fatal("Preview() wrote a file")
	}
}
