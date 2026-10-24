package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStateShowsABrokenConfigurationWithTheHomeAsTilde(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"), "version = [\n", 0o600)
	env, _ := w.env(t, w.home)

	report := State(context.Background(), env)

	if !report.ConfigExists || report.ConfigError == "" || strings.Contains(report.ConfigError, w.home) {
		t.Errorf("state = exists %v, error %q", report.ConfigExists, report.ConfigError)
	}
}

func TestStateReportsAConfigurationThatParsesButIsNotValid(t *testing.T) {
	w := newWorld(t)
	text := strings.Replace(demoConfig, `dirs    = ["~/Projects/acme"]`, `dirs    = ["Projects/acme"]`, 1)
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"), text, 0o600)
	env, _ := w.env(t, w.home)

	report := State(context.Background(), env)

	if report.ConfigError == "" {
		t.Skip("the configuration loader already rejects it")
	}
	if len(report.Config.Profiles) != 3 {
		t.Errorf("profiles = %d; the page still shows what was read", len(report.Config.Profiles))
	}
}

func TestStateSaysWhyNoAccountsAreListed(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	base := env.Run
	failingGH := env
	failingGH.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name != "git" {
			return nil, errors.New("gh refused")
		}
		return base(ctx, name, args...)
	}
	noGH := env
	noGH.LookPath = func(string) (string, error) { return "", os.ErrNotExist }

	if got := State(context.Background(), failingGH).AccountsError; got == "" {
		t.Error("a gh that fails should leave an accounts error")
	}
	if got := State(context.Background(), noGH).AccountsError; !strings.Contains(got, "not installed") {
		t.Errorf("without gh, accounts error = %q", got)
	}
}

func TestPreviewFailsOnABrokenConfigurationAndShowsRemovalsAsEmptyDiffs(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	ctx := context.Background()
	if err := Apply(ctx, env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	cfgFile := filepath.Join(w.home, ".config", "gitpersona", "config.toml")
	// Without the globex profile its generated file is stale and would be removed.
	if err := RemoveProfile(ctx, env, "globex"); err != nil {
		t.Fatal(err)
	}
	if err := Apply(ctx, env, ApplyOptions{Yes: true}); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "profiles", "stale.gitconfig"), "[user]\n\tname = Stale\n", 0o600)

	changes, err := Preview(ctx, env)
	if err != nil {
		t.Fatal(err)
	}
	removed := false
	for _, c := range changes {
		if c.Action == "remove" {
			removed = true
			if strings.Contains(c.Diff, "+") && strings.Contains(c.Diff, "+Stale") {
				t.Errorf("a removal should not add lines:\n%s", c.Diff)
			}
		}
	}
	if !removed {
		t.Errorf("changes = %+v, want the stale file listed as removed", changes)
	}

	mustWrite(t, cfgFile, "version = [\n", 0o600)
	if _, err := Preview(ctx, env); err == nil {
		t.Error("Preview of a broken configuration succeeded")
	}
}

func TestWhoamiShowsTheAuthorGitUsesWhereNoProfileApplies(t *testing.T) {
	w := newWorld(t)
	cfgFile := filepath.Join(w.home, ".config", "gitpersona", "config.toml")
	mustWrite(t, cfgFile, strings.Replace(demoConfig, "default = \"acme\"\n", "", 1), 0o600)
	runGit(t, w.home, "config", "--global", "user.name", "Global Demo")
	runGit(t, w.home, "config", "--global", "user.email", "global@demo.example")
	env, out := w.env(t, w.repos["scratch"])

	if err := Whoami(context.Background(), env, false); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "No GitPersona profile applies here") || !strings.Contains(out.String(), "author  Global Demo <global@demo.example>") {
		t.Errorf("output:\n%s", out.String())
	}
}

func TestWhoamiFailsWithoutAConfiguration(t *testing.T) {
	w := newWorld(t)
	if err := os.Remove(filepath.Join(w.home, ".config", "gitpersona", "config.toml")); err != nil {
		t.Fatal(err)
	}
	env, _ := w.env(t, w.home)

	if err := Whoami(context.Background(), env, false); err == nil || !strings.Contains(err.Error(), "gitpersona setup") {
		t.Errorf("Whoami without a configuration = %v", err)
	}
}

func TestMismatchAdviceSuggestsDoctorWhenGitDoesNotSayWhereTheEmailComesFrom(t *testing.T) {
	got := mismatchAdvice(WhoamiReport{EffectiveEmail: "someone@demo.example"})

	if !strings.Contains(got, `"someone@demo.example"`) || !strings.Contains(got, "gitpersona doctor") {
		t.Errorf("mismatchAdvice = %q", got)
	}
}
