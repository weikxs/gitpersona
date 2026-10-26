package provider

import (
	"context"
	"errors"
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

func TestCachingRunnerRemembersFailuresButNotCancellations(t *testing.T) {
	next, now := &countingRunner{err: errors.New("exit status 1")}, &clock{now: time.Unix(0, 0)}
	cache := newCache(next.run, now)

	for range 3 {
		if _, err := cache.Run(context.Background(), "/usr/bin/gh", statusArgs...); err == nil {
			t.Fatal("Run() succeeded, want the failure")
		}
	}
	if got := next.count("/usr/bin/gh", statusArgs...); got != 1 {
		t.Fatalf("a failing gh auth status ran %d times, want 1", got)
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
