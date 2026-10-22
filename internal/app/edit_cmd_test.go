package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/config"
)

const commentedConfig = `# demo setup
version = 1
default = "acme" # work by default

[[profile]]
id      = "personal"
account = "octo-alex"
name    = "Alex Octo"
email   = "1000001+octo-alex@users.noreply.github.com"
dirs    = [
  "~/Projects/personal", # main
]

[[profile]]
id      = "acme"
account = "acme-alex"
name    = "Alex (Acme)"
email   = "1000002+acme-alex@users.noreply.github.com"
dirs    = ["~/Projects/acme"]
`

func editWorld(t *testing.T) world {
	t.Helper()
	w := newWorld(t)
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"), commentedConfig, 0o600)
	return w
}

func readConfig(t *testing.T, w world) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(w.home, ".config", "gitpersona", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestUseAddsTheCurrentDirectoryKeepsCommentsAndApplies(t *testing.T) {
	w := editWorld(t)
	side := filepath.Join(w.home, "side-projects")
	runGit(t, w.home, "init", "-q", filepath.Join(side, "tool"))
	env, out := w.env(t, side)

	if err := Use(context.Background(), env, "personal"); err != nil {
		t.Fatalf("Use() error: %v", err)
	}

	text := readConfig(t, w)
	if !strings.Contains(text, `"~/side-projects",`) || !strings.Contains(text, "# main") || !strings.Contains(text, "# demo setup") {
		t.Fatalf("config after use:\n%s", text)
	}
	if got := runGit(t, filepath.Join(side, "tool"), "config", "user.email"); got != "1000001+octo-alex@users.noreply.github.com" {
		t.Fatalf("use should take effect immediately, git uses %q\n%s", got, out.String())
	}
}

func TestUseMovesADirectoryBetweenProfiles(t *testing.T) {
	w := editWorld(t)
	env, out := w.env(t, filepath.Join(w.home, "Projects", "acme"))

	if err := Use(context.Background(), env, "personal"); err != nil {
		t.Fatal(err)
	}

	text := readConfig(t, w)
	if strings.Contains(text, `dirs    = ["~/Projects/acme"]`) || !strings.Contains(text, `"~/Projects/acme",`) {
		t.Fatalf("directory should move from acme to personal:\n%s", text)
	}
	if !strings.Contains(out.String(), "moved from acme") {
		t.Fatalf("use should say the directory moved:\n%s", out.String())
	}
}

func TestUseRequiresAKnownProfile(t *testing.T) {
	w := editWorld(t)
	env, _ := w.env(t, w.home)

	for _, id := range []string{"", "ghost"} {
		if err := Use(context.Background(), env, id); err == nil || !strings.Contains(err.Error(), "personal, acme") {
			t.Errorf("Use(%q) = %v, want an error listing the profiles", id, err)
		}
	}
}

func TestRemoveDirectory(t *testing.T) {
	w := editWorld(t)
	env, _ := w.env(t, filepath.Join(w.home, "Projects", "acme"))

	if err := Remove(context.Background(), env, ""); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}

	if strings.Contains(readConfig(t, w), "~/Projects/acme") {
		t.Fatalf("directory still configured:\n%s", readConfig(t, w))
	}
	if err := Remove(context.Background(), env, ""); err == nil {
		t.Fatal("removing an unconfigured directory succeeded")
	}
}

func TestPinAndUnpinCurrentRepository(t *testing.T) {
	w := editWorld(t)
	env, _ := w.env(t, w.repos["scratch"])

	if err := Pin(context.Background(), env, "personal"); err != nil {
		t.Fatalf("Pin() error: %v", err)
	}
	if got := runGit(t, w.repos["scratch"], "config", "user.email"); got != "1000001+octo-alex@users.noreply.github.com" {
		t.Fatalf("pin should take effect, git uses %q", got)
	}
	if err := Unpin(context.Background(), env); err != nil {
		t.Fatalf("Unpin() error: %v", err)
	}
	if readConfig(t, w) != commentedConfig {
		t.Fatalf("unpin should restore the configuration exactly:\n%s", readConfig(t, w))
	}
}

func TestPinRefusesWorktreesAndNonRepositories(t *testing.T) {
	w := editWorld(t)
	runGit(t, w.repos["blog"], "-c", "user.email=x@example.com", "-c", "user.name=x", "commit", "-q", "--allow-empty", "-m", "init")
	wt := filepath.Join(w.home, "Desktop", "blog-wt")
	runGit(t, w.repos["blog"], "worktree", "add", "-q", wt)
	env, _ := w.env(t, wt)

	if err := Pin(context.Background(), env, "acme"); err == nil || !strings.Contains(err.Error(), "worktree") {
		t.Fatalf("Pin() in a worktree = %v, want an explanation", err)
	}
	env.WorkDir = w.home
	if err := Pin(context.Background(), env, "acme"); err == nil || !strings.Contains(err.Error(), "not inside a Git repository") {
		t.Fatalf("Pin() outside a repository = %v", err)
	}
}

func TestListShowsProfilesDirectoriesAndPins(t *testing.T) {
	w := editWorld(t)
	env, out := w.env(t, w.repos["scratch"])
	if err := Pin(context.Background(), env, "personal"); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	if err := List(context.Background(), env, false); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"personal", "octo-alex", "~/Projects/personal", "acme (default)", "pin ~/Desktop/scratch"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("list is missing %q:\n%s", want, out.String())
		}
	}

	out.Reset()
	if err := List(context.Background(), env, true); err != nil {
		t.Fatal(err)
	}
	var got ListReport
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || len(got.Profiles) != 2 || got.Default != "acme" || len(got.Pins) != 1 {
		t.Fatalf("list --json = %+v, %v\n%s", got, err, out.String())
	}
}

func TestDoctorWarnsAboutPinsToMissingRepositories(t *testing.T) {
	w := editWorld(t)
	text := commentedConfig + "\n[[pin]]\nrepo    = \"~/Projects/not-cloned-yet\"\nprofile = \"personal\"\n"
	mustWrite(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml"), text, 0o600)
	env, out := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	ok := Doctor(context.Background(), env)

	if !ok || !strings.Contains(out.String(), "! pin ~/Projects/not-cloned-yet") {
		t.Fatalf("doctor should warn (not fail) about a pin to a missing repository:\n%s", out.String())
	}
}

func TestEditRefusesWhenTheResultWouldDiffer(t *testing.T) {
	w := editWorld(t)
	env, _ := w.env(t, w.home)
	before := readConfig(t, w)

	err := env.saveEdited(context.Background(), before+"\n[[profile]]\nid = \"surprise\"\n", mustParse(t, before))

	if err == nil || !strings.Contains(err.Error(), "edit it by hand") {
		t.Fatalf("saveEdited() = %v, want a refusal", err)
	}
	if readConfig(t, w) != before {
		t.Fatal("configuration was written despite the mismatch")
	}
}

func mustParse(t *testing.T, text string) config.Config {
	t.Helper()
	cfg, err := config.Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestRollbackRestoresAnEarlierState(t *testing.T) {
	w := editWorld(t)
	env, out := w.env(t, filepath.Join(w.home, "side"))
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(env.WorkDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := Use(context.Background(), env, "personal"); err != nil {
		t.Fatal(err)
	}
	afterUse := readConfig(t, w)
	out.Reset()

	if err := Rollback(context.Background(), env, "", false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "latest") || !strings.Contains(out.String(), "config.toml") {
		t.Fatalf("rollback without a snapshot should list the backups:\n%s", out.String())
	}
	if err := Rollback(context.Background(), env, "latest", false); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("rollback without --yes = %v, want a request for confirmation", err)
	}
	if err := Rollback(context.Background(), env, "latest", true); err != nil {
		t.Fatal(err)
	}

	if readConfig(t, w) != commentedConfig || readConfig(t, w) == afterUse {
		t.Fatalf("rollback should restore the configuration before `use`:\n%s", readConfig(t, w))
	}
}

func TestUninstallRestoresTheUsersGitConfig(t *testing.T) {
	w := editWorld(t)
	userConfig := "[user]\n\temail = me@example.com\n"
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), userConfig, 0o644)
	env, _ := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	if err := Uninstall(context.Background(), env, false); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("uninstall without --yes = %v", err)
	}
	if err := Uninstall(context.Background(), env, true); err != nil {
		t.Fatal(err)
	}

	if got, _ := os.ReadFile(filepath.Join(w.home, ".gitconfig")); string(got) != userConfig {
		t.Fatalf("~/.gitconfig after uninstall = %q, want the original", got)
	}
	for _, gone := range []string{"rules.gitconfig", "profiles"} {
		if _, err := os.Stat(filepath.Join(w.home, ".config", "gitpersona", gone)); !os.IsNotExist(err) {
			t.Errorf("%s still exists after uninstall", gone)
		}
	}
	if readConfig(t, w) != commentedConfig {
		t.Error("uninstall must keep the user's config.toml")
	}
	if got := runGit(t, w.repos["blog"], "config", "user.email"); got != "me@example.com" {
		t.Fatalf("after uninstall git should use the user's own email, got %q", got)
	}
}

func TestUninstallDeletesAGitConfigItCreated(t *testing.T) {
	w := editWorld(t)
	env, _ := w.env(t, w.home)
	if err := Apply(context.Background(), env, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	if err := Uninstall(context.Background(), env, true); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(w.home, ".gitconfig")); !os.IsNotExist(err) {
		t.Fatal("~/.gitconfig created by GitPersona should be removed when it would be empty")
	}
}
