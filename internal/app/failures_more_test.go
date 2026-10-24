package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/config"
)

// commandsNeedingAConfig runs every command that reads config.toml.
func commandsNeedingAConfig(t *testing.T, env Env) map[string]error {
	t.Helper()
	ctx := context.Background()
	str := func(s string) *string { return &s }
	return map[string]error{
		"apply":          Apply(ctx, env, ApplyOptions{}),
		"use":            Use(ctx, env, "acme"),
		"remove":         Remove(ctx, env, ""),
		"pin":            Pin(ctx, env, "acme"),
		"unpin":          Unpin(ctx, env),
		"add profile":    AddProfile(ctx, env, config.Profile{ID: "demo", Account: "octo-alex", Name: "A", Email: "a@demo.example"}),
		"update profile": UpdateProfile(ctx, env, "acme", ProfileChanges{Name: str("B")}),
		"remove profile": RemoveProfile(ctx, env, "acme"),
		"set default":    SetDefaultProfile(ctx, env, "acme"),
		"auth status":    AuthStatus(ctx, env, AuthOptions{}),
		"auth login":     AuthLogin(ctx, env, AuthOptions{Profile: "globex"}),
		"auth logout":    AuthLogout(ctx, env, AuthOptions{Profile: "globex"}),
		"check ssh":      CheckSSH(ctx, env, CheckSSHOptions{}),
		"old rules":      RemoveOldRules(ctx, env),
		"list":           List(ctx, env, false),
		"verify":         Verify(ctx, env, VerifyOptions{}),
	}
}

func TestEveryCommandThatReadsTheConfigurationSaysHowToCreateItWhenItIsMissing(t *testing.T) {
	w := newWorld(t)
	if err := os.Remove(filepath.Join(w.home, ".config", "gitpersona", "config.toml")); err != nil {
		t.Fatal(err)
	}
	env, _ := w.env(t, w.home)

	for name, err := range commandsNeedingAConfig(t, env) {
		var missing noConfigError
		if !errors.As(err, &missing) || !strings.Contains(err.Error(), "gitpersona setup") {
			t.Errorf("%s without a configuration = %v, want the hint to run setup", name, err)
		}
	}
}

func TestEveryCommandThatReadsTheConfigurationShowsTheHomeAsTildeWhenItIsBroken(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"), "version = [\n", 0o600)
	env, _ := w.env(t, w.home)

	for name, err := range commandsNeedingAConfig(t, env) {
		if err == nil || strings.Contains(err.Error(), w.home) || !strings.Contains(err.Error(), "~/.config/gitpersona/config.toml") {
			t.Errorf("%s with a broken configuration = %v, want the file shown as ~/...", name, err)
		}
	}
}

func TestCommandsRefuseABrokenGlobalGitConfigInsteadOfActingOnAnEmptyAnswer(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[user\n\tname = broken\n", 0o600)
	env, _ := w.env(t, w.home)
	ctx := context.Background()
	appendProfileDir := func() error { return Use(ctx, env, "personal") }

	cases := map[string]error{
		"apply": Apply(ctx, env, ApplyOptions{}),
		"use":   appendProfileDir(),
		"auth":  AuthLogin(ctx, env, AuthOptions{Profile: "globex"}),
	}
	for name, err := range cases {
		if err == nil {
			t.Errorf("%s with an unparsable ~/.gitconfig succeeded, want an error", name)
		}
	}
}

func TestRollbackAndUninstallReportAnUnusableBackupFolder(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "backups"), "not a folder", 0o600)

	if err := Rollback(context.Background(), env, "", false); err == nil || !strings.Contains(err.Error(), "list backups") {
		t.Errorf("Rollback with a file in place of the backup folder = %v", err)
	}
	if err := Rollback(context.Background(), env, "latest", true); err == nil {
		t.Error("restoring from an unusable backup folder succeeded")
	}
}

func TestRollbackSaysThereAreNoBackupsYet(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.home)

	if err := Rollback(context.Background(), env, "latest", true); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "No backups yet.") {
		t.Errorf("output = %q", out.String())
	}
}

func TestRollbackToAnUnknownSnapshotNamesTheCommandThatListsThem(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	err := Rollback(context.Background(), env, "no-such-snapshot", true)

	if err == nil || !strings.Contains(err.Error(), "gitpersona rollback") {
		t.Errorf("Rollback(unknown) = %v", err)
	}
}

func TestUninstallAndRulesCommandsReportAnUnreadableGitConfig(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("file permissions do not stop root")
	}
	w := newWorld(t)
	env, _ := w.env(t, w.home)
	ctx := context.Background()
	if err := Apply(ctx, env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	gitconfig := filepath.Join(w.home, ".gitconfig")
	if err := os.Chmod(gitconfig, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(gitconfig, 0o600) })

	for name, err := range map[string]error{
		"uninstall": Uninstall(ctx, env, true),
		"old rules": RemoveOldRules(ctx, env),
		"rollback":  Rollback(ctx, env, "latest", true),
		"apply":     Apply(ctx, env, ApplyOptions{}),
	} {
		if err == nil {
			t.Errorf("%s with an unreadable ~/.gitconfig succeeded, want an error", name)
		}
	}
}

func TestAuthLoginAsksForTheTokenAndNamesWhatWentWrongWithTheHelper(t *testing.T) {
	w := newWorld(t)
	fakeKeychain(t, w)
	env, _ := w.env(t, w.home)
	ctx := context.Background()

	if err := AuthLogin(ctx, env, AuthOptions{Profile: "globex"}); err == nil || !strings.Contains(err.Error(), "no token given") {
		t.Errorf("AuthLogin without any input = %v", err)
	}
	failing := func(context.Context, string, []string, string, ...string) ([]byte, error) {
		return nil, errors.New("helper crashed")
	}
	env.RunInput = failing
	env.Stdin = strings.NewReader("demo-token\n")
	if err := AuthLogin(ctx, env, AuthOptions{Profile: "globex"}); err == nil || !strings.Contains(err.Error(), "did not save the token") {
		t.Errorf("AuthLogin with a failing helper = %v", err)
	}
	if err := AuthLogout(ctx, env, AuthOptions{Profile: "globex"}); err == nil || !strings.Contains(err.Error(), "did not remove the token") {
		t.Errorf("AuthLogout with a failing helper = %v", err)
	}
	if err := AuthLogout(ctx, env, AuthOptions{Profile: "no-such-profile"}); err == nil {
		t.Error("AuthLogout for an unknown profile succeeded")
	}
}

func TestAuthStatusTreatsAnAnswerWithoutAPasswordAsNoToken(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.home)
	env.RunInput = func(context.Context, string, []string, string, ...string) ([]byte, error) {
		return []byte("protocol=https\nhost=gitlab.globex.example\nusername=globex-alex\n"), nil
	}

	err := AuthStatus(context.Background(), env, AuthOptions{})

	if err == nil || !strings.Contains(out.String(), "no token for globex-alex") {
		t.Errorf("AuthStatus = %v\n%s", err, out.String())
	}
}

func TestAuthStatusCountsMissingTokensAndInvalidProfilesTogether(t *testing.T) {
	w := newWorld(t)
	text := readFile(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"))
	text += "\n[[profile]]\nid = \"broken\"\nprovider = \"gitlab\"\nhost = \"gitlab.broken.example\"\naccount = \"bad account!\"\nname = \"A\"\nemail = \"a@broken.example\"\ndirs = [\"~/Projects/broken\"]\n"
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"), text, 0o600)
	env, out := w.env(t, w.home)
	env.RunInput = func(context.Context, string, []string, string, ...string) ([]byte, error) {
		return nil, errors.New("no helper")
	}

	err := AuthStatus(context.Background(), env, AuthOptions{})

	if err == nil || !strings.Contains(err.Error(), "no token for 1 of 2 profiles, and 1 need a valid host and account") {
		t.Errorf("AuthStatus = %v\n%s", err, out.String())
	}
}

func TestAuthStatusWithOnlyGitHubProfilesSaysThereIsNothingToCheck(t *testing.T) {
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"), `version = 1

[[profile]]
id      = "personal"
account = "octo-alex"
name    = "Alex Octo"
email   = "1000001+octo-alex@users.noreply.github.com"
dirs    = ["~/Projects/personal"]
`, 0o600)
	env, out := w.env(t, w.home)

	if err := AuthStatus(context.Background(), env, AuthOptions{}); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "No profile is on a host other than GitHub") {
		t.Errorf("output = %q", out.String())
	}
}
