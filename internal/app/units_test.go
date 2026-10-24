package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weikxs/gitpersona/internal/shell"
)

func TestGitValueReadsAValueTheWayGitDoes(t *testing.T) {
	for raw, want := range map[string]string{
		`plain value`:              "plain value",
		`  padded  `:               "padded",
		`"quoted # not a comment"`: "quoted # not a comment",
		`value # a comment`:        "value",
		`value ; another comment`:  "value",
		`say \"hi\"`:               `say "hi"`,
		`tab\there`:                "tab\there",
		`line\nbreak`:              "line\nbreak",
		`back\bspace`:              "back\bspace",
		`back\\slash`:              `back\slash`,
		`trailing\`:                `trailing\`,
	} {
		if got := gitValue(raw); got != want {
			t.Errorf("gitValue(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestSplitHostPortDropsTheDefaultPortAndKeepsOthers(t *testing.T) {
	for hp, want := range map[string][2]string{
		"github.com":               {"github.com", ""},
		"GitLab.Demo.Example:443":  {"gitlab.demo.example", ""},
		"gitlab.demo.example:8443": {"gitlab.demo.example", "8443"},
		"[::1]:2222":               {"::1", "2222"},
		"host.example.":            {"host.example", ""},
		"host.example:notaport":    {"host.example:notaport", ""},
	} {
		host, port := splitHostPort(hp)
		if host != want[0] || port != want[1] {
			t.Errorf("splitHostPort(%q) = %q, %q; want %q, %q", hp, host, port, want[0], want[1])
		}
	}
}

func TestOwnerDirsCollectsEveryProfileDirectory(t *testing.T) {
	owners := []ruleOwner{{id: "a", dirs: []string{"~/one", "~/two"}}, {id: "b"}, {id: "c", dirs: []string{"~/three"}}}

	got := ownerDirs(owners)

	if strings.Join(got, ",") != "~/one,~/two,~/three" {
		t.Errorf("ownerDirs() = %v", got)
	}
	if len(ownerDirs(nil)) != 0 {
		t.Error("ownerDirs(nil) should be empty")
	}
}

func TestCheckSSHLinesSkipBannersAndBlanks(t *testing.T) {
	text := "\n  Welcome to the demo host  \nPermission denied (publickey).\n"

	if got := deniedLine(text); got != "Permission denied (publickey)." {
		t.Errorf("deniedLine() = %q, want the refusal", got)
	}
	if got := deniedLine("\n\n Hello there \n"); got != "Hello there" {
		t.Errorf("deniedLine() without a refusal = %q, want the first line", got)
	}
	if got := firstLine(" \n\t\n"); got != "no answer" {
		t.Errorf("firstLine() of blank text = %q, want the placeholder", got)
	}
}

func TestReadGitFileFollowsAWorktreePointer(t *testing.T) {
	dir := t.TempDir()
	absolute := filepath.Join(dir, "absolute")
	mustWrite(t, absolute, "gitdir: /srv/demo/repo.git/worktrees/w1\n", 0o600)
	relative := filepath.Join(dir, "sub", "relative")
	mustWrite(t, relative, "gitdir:  ../real/.git \n", 0o600)
	notPointer := filepath.Join(dir, "plain")
	mustWrite(t, notPointer, "something else\n", 0o600)

	if got, ok := readGitFile(absolute); !ok || got != "/srv/demo/repo.git/worktrees/w1" {
		t.Errorf("absolute pointer = %q, %v", got, ok)
	}
	if got, ok := readGitFile(relative); !ok || got != filepath.Join(dir, "real", ".git") {
		t.Errorf("relative pointer = %q, %v; want it resolved next to the file", got, ok)
	}
	if _, ok := readGitFile(notPointer); ok {
		t.Error("a file without gitdir: was taken as a pointer")
	}
	if _, ok := readGitFile(filepath.Join(dir, "absent")); ok {
		t.Error("a missing file was taken as a pointer")
	}
}

func TestAsWrittenKeepsTheSymlinkedPathTheUserTyped(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "real-code", "repo")
	if err := os.MkdirAll(repoDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real-code"), filepath.Join(root, "code")); err != nil {
		t.Fatal(err)
	}
	typed := filepath.Join(root, "code", "repo")

	if got := asWritten(typed, repoDir); got != typed {
		t.Errorf("asWritten() = %q, want the path through the link, %q", got, typed)
	}
	if got := asWritten(filepath.Join(root, "other"), repoDir); got != repoDir {
		t.Errorf("asWritten() for an unrelated directory = %q, want the real root %q", got, repoDir)
	}
}

func TestComparisonKeyAndDisplayPathFollowTheDiskAndHome(t *testing.T) {
	env := Env{Home: "/tmp/home"}
	insensitive := Env{Home: "/tmp/home", CaseInsensitive: true}

	if got := env.comparisonKey("~/Code/Repo"); got != "/tmp/home/Code/Repo" {
		t.Errorf("comparisonKey() = %q", got)
	}
	if got := insensitive.comparisonKey("~/Code/Repo"); got != "/tmp/home/code/repo" {
		t.Errorf("comparisonKey() on a case-insensitive disk = %q", got)
	}
	if got := env.comparisonKey("relative/path"); got != "relative/path" {
		t.Errorf("comparisonKey() of a relative path = %q, want it unchanged", got)
	}
	if got := env.displayPath("/tmp/home/.ssh/key.pub"); got != "~/.ssh/key.pub" {
		t.Errorf("displayPath() = %q", got)
	}
	if got := env.displayPath("3AA5C34371567BD2"); got != "3AA5C34371567BD2" {
		t.Errorf("displayPath() of a GPG key id = %q, want it unchanged", got)
	}
}

func TestMismatchAdviceNamesTheCauseOfADisagreement(t *testing.T) {
	for name, tc := range map[string]struct {
		report WhoamiReport
		want   string
	}{
		"an environment variable": {WhoamiReport{EnvOverride: "GIT_AUTHOR_EMAIL"}, "GIT_AUTHOR_EMAIL overrides the profile"},
		"a repository setting":    {WhoamiReport{LocalEmail: "me@demo.example"}, "git config --local --unset user.email"},
		"a worktree setting":      {WhoamiReport{EmailSource: "~/repo/.git/config.worktree", EffectiveEmail: "w@demo.example"}, "git config --worktree --unset user.email"},
		"another file":            {WhoamiReport{EmailSource: "~/.gitconfig"}, "~/.gitconfig"},
	} {
		if got := mismatchAdvice(tc.report); !strings.Contains(got, tc.want) {
			t.Errorf("%s: mismatchAdvice() = %q, want it to contain %q", name, got, tc.want)
		}
	}
}

func TestInitScriptPrintsTheIntegrationForTheShell(t *testing.T) {
	w := newWorld(t)
	env, out := w.env(t, w.home)

	InitScript(env, shell.Fish)

	if got := out.String(); !strings.Contains(got, "gitpersona_prompt") || !strings.Contains(got, filepath.Join(env.ConfigDir, "bin")) {
		t.Errorf("InitScript(fish) = %q, want the fish script with the shim directory", got)
	}
}

func TestWithTildeHidesTheHomeDirectoryInPageText(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)

	got := env.WithTilde("cannot read " + filepath.Join(w.home, ".gitconfig") + " now")

	if strings.Contains(got, w.home) || !strings.Contains(got, "~/.gitconfig") {
		t.Errorf("WithTilde() = %q, want the home written as ~", got)
	}
}

func TestPreviewShellsListsStartupFilesWithoutWritingThem(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)

	changes, err := PreviewShells(env, []shell.Kind{shell.Zsh})

	if err != nil || len(changes) == 0 {
		t.Fatalf("PreviewShells() = %+v, %v; want the zsh startup files", changes, err)
	}
	for _, c := range changes {
		if !strings.HasPrefix(c.Path, "~/") || c.Action != "create" {
			t.Errorf("change = %+v, want a new file under ~/", c)
		}
	}
	if _, err := os.Stat(filepath.Join(w.home, ".zshrc")); !os.IsNotExist(err) {
		t.Error("the preview wrote .zshrc")
	}
}

func TestExistingIdentitiesMergesTheSameIdentityAcrossRules(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	mustWrite(t, filepath.Join(w.home, ".gitconfig-demo"), "[user]\n\tname = Alex Octo\n\temail = alex@demo.example\n", 0o600)
	mustWrite(t, filepath.Join(w.home, ".gitconfig"), "[includeIf \"gitdir:~/one/\"]\n\tpath = ~/.gitconfig-demo\n"+
		"[includeIf \"gitdir:~/two/\"]\n\tpath = ~/.gitconfig-demo\n", 0o600)
	env, _ := w.env(t, w.home)

	ids := env.ExistingIdentities(context.Background())

	if len(ids) != 1 || ids[0].Email != "alex@demo.example" || len(ids[0].Dirs) != 2 {
		t.Fatalf("ExistingIdentities() = %+v, want one identity with both folders", ids)
	}
}

func TestWhoamiReportForNamesTheProfileOfTheDirectory(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.repos["billing"])

	report, err := WhoamiReportFor(context.Background(), env)

	if err != nil || !report.Found || report.Profile != "acme" || report.Account != "acme-alex" {
		t.Fatalf("WhoamiReportFor() = %+v, %v; want profile acme", report, err)
	}
}

func TestWhoamiReportForFailsWithoutAConfiguration(t *testing.T) {
	w := newWorld(t)
	withoutConfig(t, w)
	env, _ := w.env(t, w.home)

	if _, err := WhoamiReportFor(context.Background(), env); err == nil {
		t.Fatal("WhoamiReportFor() without a configuration succeeded, want an error")
	}
}

func TestSetDefaultProfileClearsAndRefusesUnknownProfiles(t *testing.T) {
	w := newWorld(t)
	env, _ := w.env(t, w.home)

	unknown := SetDefaultProfile(context.Background(), env, "ghost")
	cleared := SetDefaultProfile(context.Background(), env, "")

	if unknown == nil || !strings.Contains(unknown.Error(), "unknown profile") {
		t.Errorf("SetDefaultProfile(ghost) = %v, want an unknown profile error", unknown)
	}
	if cleared != nil {
		t.Fatalf("SetDefaultProfile(\"\") = %v", cleared)
	}
	if text := readFile(t, filepath.Join(w.home, ".config", "gitpersona", "config.toml")); strings.Contains(text, "default =") {
		t.Errorf("the default is still set:\n%s", text)
	}
}
