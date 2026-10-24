package shell

import (
	"context"
	"errors"
	"strconv"
	"testing"
)

func TestCurrentShellIsUnknownWhenPsAnswersBadly(t *testing.T) {
	tests := []struct {
		name   string
		answer func() ([]byte, error)
	}{
		{"ps fails", func() ([]byte, error) { return nil, errors.New("exit status 1") }},
		{"ps prints one field", func() ([]byte, error) { return []byte("42\n"), nil }},
		{"parent is not a number", func() ([]byte, error) { return []byte("abc zsh\n"), nil }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			run := func(context.Context, string, ...string) ([]byte, error) { return tc.answer() }

			if got := CurrentShell(context.Background(), 400, run); got.Kind != Unknown {
				t.Fatalf("CurrentShell() = %+v, want unknown", got)
			}
		})
	}
}

func TestCurrentShellGivesUpAfterTooManyNonShellAncestors(t *testing.T) {
	calls := 0
	// Every process has a parent that is not a shell, one pid further up.
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		calls++
		pid, _ := strconv.Atoi(args[len(args)-1])
		return []byte(strconv.Itoa(pid+1) + " /usr/bin/make\n"), nil
	}

	got := CurrentShell(context.Background(), 100, run)

	if got.Kind != Unknown {
		t.Fatalf("CurrentShell() = %+v, want unknown", got)
	}
	if want := 2 * maxAncestors; calls != want {
		t.Errorf("ps ran %d times, want %d: the walk must stop at maxAncestors", calls, want)
	}
}
