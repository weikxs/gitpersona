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

func TestGitHubSignedInAccountsUsesTokenFreeStatus(t *testing.T) {
	var gotArgs []string
	gh := GitHub{GH: "gh", Run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(`{"hosts":{"github.com":[
			{"login":"octo-alex","state":"success","active":true},
			{"login":"acme-alex","state":"success","active":false},
			{"login":"expired-alex","state":"error","active":false}]}}`), nil
	}}

	accounts, err := gh.SignedInAccounts(context.Background(), "github.com")

	if err != nil {
		t.Fatalf("SignedInAccounts() error: %v", err)
	}
	if !reflect.DeepEqual(accounts, []string{"octo-alex", "acme-alex"}) {
		t.Errorf("SignedInAccounts() = %v, want only accounts in the success state", accounts)
	}
	wantArgs := []string{"auth", "status", "--hostname", "github.com", "--json", "hosts"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Errorf("ran gh %v, want gh %v", gotArgs, wantArgs)
	}
	for _, a := range gotArgs {
		if a == "--show-token" || a == "-t" || a == "token" {
			t.Fatalf("status query must never ask for a token, ran gh %v", gotArgs)
		}
	}
}

func TestGitHubSignedInAccountsErrors(t *testing.T) {
	failing := GitHub{GH: "gh", Run: func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("exit status 1")
	}}
	garbage := GitHub{GH: "gh", Run: func(context.Context, string, ...string) ([]byte, error) {
		return []byte("not json"), nil
	}}

	if _, err := failing.SignedInAccounts(context.Background(), "github.com"); err == nil {
		t.Error("SignedInAccounts() hid a gh failure")
	}
	if _, err := garbage.SignedInAccounts(context.Background(), "github.com"); err == nil {
		t.Error("SignedInAccounts() accepted output that is not JSON")
	}
}

func TestGitHubAccountsAndUser(t *testing.T) {
	gh := GitHub{GH: "gh", Run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "api" {
			return []byte(`{"login":"octo-alex","id":1000001,"name":"Alex Octo"}`), nil
		}
		return []byte(`{"hosts":{"github.com":[{"login":"acme-alex","state":"success","active":false},{"login":"octo-alex","state":"success","active":true}]}}`), nil
	}}

	accounts, err := gh.Accounts(context.Background(), "github.com")
	user, uerr := gh.User(context.Background(), "github.com", "octo-alex")

	if err != nil || len(accounts) != 2 || accounts[1].Login != "octo-alex" || !accounts[1].Active {
		t.Fatalf("Accounts() = %+v, %v", accounts, err)
	}
	if uerr != nil || user.ID != 1000001 || user.Name != "Alex Octo" {
		t.Fatalf("User() = %+v, %v", user, uerr)
	}
}
