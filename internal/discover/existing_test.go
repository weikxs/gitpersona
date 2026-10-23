package discover

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func TestExistingIdentitiesKeepTheirSSHKeyAndSigning(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".gitconfig-work"), "[user]\n\temail = worker@demo.example\n\tsigningkey = ~/.ssh/work_sign.pub\n"+
		"[gpg]\n\tformat = ssh\n[commit]\n\tgpgsign = true\n[core]\n\tsshCommand = ssh -i '~/.ssh/id work' -o IdentitiesOnly=yes\n")
	write(t, filepath.Join(home, ".gitconfig-fun"), "[user]\n\temail = fun@demo.example\n\tsigningkey = 3AA5C34371567BD2\n[commit]\n\tgpgsign = true\n")
	write(t, filepath.Join(home, ".gitconfig"), "[includeIf \"gitdir:~/work/\"]\n\tpath = ~/.gitconfig-work\n"+
		"[includeIf \"gitdir:~/fun/\"]\n\tpath = ~/.gitconfig-fun\n")

	got, err := Existing(context.Background(), home, filepath.Join(home, ".gitconfig"), provider.ExecRunner)

	if err != nil || len(got) != 2 {
		t.Fatalf("Existing() = %+v, %v", got, err)
	}
	if w := got[0]; w.SSHKey != "~/.ssh/id work" || w.Signing.Method != "ssh" || w.Signing.Key != "~/.ssh/work_sign.pub" {
		t.Errorf("work = %+v, want its SSH key and SSH signing", w)
	}
	if f := got[1]; f.SSHKey != "" || f.Signing.Method != "gpg" || f.Signing.Key != "3AA5C34371567BD2" {
		t.Errorf("fun = %+v, want GPG signing", f)
	}
}

func TestSSHCommandKey(t *testing.T) {
	for cmd, want := range map[string]string{
		"ssh -i ~/.ssh/id_work -o IdentitiesOnly=yes": "~/.ssh/id_work",
		`ssh -o IdentitiesOnly=yes -i "/k/id x"`:      "/k/id x",
		"ssh -i'~/.ssh/a'":                            "~/.ssh/a",
		"ssh":                                         "",
		"ssh -o IdentityFile=~/.ssh/b":                "~/.ssh/b",
		"ssh -oIdentityFile=~/.ssh/c":                 "~/.ssh/c",
	} {
		if got := sshCommandKey(cmd); got != want {
			t.Errorf("sshCommandKey(%q) = %q, want %q", cmd, got, want)
		}
	}
}

func TestIdentityRulesSayWhichOnesGitPersonaCanReplace(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".gitconfig-plain"), "[user]\n\tname = A\n\temail = a@demo.example\n"+
		"[core]\n\tsshCommand = ssh -i ~/.ssh/a -o IdentitiesOnly=yes\n[commit]\n\tgpgsign = yes\n[user]\n\tsigningkey = KEYID\n")
	write(t, filepath.Join(home, ".gitconfig-url"), "[user]\n\temail = b@demo.example\n[url \"git@github-b:\"]\n\tinsteadOf = https://github.com/\n")
	write(t, filepath.Join(home, ".gitconfig-port"), "[user]\n\temail = c@demo.example\n[core]\n\tsshCommand = ssh -p 2222 -i ~/.ssh/c\n")
	write(t, filepath.Join(home, ".gitconfig"), "[includeIf \"gitdir:~/a/\"]\n\tpath = ~/.gitconfig-plain\n"+
		"[includeIf \"gitdir:~/b/\"]\n\tpath = ~/.gitconfig-url\n"+
		"[includeIf \"gitdir:~/c/\"]\n\tpath = ~/.gitconfig-port\n"+
		"[includeIf \"gitdir:~/d/\"]\n\tpath = ~/.gitconfig-plain\n\tpath = ~/.gitconfig-extra\n")

	got := IdentityRules(context.Background(), home, filepath.Join(home, ".gitconfig"), provider.ExecRunner)

	keep := map[string]string{}
	for _, r := range got {
		keep[r.Dir] = r.Keep
	}
	if keep["~/a"] != "" {
		t.Errorf("~/a holds only an identity and can be replaced, got %q", keep["~/a"])
	}
	for dir, why := range map[string]string{"~/b": "url.", "~/c": "sshCommand", "~/d": "more than one file"} {
		if !strings.Contains(keep[dir], why) {
			t.Errorf("%s must be kept, mentioning %q; got %q", dir, why, keep[dir])
		}
	}
	ids, _ := Existing(context.Background(), home, filepath.Join(home, ".gitconfig"), provider.ExecRunner)
	if ids[0].Signing.Method != "gpg" || ids[0].Signing.Key != "KEYID" {
		t.Errorf("gpgsign = yes turns signing on, got %+v", ids[0].Signing)
	}
}
