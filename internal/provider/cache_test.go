package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// countingRunner answers every command and counts how often each one ran.
type countingRunner struct {
	mu    sync.Mutex
	calls map[string]int
	err   error
}

func (c *countingRunner) run(_ context.Context, name string, args ...string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.calls == nil {
		c.calls = map[string]int{}
	}
	c.calls[cacheKey(name, args)]++
	if c.err != nil {
		return nil, c.err
	}
	return []byte("answer"), nil
}

func (c *countingRunner) count(name string, args ...string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[cacheKey(name, args)]
}

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func newCache(next Runner, c *clock) *CachingRunner {
	return &CachingRunner{Next: next, TTL: 10 * time.Minute, Now: c.Now}
}

var statusArgs = []string{"auth", "status", "--hostname", "github.com", "--json", "hosts"}

func TestCachingRunnerAsksGhAuthStatusOnceWithinTheTTL(t *testing.T) {
	next, now := &countingRunner{}, &clock{now: time.Unix(0, 0)}
	cache := newCache(next.run, now)

	for range 5 {
		out, err := cache.Run(context.Background(), "/usr/bin/gh", statusArgs...)
		if err != nil || string(out) != "answer" {
			t.Fatalf("Run() = %q, %v", out, err)
		}
	}

	if got := next.count("/usr/bin/gh", statusArgs...); got != 1 {
		t.Fatalf("gh auth status ran %d times, want 1", got)
	}
}

func TestCachingRunnerAsksAgainAfterTheTTL(t *testing.T) {
	next, now := &countingRunner{}, &clock{now: time.Unix(0, 0)}
	cache := newCache(next.run, now)

	_, _ = cache.Run(context.Background(), "/usr/bin/gh", statusArgs...)
	now.now = now.now.Add(10*time.Minute + time.Second)
	_, _ = cache.Run(context.Background(), "/usr/bin/gh", statusArgs...)

	if got := next.count("/usr/bin/gh", statusArgs...); got != 2 {
		t.Fatalf("gh auth status ran %d times, want 2", got)
	}
}

func TestCachingRunnerForgetAsksAgain(t *testing.T) {
	next, now := &countingRunner{}, &clock{now: time.Unix(0, 0)}
	cache := newCache(next.run, now)

	_, _ = cache.Run(context.Background(), "/usr/bin/gh", statusArgs...)
	cache.Forget()
	_, _ = cache.Run(context.Background(), "/usr/bin/gh", statusArgs...)

	if got := next.count("/usr/bin/gh", statusArgs...); got != 2 {
		t.Fatalf("gh auth status ran %d times, want 2", got)
	}
}

func TestCachingRunnerRemembersGhAPIUserLookups(t *testing.T) {
	next, now := &countingRunner{}, &clock{now: time.Unix(0, 0)}
	cache := newCache(next.run, now)
	args := []string{"api", "--hostname", "github.com", "users/acme-alex"}

	_, _ = cache.Run(context.Background(), "/usr/bin/gh", args...)
	_, _ = cache.Run(context.Background(), "/usr/bin/gh", args...)

	if got := next.count("/usr/bin/gh", args...); got != 1 {
		t.Fatalf("gh api ran %d times, want 1", got)
	}
}

func TestCachingRunnerNeverRemembersTokensOrOtherCommands(t *testing.T) {
	next, now := &countingRunner{}, &clock{now: time.Unix(0, 0)}
	cache := newCache(next.run, now)
	token := []string{"auth", "token", "--hostname", "github.com", "--user", "acme-alex"}
	git := []string{"config", "--get", "user.email"}

	for range 3 {
		_, _ = cache.Run(context.Background(), "/usr/bin/gh", token...)
		_, _ = cache.Run(context.Background(), "/usr/bin/git", git...)
	}

	if got := next.count("/usr/bin/gh", token...); got != 3 {
		t.Fatalf("gh auth token ran %d times, want 3 (never cached)", got)
	}
	if got := next.count("/usr/bin/git", git...); got != 3 {
		t.Fatalf("git config ran %d times, want 3 (never cached)", got)
	}
}

func TestCachingRunnerNeverRemembersFailuresOrCancellations(t *testing.T) {
	next, now := &countingRunner{err: errors.New("exit status 1")}, &clock{now: time.Unix(0, 0)}
	cache := newCache(next.run, now)

	for range 3 {
		if _, err := cache.Run(context.Background(), "/usr/bin/gh", statusArgs...); err == nil {
			t.Fatal("Run() succeeded, want the failure")
		}
	}
	// A failure (gh signed out for a moment, no network) must not stick for
	// the whole TTL: the next question asks again.
	if got := next.count("/usr/bin/gh", statusArgs...); got != 3 {
		t.Fatalf("a failing gh auth status ran %d times, want 3", got)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	other := []string{"auth", "status", "--hostname", "ghe.example.com", "--json", "hosts"}
	_, _ = cache.Run(cancelled, "/usr/bin/gh", other...)
	_, _ = cache.Run(context.Background(), "/usr/bin/gh", other...)
	if got := next.count("/usr/bin/gh", other...); got != 2 {
		t.Fatalf("after a cancelled call gh auth status ran %d times, want 2", got)
	}
}

func TestCachingRunnerRemembersASuccessAfterAFailure(t *testing.T) {
	next, now := &countingRunner{err: errors.New("exit status 1")}, &clock{now: time.Unix(0, 0)}
	cache := newCache(next.run, now)
	_, _ = cache.Run(context.Background(), "/usr/bin/gh", statusArgs...)

	next.mu.Lock()
	next.err = nil
	next.mu.Unlock()
	for range 2 {
		if out, err := cache.Run(context.Background(), "/usr/bin/gh", statusArgs...); err != nil || string(out) != "answer" {
			t.Fatalf("Run() = %q, %v", out, err)
		}
	}
	if got := next.count("/usr/bin/gh", statusArgs...); got != 2 {
		t.Fatalf("gh auth status ran %d times, want 2 (the failure, then one success)", got)
	}
}

func TestCachingRunnerSharesOneCallBetweenConcurrentCallers(t *testing.T) {
	release := make(chan struct{})
	next := &countingRunner{}
	slow := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		<-release
		return next.run(ctx, name, args...)
	}
	cache := newCache(slow, &clock{now: time.Unix(0, 0)})

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { _, _ = cache.Run(context.Background(), "/usr/bin/gh", statusArgs...) })
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := next.count("/usr/bin/gh", statusArgs...); got != 1 {
		t.Fatalf("concurrent callers ran gh auth status %d times, want 1", got)
	}
}

// diskCache returns a runner keeping its answers in dir, as a separate
// gitpersona process would, with gh's account file fingerprinted by stamp.
func diskCache(next Runner, now *clock, dir string, stamp *string) *CachingRunner {
	return &CachingRunner{Next: next, TTL: 10 * time.Minute, Now: now.Now, Dir: dir, Stamp: func() string { return *stamp }}
}

func TestCachingRunnerSharesAnswersBetweenProcessesThroughItsDirectory(t *testing.T) {
	next, now, dir, stamp := &countingRunner{}, &clock{now: time.Unix(1000, 0)}, t.TempDir(), "hosts-v1"

	first, err := diskCache(next.run, now, dir, &stamp).Run(context.Background(), "/usr/bin/gh", statusArgs...)
	now.now = now.now.Add(5 * time.Minute)
	second, err2 := diskCache(next.run, now, dir, &stamp).Run(context.Background(), "/usr/bin/gh", statusArgs...)

	if err != nil || err2 != nil || string(first) != "answer" || string(second) != "answer" {
		t.Fatalf("answers = %q, %v / %q, %v", first, err, second, err2)
	}
	if got := next.count("/usr/bin/gh", statusArgs...); got != 1 {
		t.Fatalf("two processes ran gh auth status %d times, want 1", got)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*"))
	for _, f := range files {
		if info, err := os.Stat(f); err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("%s: mode %v, %v; want 0600", f, info.Mode().Perm(), err)
		}
	}
}

func TestCachingRunnerAsksAgainWhenGhsAccountsChangeOrTheAnswerIsOld(t *testing.T) {
	next, now, dir, stamp := &countingRunner{}, &clock{now: time.Unix(1000, 0)}, t.TempDir(), "hosts-v1"
	_, _ = diskCache(next.run, now, dir, &stamp).Run(context.Background(), "/usr/bin/gh", statusArgs...)

	stamp = "hosts-v2" // gh auth login, logout or switch rewrote gh's account file
	_, _ = diskCache(next.run, now, dir, &stamp).Run(context.Background(), "/usr/bin/gh", statusArgs...)
	now.now = now.now.Add(11 * time.Minute)
	_, _ = diskCache(next.run, now, dir, &stamp).Run(context.Background(), "/usr/bin/gh", statusArgs...)
	now.now = now.now.Add(-time.Hour) // the clock went back: an answer from the future is not trusted
	_, _ = diskCache(next.run, now, dir, &stamp).Run(context.Background(), "/usr/bin/gh", statusArgs...)

	if got := next.count("/usr/bin/gh", statusArgs...); got != 4 {
		t.Fatalf("gh auth status ran %d times, want 4", got)
	}
}

func TestCachingRunnerKeepsNoFailuresOrTokensOnDisk(t *testing.T) {
	dir, stamp, now := t.TempDir(), "hosts-v1", &clock{now: time.Unix(1000, 0)}
	failing := &countingRunner{err: errors.New("exit status 1")}
	_, _ = diskCache(failing.run, now, dir, &stamp).Run(context.Background(), "/usr/bin/gh", statusArgs...)
	tokens := &countingRunner{}
	_, _ = diskCache(tokens.run, now, dir, &stamp).Run(context.Background(), "/usr/bin/gh", "auth", "token", "--user", "acme-alex")

	if files, _ := filepath.Glob(filepath.Join(dir, "*")); len(files) != 0 {
		t.Fatalf("files on disk after a failure and a token lookup: %v", files)
	}
}

func TestCachingRunnerForgetAlsoClearsTheDirectory(t *testing.T) {
	next, now, dir, stamp := &countingRunner{}, &clock{now: time.Unix(1000, 0)}, t.TempDir(), "hosts-v1"
	cache := diskCache(next.run, now, dir, &stamp)
	_, _ = cache.Run(context.Background(), "/usr/bin/gh", statusArgs...)

	cache.Forget()
	_, _ = diskCache(next.run, now, dir, &stamp).Run(context.Background(), "/usr/bin/gh", statusArgs...)

	if got := next.count("/usr/bin/gh", statusArgs...); got != 2 {
		t.Fatalf("gh auth status ran %d times after Forget, want 2", got)
	}
}
