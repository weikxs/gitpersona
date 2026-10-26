package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/weikxs/gitpersona/internal/app"
)

const (
	// exitInterrupted is the status of a command stopped by Ctrl-C (128 + SIGINT).
	exitInterrupted = 130
	// interruptGrace is how long an interrupted command may take to stop by
	// itself before gitpersona exits.
	interruptGrace = 2 * time.Second
)

// interruptible returns a context that ends on Ctrl-C or SIGTERM, so commands
// stop what they are doing, and a function to call when the command
// returns. A command that does not stop within interruptGrace (one waiting
// for input, say) is ended, but never while it writes files: those are
// finished first, so a change is never left half done.
func interruptible(guard *app.WriteGuard) (context.Context, func()) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	finished := make(chan struct{})
	go exitWhenInterrupted(ctx, finished, guard, interruptGrace, os.Exit)
	return ctx, func() {
		close(finished)
		stop()
	}
}

func exitWhenInterrupted(ctx context.Context, finished <-chan struct{}, guard *app.WriteGuard,
	grace time.Duration, exit func(int)) {
	select {
	case <-finished:
		return
	case <-ctx.Done():
	}
	select {
	case <-finished: // the command ended first; ctx ended with it
		return
	case <-time.After(grace):
	}
	if guard != nil {
		guard.Hold()
	}
	exit(exitInterrupted)
}
