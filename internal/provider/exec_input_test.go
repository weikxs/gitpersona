package provider

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestExecInputRunnerFeedsInputAndExtraEnvironmentToTheProgram(t *testing.T) {
	script := writeScript(t, "#!/bin/sh\ncat\necho \"env:$DEMO_TOKEN_VAR\"\necho noise >&2\n")

	out, err := ExecInputRunner(context.Background(), "secret-in\n", []string{"DEMO_TOKEN_VAR=demo-value"}, script)

	if err != nil || string(out) != "secret-in\nenv:demo-value\n" {
		t.Fatalf("ExecInputRunner() = %q, %v; want stdin echoed, env set and stderr dropped", out, err)
	}
}

func TestExecInputRunnerReportsAFailingProgramWithoutOutput(t *testing.T) {
	script := writeScript(t, "#!/bin/sh\necho partial-secret\nexit 3\n")

	out, err := ExecInputRunner(context.Background(), "", nil, script)

	if err == nil || out != nil {
		t.Fatalf("ExecInputRunner() = %q, %v; want an error and no output", out, err)
	}
}

func TestExecInputRunnerHonorsTimeout(t *testing.T) {
	script := writeScript(t, "#!/bin/sh\nexec sleep 5\n")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if _, err := ExecInputRunner(ctx, "", nil, script); err == nil {
		t.Fatal("ExecInputRunner() ignored the context deadline")
	}
}

func TestExecRunnerReportsAMissingProgram(t *testing.T) {
	if _, err := ExecRunner(context.Background(), "/nonexistent/demo-gh"); err == nil {
		t.Fatal("ExecRunner() of a missing program succeeded, want an error")
	}
}

func TestGitHubUserReportsFailureAndGarbage(t *testing.T) {
	failing := GitHub{GH: "gh", Run: func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("exit status 1")
	}}
	garbage := GitHub{GH: "gh", Run: func(context.Context, string, ...string) ([]byte, error) {
		return []byte("<html>"), nil
	}}

	_, failErr := failing.User(context.Background(), "github.com", "octo-alex")
	_, parseErr := garbage.User(context.Background(), "github.com", "octo-alex")

	if failErr == nil || !strings.Contains(failErr.Error(), "octo-alex") {
		t.Errorf("User() failure = %v, want an error naming the login", failErr)
	}
	if parseErr == nil {
		t.Error("User() accepted a response that is not JSON")
	}
}

func TestGitHubUserKeepsThePublicEmail(t *testing.T) {
	gh := GitHub{GH: "gh", Run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) < 4 || args[3] != "users/octo-alex" {
			t.Errorf("ran gh %v, want an api call for users/octo-alex", args)
		}
		return []byte(`{"id":7,"name":"Alex Octo","email":"alex@demo.example"}`), nil
	}}

	u, err := gh.User(context.Background(), "github.com", "octo-alex")

	if err != nil || u.Email != "alex@demo.example" {
		t.Fatalf("User() = %+v, %v", u, err)
	}
}
