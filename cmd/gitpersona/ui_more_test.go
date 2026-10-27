package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/weikxs/gitpersona/internal/app"
)

const uiWait = 5 * time.Second

// fakeOpener puts a program named like the platform's browser opener first on
// PATH. It records its argument, so nothing real is ever opened.
func fakeOpener(t *testing.T) (record string) {
	t.Helper()
	bin := t.TempDir()
	record = filepath.Join(t.TempDir(), "opened")
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	script := "#!/bin/sh\necho \"$1\" > '" + record + "'\n"
	if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	return record
}

func TestOpenStartsTheSystemOpenerWithTheLink(t *testing.T) {
	record := fakeOpener(t)

	if err := open("http://127.0.0.1:1/?key=demo"); err != nil {
		t.Fatalf("open() error: %v", err)
	}

	deadline := time.Now().Add(uiWait)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(record); err == nil && strings.Contains(string(data), "key=demo") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the opener never received the link")
}

func TestOpenFailsWhenThereIsNoOpener(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if err := open("http://127.0.0.1:1/"); err == nil {
		t.Fatal("open() without an opener succeeded, want an error")
	}
}

func TestUITellsTheUserToOpenTheLinkWhenNoBrowserCanBeStarted(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	out := &lockedBuffer{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runUI(ctx, app.Env{Stdout: out, ConfigDir: t.TempDir()}, true, "") }()

	deadline := time.Now().Add(uiWait)
	for !strings.Contains(out.String(), "Could not open a browser") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("runUI() = %v", err)
	}
	if !strings.Contains(out.String(), "open the link above yourself") {
		t.Fatalf("output = %q, want the fallback instruction", out.String())
	}
}

func TestLockUIFailsWhenTheConfigDirectoryCannotBeCreated(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := lockUI(filepath.Join(blocker, "gitpersona")); err == nil {
		t.Fatal("lockUI() below a regular file succeeded, want an error")
	}
}

func TestLockUIFailsWhenTheLockCannotBeCreated(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if _, err := lockUI(dir); err == nil {
		t.Fatal("lockUI() in a read-only folder succeeded, want an error")
	}
}

func TestALockThatCannotBeReadIsReplaced(t *testing.T) {
	dir := t.TempDir()
	// An empty folder where the lock file should be: it exists but has no owner to read.
	if err := os.Mkdir(filepath.Join(dir, uiLockName), 0o700); err != nil {
		t.Fatal(err)
	}

	release, err := lockUI(dir)

	if err != nil {
		t.Fatalf("lockUI() = %v, want the unreadable lock replaced", err)
	}
	release()
}

func TestTheBrowserOpenerIsWaitedFor(t *testing.T) {
	cmd := exec.Command("true")

	reaped, err := startAndReap(cmd)

	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-reaped:
	case <-time.After(uiWait):
		t.Fatal("the opener was never waited for, so it stays a zombie")
	}
	if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
		t.Fatal("the opener has no exit status after the wait")
	}
}
