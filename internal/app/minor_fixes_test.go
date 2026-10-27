package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/provider"
)

func TestTheShimsWaitForATokenOnlySoLong(t *testing.T) {
	_, env, _, _ := glabWorld(t)
	hasDeadline := false
	inner := env.runInput()
	env.RunInput = func(ctx context.Context, input string, environ []string, name string, args ...string) ([]byte, error) {
		_, hasDeadline = ctx.Deadline()
		return inner(ctx, input, environ, name, args...)
	}

	if code := GlabShim(context.Background(), env, []string{"mr", "list"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	if !hasDeadline {
		t.Error("git credential fill for glab ran without a time limit")
	}

	w := newWorld(t)
	ghEnv, _, _ := shimEnv(t, w, w.repos["sdk"])
	ghDeadline := false
	ghEnv.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if len(args) > 1 && args[0] == "auth" && args[1] == "token" {
			_, ghDeadline = ctx.Deadline()
		}
		return env.Run(ctx, name, args...)
	}
	if code := GHShim(context.Background(), ghEnv, []string{"pr", "list"}); code != 0 {
		t.Fatalf("gh shim code %d", code)
	}
	if !ghDeadline {
		t.Error("gh auth token for the gh shim ran without a time limit")
	}
}

func TestWhoamiSaysWhenGitCannotReadTheRepositorysSettings(t *testing.T) {
	w := newWorld(t)
	if err := Apply(context.Background(), mustEnv(t, w, w.home), ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	env, out := w.env(t, w.repos["blog"])
	// git config --list fails (it timed out, or an included file is
	// unreadable) while the repository itself is found.
	env.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if slices.Contains(args, "--list") {
			return nil, errors.New("exit status 128")
		}
		return provider.ExecRunner(ctx, name, args...)
	}

	err := Whoami(context.Background(), env, false)

	report, _ := WhoamiReportFor(context.Background(), env)
	if err != nil || report.ConfigError == "" || report.Consistent || !report.InRepository {
		t.Fatalf("Whoami() = %v, report %+v; want the read error reported", err, report)
	}
	if text := out.String(); !strings.Contains(text, "author  unknown") || !strings.Contains(text, "could not read its settings") {
		t.Errorf("whoami should say the settings could not be read, not show an author:\n%s", text)
	}
}

func mustEnv(t *testing.T, w world, dir string) Env {
	t.Helper()
	env, _ := w.env(t, dir)
	return env
}
