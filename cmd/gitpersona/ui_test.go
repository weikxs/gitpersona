package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/weikxs/gitpersona/internal/app"
)

// lockedBuffer lets the test read what the server goroutine prints.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestUILinkOpensTheChosenSection(t *testing.T) {
	out := &lockedBuffer{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runUI(ctx, app.Env{Stdout: out, ConfigDir: t.TempDir()}, false, "setup") }()

	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(out.String(), "#setup") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("runUI() = %v", err)
	}
	if !strings.Contains(out.String(), "http://127.0.0.1:") || !strings.Contains(out.String(), "#setup") {
		t.Fatalf("the link should open the setup steps:\n%s", out.String())
	}
}

func TestOnlyOnePageRunsAtATime(t *testing.T) {
	dir := t.TempDir()
	release, err := lockUI(dir)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := lockUI(dir); err == nil || !strings.Contains(err.Error(), "already open") {
		t.Fatalf("a second page = %v, want it refused", err)
	}
	err = runUI(context.Background(), app.Env{Stdout: &lockedBuffer{}, ConfigDir: dir}, false, "")
	if err == nil || !strings.Contains(err.Error(), "already open") {
		t.Fatalf("runUI() while a page runs = %v", err)
	}

	release()
	again, err := lockUI(dir)
	if err != nil {
		t.Fatalf("after the first page stopped: %v", err)
	}
	again()
}

func TestAStaleLockIsTakenOver(t *testing.T) {
	dir := t.TempDir()
	// A process number that cannot exist: the page that wrote it is gone.
	if err := os.WriteFile(filepath.Join(dir, uiLockName), []byte("2147483646\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	release, err := lockUI(dir)
	if err != nil {
		t.Fatalf("a lock left by a stopped page should be taken over: %v", err)
	}
	release()
	if _, err := os.Stat(filepath.Join(dir, uiLockName)); !os.IsNotExist(err) {
		t.Error("releasing should remove the lock")
	}
}
