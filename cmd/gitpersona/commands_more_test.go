package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/app"
)

func runCommand(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = run(args, strings.NewReader(""), &out, &errOut)
	return code, out.String(), errOut.String()
}

// holdUILock makes every page the test starts fail at once, instead of
// serving until it is idle.
func holdUILock(t *testing.T, home string) {
	t.Helper()
	release, err := lockUI(filepath.Join(home, ".config", "gitpersona"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
}

func TestUICommandReportsThatAPageIsAlreadyOpen(t *testing.T) {
	holdUILock(t, demoHome(t))

	code, _, stderr := runCommand("ui", "--no-open")

	if code != 1 || !strings.Contains(stderr, "already open") {
		t.Fatalf("ui: exit %d, stderr %q; want a refusal because a page is open", code, stderr)
	}
}

func TestSetupWebOpensOnTheOverviewWhenAConfigurationExists(t *testing.T) {
	holdUILock(t, demoHome(t))

	code, stdout, stderr := runCommand("setup", "--web", "--no-open")

	if code != 1 || !strings.Contains(stderr, "already open") {
		t.Fatalf("setup --web: exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "You already have a configuration") {
		t.Errorf("stdout = %q, want the note that the page opens on the overview", stdout)
	}
}

func TestSetupWebOpensOnSetupWhenThereIsNoConfigurationYet(t *testing.T) {
	home := demoHome(t)
	if err := os.Remove(filepath.Join(home, ".config", "gitpersona", "config.toml")); err != nil {
		t.Fatal(err)
	}
	holdUILock(t, home)

	code, stdout, _ := runCommand("setup", "--web", "--no-open")

	if code != 1 || strings.Contains(stdout, "You already have a configuration") {
		t.Fatalf("setup --web: exit %d, stdout %q; want no note about an existing configuration", code, stdout)
	}
}

func TestAuthCommandRoutesItsSubcommands(t *testing.T) {
	demoHome(t)
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStderr string
	}{
		{"login for an unknown profile", []string{"auth", "login", "--profile", "ghost"}, 1, "ghost"},
		{"logout for an unknown profile", []string{"auth", "logout", "--profile", "ghost"}, 1, "ghost"},
		{"status for an unknown profile", []string{"auth", "status", "--profile", "ghost"}, 1, "ghost"},
		{"unknown subcommand", []string{"auth", "bogus"}, 2, "login, logout or status"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, _, stderr := runCommand(tc.args...)

			if code != tc.wantCode || !strings.Contains(stderr, tc.wantStderr) {
				t.Fatalf("exit %d, stderr %q; want exit %d mentioning %q", code, stderr, tc.wantCode, tc.wantStderr)
			}
		})
	}
}

func TestAuthWithoutASubcommandShowsStatus(t *testing.T) {
	demoHome(t)

	code, stdout, stderr := runCommand("auth")

	if code != 0 {
		t.Fatalf("auth: exit %d, stdout %q, stderr %q; want it to show the status", code, stdout, stderr)
	}
}

func TestCheckSSHRunsWithoutKeysToCheck(t *testing.T) {
	demoHome(t)

	code, stdout, stderr := runCommand("check-ssh")

	if code != 0 {
		t.Fatalf("check-ssh: exit %d, stdout %q, stderr %q; want success when no profile has a key", code, stdout, stderr)
	}
}

func TestHelpWithoutACommandPrintsTheUsage(t *testing.T) {
	code, stdout, _ := runCommand("help")

	if code != 0 || !strings.Contains(stdout, "Usage:") {
		t.Fatalf("help: exit %d, stdout %q", code, stdout)
	}
}

func TestRunShimPassesTheEnvironmentToTheShim(t *testing.T) {
	home := demoHome(t)
	var gotArgs []string
	var gotHome string
	shim := func(_ context.Context, env app.Env, args []string) int {
		gotArgs, gotHome = args, env.Home
		return 7
	}

	code := runShim(shim, []string{"auth", "status"})

	if code != 7 || strings.Join(gotArgs, " ") != "auth status" || gotHome != home {
		t.Fatalf("runShim() = %d, args %v, home %q; want the shim's code with the arguments and the home", code, gotArgs, gotHome)
	}
}

func TestRunShimFailsWhenTheEnvironmentCannotBeRead(t *testing.T) {
	t.Setenv("HOME", "")
	called := false
	shim := func(context.Context, app.Env, []string) int { called = true; return 0 }

	code := runShim(shim, nil)

	if code != exitFailure || called {
		t.Fatalf("runShim() = %d (shim called: %v), want a failure without running the shim", code, called)
	}
}
