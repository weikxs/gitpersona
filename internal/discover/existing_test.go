package discover

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/weikxs/gitpersona/internal/provider"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestExistingSetupIsTurnedIntoProfilesAndRules(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".config", "git", "personal.gitconfig"),
		"[user]\n\tname = Alex Octo\n\temail = 1000001+octo-alex@users.noreply.github.com\n[credential \"https://github.com\"]\n\tusername = octo-alex\n")
	write(t, filepath.Join(home, ".config", "git", "work.gitconfig"),
		"[user]\n\tname = Alex (Acme)\n\temail = 1000002+acme-alex@users.noreply.github.com\n")
	write(t, filepath.Join(home, ".gitconfig"), `[user]
	name = Alex
[includeIf "gitdir:~/Projects/personal/"]
	path = ~/.config/git/personal.gitconfig
[includeIf "gitdir/i:~/Projects/oss/**"]
	path = ~/.config/git/personal.gitconfig
[includeIf "gitdir:~/Projects/acme/"]
	path = ~/.config/git/work.gitconfig
[includeIf "hasconfig:remote.*.url:git@github-work:*/**"]
	path = ~/.config/git/work.gitconfig
# >>> gitpersona >>>
[include]
	path = ~/.config/gitpersona/rules.gitconfig
# <<< gitpersona <<<
`)

	got, err := Existing(context.Background(), home, filepath.Join(home, ".gitconfig"), provider.ExecRunner)

	if err != nil {
		t.Fatalf("Existing() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Existing() = %+v, want two identities", got)
	}
	personal, work := got[0], got[1]
	if personal.Account != "octo-alex" || personal.Email != "1000001+octo-alex@users.noreply.github.com" || personal.Name != "Alex Octo" ||
		len(personal.Dirs) != 2 || personal.Dirs[0] != "~/Projects/personal" || personal.Dirs[1] != "~/Projects/oss" {
		t.Errorf("personal = %+v", personal)
	}
	if work.Account != "" || work.Email != "1000002+acme-alex@users.noreply.github.com" || len(work.Dirs) != 1 || work.Dirs[0] != "~/Projects/acme" {
		t.Errorf("work = %+v (hasconfig rules cannot be expressed as directories and are skipped)", work)
	}
}

func TestExistingWithoutIncludeIfFindsNothing(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".gitconfig"), "[user]\n\tname = Alex\n")

	got, err := Existing(context.Background(), home, filepath.Join(home, ".gitconfig"), provider.ExecRunner)

	if err != nil || len(got) != 0 {
		t.Fatalf("Existing() = %+v, %v", got, err)
	}
}

func TestGitdirPatternToDirectory(t *testing.T) {
	for in, want := range map[string]string{
		"gitdir:~/Projects/a/":     "~/Projects/a",
		"gitdir/i:~/Projects/b/**": "~/Projects/b",
		"gitdir:/srv/code/":        "/srv/code",
		"gitdir:~/Projects/c/.git": "",
		"gitdir:relative/":         "",
		"onbranch:main":            "",
	} {
		if got := gitdirToDir(in); got != want {
			t.Errorf("gitdirToDir(%q) = %q, want %q", in, got, want)
		}
	}
}
