package provider

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestGitHubUserReportsAFailingOrGarbledAnswer(t *testing.T) {
	failing := GitHub{GH: "gh", Run: func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("exit status 1")
	}}
	garbled := GitHub{GH: "gh", Run: func(context.Context, string, ...string) ([]byte, error) {
		return []byte("<html>"), nil
	}}

	_, failErr := failing.User(context.Background(), "github.com", "octo-alex")
	_, parseErr := garbled.User(context.Background(), "github.com", "octo-alex")

	if failErr == nil || !strings.Contains(failErr.Error(), "users/octo-alex") {
		t.Errorf("User() on failure = %v, want an error naming the request", failErr)
	}
	if parseErr == nil || !strings.Contains(parseErr.Error(), "octo-alex") {
		t.Errorf("User() on garbage = %v, want a parse error naming the login", parseErr)
	}
}

func TestExecInputRunnerFeedsInputAndExtraEnvironmentAndReturnsStdoutOnly(t *testing.T) {
	script := writeScript(t, "#!/bin/sh\nread line\necho \"$line|$DEMO_EXTRA|$*\"\necho noise >&2\n")

	out, err := ExecInputRunner(context.Background(), "typed\n", []string{"DEMO_EXTRA=on"}, script, "a", "b")

	if err != nil || string(out) != "typed|on|a b\n" {
		t.Fatalf("ExecInputRunner() = %q, %v", out, err)
	}
}

func TestExecInputRunnerReportsAFailingProgramWithoutOutput(t *testing.T) {
	script := writeScript(t, "#!/bin/sh\necho partial\nexit 3\n")

	out, err := ExecInputRunner(context.Background(), "", nil, script)

	if err == nil || out != nil {
		t.Fatalf("ExecInputRunner() = %q, %v; want an error and no output", out, err)
	}
}
