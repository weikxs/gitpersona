package main

import (
	"context"
	"testing"
	"time"

	"github.com/weikxs/gitpersona/internal/app"
)

func TestAnInterruptWaitsForFilesBeingWritten(t *testing.T) {
	guard := &app.WriteGuard{}
	release := guard.Writing() // a command in the middle of writing files
	ctx, cancel := context.WithCancel(context.Background())
	exited := make(chan int, 1)
	go exitWhenInterrupted(ctx, make(chan struct{}), guard, time.Millisecond, func(code int) { exited <- code })

	cancel() // Ctrl-C

	select {
	case code := <-exited:
		t.Fatalf("exited with %d while files were being written", code)
	case <-time.After(200 * time.Millisecond):
	}
	release()
	select {
	case code := <-exited:
		if code != exitInterrupted {
			t.Fatalf("exit code %d, want %d", code, exitInterrupted)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not exit after the writes ended")
	}
}

func TestACommandThatEndsDoesNotExit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	exited := make(chan int, 1)
	go exitWhenInterrupted(ctx, finished, nil, time.Hour, func(code int) { exited <- code })

	cancel()
	close(finished)

	select {
	case code := <-exited:
		t.Fatalf("exited with %d although the command ended by itself", code)
	case <-time.After(100 * time.Millisecond):
	}
}
