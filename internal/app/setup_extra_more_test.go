package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupProposalLeavesOutAccountsThatAreNotSignedIn(t *testing.T) {
	w, env, _ := setupWorld(t)
	mustWrite(t, filepath.Join(filepath.Dir(w.home), "bin", "gh"), `#!/bin/sh
case "$1 $2" in
"auth status") printf '{"hosts":{"github.com":[{"login":"octo-alex","state":"success","active":true},{"login":"old-alex","state":"error","active":false}]}}\n' ;;
"api --hostname") exit 1 ;;
esac
`, 0o700)

	got, err := ProposeSetup(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	if len(got.Profiles) != 1 || got.Profiles[0].Account != "octo-alex" {
		t.Fatalf("profiles = %+v, want only the signed-in account", got.Profiles)
	}
	if got.Profiles[0].NameSource != "login" {
		t.Errorf("name source = %q; when the profile cannot be read the login is the name", got.Profiles[0].NameSource)
	}
}

func TestSetupPlanIsRefusedWhenGitCannotReadItsSettings(t *testing.T) {
	w, env, _ := setupWorld(t)
	ctx := context.Background()
	proposal, err := ProposeSetup(ctx, env)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[user\n\tname = broken\n", 0o600)

	if _, err := PreviewSetup(ctx, env, proposal.Plan()); err == nil {
		t.Error("PreviewSetup with an unreadable ~/.gitconfig succeeded")
	}
}

func TestSetupPlanIsRefusedWhenAShellStartupFileIsBroken(t *testing.T) {
	w, env, _ := setupWorld(t)
	ctx := context.Background()
	proposal, err := ProposeSetup(ctx, env)
	if err != nil {
		t.Fatal(err)
	}
	plan := proposal.Plan()
	plan.Shells = []string{"fish"}
	mustWrite(t, fishFile(w), "# >>> gitpersona >>>\nset -x DEMO 1\n", 0o644)

	if _, err := PreviewSetup(ctx, env, plan); err == nil {
		t.Error("PreviewSetup with a broken shell block succeeded")
	}
}

func TestPromptShowsNothingInARepositoryNoProfileCovers(t *testing.T) {
	w := newWorld(t)
	cfgFile := filepath.Join(w.home, ".config", "gitpersona", "config.toml")
	mustWrite(t, cfgFile, strings.Replace(demoConfig, "default = \"acme\"\n", "", 1), 0o600)
	env, out := w.env(t, w.repos["scratch"])

	Prompt(env, "")

	if out.Len() != 0 {
		t.Errorf("prompt = %q, want nothing", out.String())
	}
}

func TestScanRootsWriteAFolderOutsideEveryRootAsTheDiskHasIt(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	roots, err := env.ResolveScanRoots([]string{"~/Projects"})
	if err != nil {
		t.Fatal(err)
	}

	got := roots.Written(env, filepath.Join(w.home, "Desktop", "scratch"))

	if got != "~/Desktop/scratch" {
		t.Errorf("Written = %q", got)
	}
}

func TestPathsTheConfigurationCannotExpandAreSkippedByTheChecks(t *testing.T) {
	w := newWorld(t)
	cfg := strings.Replace(demoConfig, `dirs    = ["~/Projects/acme"]`, `dirs    = ["~nobody/acme"]`+"\n"+`ssh_key = "~nobody/id_acme"`, 1)
	cfg = strings.Replace(cfg, `repo    = "~/Downloads/globex-hotfix"`, `repo    = "~nobody/hotfix"`, 1)
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"), cfg, 0o600)
	env, out := w.env(t, w.home)

	ok := Doctor(context.Background(), env)
	sshErr := CheckSSH(context.Background(), env, CheckSSHOptions{Profile: "acme"})

	if ok {
		t.Errorf("doctor passed with paths it cannot expand:\n%s", out.String())
	}
	if sshErr == nil || !strings.Contains(out.String(), "ssh_key ~nobody/id_acme") {
		t.Errorf("check-ssh = %v\n%s", sshErr, out.String())
	}
}
