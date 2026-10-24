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

func TestRunShimPassesItsArgumentsToTheShimAndReturnsItsExitCode(t *testing.T) {
	demoHome(t)
	var gotArgs []string
	shim := func(_ context.Context, env app.Env, args []string) int {
		gotArgs = args
		if env.Home == "" {
			t.Error("the shim got an environment without a home")
		}
		return 7
	}

	code := runShim(shim, []string{"pr", "list"})

	if code != 7 || strings.Join(gotArgs, " ") != "pr list" {
		t.Fatalf("runShim() = %d with args %v, want 7 and the arguments untouched", code, gotArgs)
	}
}

func TestRunShimFailsWithoutAHomeAndNeverCallsTheShim(t *testing.T) {
	t.Setenv("HOME", "")
	called := false
	shim := func(context.Context, app.Env, []string) int { called = true; return 0 }

	code := runShim(shim, nil)

	if code != 1 || called {
		t.Fatalf("runShim() = %d (shim called: %v), want 1 and no call", code, called)
	}
}

func TestHelpWithoutAnArgumentPrintsTheUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := help(nil, &stdout, &stderr)

	if code != 0 || !strings.Contains(stdout.String(), "Usage:") || stderr.Len() != 0 {
		t.Fatalf("help() = %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

func TestAuthRefusesAnUnknownSubcommand(t *testing.T) {
	demoHome(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"auth", "bogus"}, strings.NewReader(""), &stdout, &stderr)

	if code != 2 || !strings.Contains(stderr.String(), "auth takes login, logout or status") {
		t.Fatalf("auth bogus: exit %d, stderr %q; want a usage error", code, stderr.String())
	}
}

func TestAuthLoginAndLogoutFailForAGitHubProfile(t *testing.T) {
	demoHome(t)
	for _, sub := range []string{"login", "logout"} {
		var stdout, stderr bytes.Buffer

		code := run([]string{"auth", sub, "--profile", "acme"}, strings.NewReader("demo-token\n"), &stdout, &stderr)

		if code != 1 || !strings.Contains(stderr.String(), "is on GitHub") {
			t.Errorf("auth %s on a GitHub profile: exit %d, stderr %q", sub, code, stderr.String())
		}
		if strings.Contains(stdout.String()+stderr.String(), "demo-token") {
			t.Errorf("auth %s echoed the token", sub)
		}
	}
}

func TestAuthStatusIsTheDefaultSubcommand(t *testing.T) {
	demoHome(t)
	var withWord, withoutWord, stderr bytes.Buffer

	codeWord := run([]string{"auth", "status"}, strings.NewReader(""), &withWord, &stderr)
	codeNone := run([]string{"auth"}, strings.NewReader(""), &withoutWord, &stderr)

	if codeWord != codeNone || withWord.String() != withoutWord.String() {
		t.Fatalf("auth status (%d, %q) differs from auth (%d, %q)", codeWord, withWord.String(), codeNone, withoutWord.String())
	}
}

func TestCheckSSHFailsWithoutAConfiguration(t *testing.T) {
	home := demoHome(t)
	if err := os.Remove(filepath.Join(home, ".config", "gitpersona", "config.toml")); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer

	code := run([]string{"check-ssh"}, strings.NewReader(""), &stdout, &stderr)

	if code != 1 || !strings.Contains(stderr.String(), "no configuration yet") {
		t.Fatalf("check-ssh without config: exit %d, stderr %q", code, stderr.String())
	}
}

func TestUIFailsWhenItsLockFolderCannotBeCreated(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	err := runUI(context.Background(), app.Env{Stdout: &lockedBuffer{}, ConfigDir: filepath.Join(blocker, "gitpersona")}, false, "")

	if err == nil {
		t.Fatal("runUI() under a regular file succeeded, want an error")
	}
}

func TestLockOwnerReadsTheProcessNumberOfALock(t *testing.T) {
	dir := t.TempDir()
	for name, tc := range map[string]struct {
		content string
		want    int
	}{
		"a process number": {"4242\n", 4242},
		"garbage":          {"not a number", 0},
		"empty":            {"", 0},
	} {
		path := filepath.Join(dir, strings.ReplaceAll(name, " ", "-"))
		if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := lockOwner(path); got != tc.want {
			t.Errorf("lockOwner(%s) = %d, want %d", name, got, tc.want)
		}
	}
	if got := lockOwner(filepath.Join(dir, "absent")); got != 0 {
		t.Errorf("lockOwner(absent) = %d, want 0", got)
	}
}

func TestProcessRunsKnowsOurOwnProcessAndNotAnImpossibleOne(t *testing.T) {
	if !processRuns(os.Getpid()) {
		t.Error("processRuns(own pid) = false")
	}
	if processRuns(2147483646) {
		t.Error("processRuns(impossible pid) = true")
	}
}

func TestALockHeldByALiveProcessWithoutAReadableNumberIsTakenOver(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, uiLockName), []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}

	release, err := lockUI(dir)

	if err != nil {
		t.Fatalf("lockUI() = %v, want a lock with no process number to count as stale", err)
	}
	release()
}
