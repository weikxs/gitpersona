package main

import (
	"bytes"
	"context"
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
	go func() { done <- runUI(ctx, app.Env{Stdout: out}, false, "setup") }()

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
