package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestGitHubTokenRunsGhAuthToken(t *testing.T) {
	var gotName string
	var gotArgs []string
	gh := GitHub{GH: "/opt/tools/gh", Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		gotName, gotArgs = name, args
		return []byte("tok\n"), nil
	}}

	token, err := gh.Token(context.Background(), "github.com", "acme-alex")

	if err != nil || token != "tok\n" {
		t.Fatalf("Token() = %q, %v", token, err)
	}
	wantArgs := []string{"auth", "token", "--hostname", "github.com", "--user", "acme-alex"}
	if gotName != "/opt/tools/gh" || !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("ran %s %v, want /opt/tools/gh %v", gotName, gotArgs, wantArgs)
	}
}

func TestGitHubTokenReportsFailureWithoutOutput(t *testing.T) {
	gh := GitHub{GH: "gh", Run: func(context.Context, string, ...string) ([]byte, error) {
		return []byte("partial-secret"), errors.New("exit status 1")
	}}

	_, err := gh.Token(context.Background(), "github.com", "acme-alex")

	if err == nil || strings.Contains(err.Error(), "partial-secret") {
		t.Fatalf("Token() error = %v, want an error that does not echo command output", err)
	}
}

func TestExecRunnerReturnsStdoutOnly(t *testing.T) {
	script := writeScript(t, "#!/bin/sh\necho \"args:$*\"\necho noise >&2\n")

	out, err := ExecRunner(context.Background(), script, "auth", "token")

	if err != nil || string(out) != "args:auth token\n" {
		t.Fatalf("ExecRunner() = %q, %v", out, err)
	}
}

func TestExecRunnerHonorsTimeout(t *testing.T) {
	script := writeScript(t, "#!/bin/sh\nexec sleep 5\n")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if _, err := ExecRunner(ctx, script); err == nil {
		t.Fatal("ExecRunner() ignored the context deadline")
	}
}

func writeScript(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-gh")
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
