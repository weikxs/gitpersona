package discover

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/weikxs/gitpersona/internal/provider"
)

func TestRulesSettingsListsEveryRuleWithTheKeysItsFileSets(t *testing.T) {
	home := t.TempDir()
	gitconfig := filepath.Join(home, ".gitconfig")
	write(t, filepath.Join(home, ".gitconfig-acme"),
		"[user]\n\temail = acme-alex@demo.example\n[core]\n\tsshCommand = ssh -i ~/.ssh/acme -o IdentitiesOnly=yes\n")
	write(t, filepath.Join(home, ".gitconfig-plain"), "[core]\n\tsshCommand = ssh -p 2222\n")
	write(t, gitconfig, "[includeIf \"gitdir:~/acme/\"]\n\tpath = ~/.gitconfig-acme\n"+
		"[includeIf \"hasconfig:remote.*.url:git@github-acme:*/**\"]\n\tpath = ~/.gitconfig-plain\n"+
		"[includeIf \"gitdir:~/self/\"]\n\tpath = ~/.gitconfig\n"+
		"[includeIf \"gitdir:~/missing/\"]\n\tpath = ~/.gitconfig-absent\n")

	got := RulesSettings(context.Background(), home, gitconfig, provider.ExecRunner)

	if len(got) != 3 {
		t.Fatalf("RulesSettings() = %+v, want three rules (the self-include is skipped)", got)
	}
	acme, hasconfig, missing := got[0], got[1], got[2]
	if acme.Dir != "~/acme" || !acme.Keys["user.email"] || !acme.Keys["core.sshcommand"] || !acme.SSHKey {
		t.Errorf("acme rule = %+v, want its directory, keys and an ssh key", acme)
	}
	if hasconfig.Dir != "" || !hasconfig.Keys["core.sshcommand"] || hasconfig.SSHKey {
		t.Errorf("hasconfig rule = %+v, want no directory and an sshCommand without a key", hasconfig)
	}
	if missing.Dir != "~/missing" || len(missing.Keys) != 0 {
		t.Errorf("missing-file rule = %+v, want a rule with no keys", missing)
	}
}

func TestOtherIdentityRulesReturnsOnlyRulesThatAreNotPlainDirectories(t *testing.T) {
	home := t.TempDir()
	gitconfig := filepath.Join(home, ".gitconfig")
	write(t, filepath.Join(home, ".gitconfig-branch"), "[user]\n\tname = Alex Octo\n\temail = octo-alex@demo.example\n")
	write(t, filepath.Join(home, ".gitconfig-empty"), "[core]\n\teditor = vi\n")
	write(t, gitconfig, "[includeIf \"gitdir:~/acme/\"]\n\tpath = ~/.gitconfig-branch\n"+
		"[includeIf \"onbranch:main\"]\n\tpath = ~/.gitconfig-branch\n"+
		"[includeIf \"gitdir:"+home+"/\"]\n\tpath = ~/.gitconfig-branch\n"+
		"[includeIf \"onbranch:dev\"]\n\tpath = ~/.gitconfig-empty\n"+
		"[includeIf \"onbranch:self\"]\n\tpath = ~/.gitconfig\n")

	got := OtherIdentityRules(context.Background(), home, gitconfig, provider.ExecRunner)

	if len(got) != 2 || got[0].Condition != "onbranch:main" || got[1].Condition != "gitdir:"+home+"/" {
		t.Fatalf("OtherIdentityRules() = %+v, want the onbranch rule and the whole-home rule", got)
	}
	if got[0].Email != "octo-alex@demo.example" || got[0].Name != "Alex Octo" {
		t.Errorf("first rule = %+v, want the identity its file sets", got[0])
	}
}

func TestIncludeIfPathsIgnoresASectionWithoutACondition(t *testing.T) {
	home := t.TempDir()
	gitconfig := filepath.Join(home, ".gitconfig")
	write(t, gitconfig, "[includeIf]\n\tpath = ~/.gitconfig-x\n[includeIf \"onbranch:main\"]\n\tpath = ~/.gitconfig-y\n")

	got := includeIfPaths(context.Background(), gitconfig, provider.ExecRunner)

	if len(got) != 1 || got[0].condition != "onbranch:main" {
		t.Fatalf("includeIfPaths() = %+v, want only the rule that has a condition", got)
	}
}

func TestIdentityRulesSkipARuleThatIncludesTheGlobalFileItself(t *testing.T) {
	home := t.TempDir()
	gitconfig := filepath.Join(home, ".gitconfig")
	write(t, gitconfig, "[user]\n\temail = octo-alex@demo.example\n[includeIf \"gitdir:~/loop/\"]\n\tpath = ~/.gitconfig\n")

	got := IdentityRules(context.Background(), home, gitconfig, provider.ExecRunner)

	if len(got) != 0 {
		t.Fatalf("IdentityRules() = %+v, want none", got)
	}
}

func TestIdentityRulesExplainWhySomeSigningSetupsAreKept(t *testing.T) {
	home := t.TempDir()
	gitconfig := filepath.Join(home, ".gitconfig")
	write(t, filepath.Join(home, ".gitconfig-tags"), "[user]\n\temail = a@demo.example\n\tsigningkey = KEYID\n")
	write(t, filepath.Join(home, ".gitconfig-x509"),
		"[user]\n\temail = b@demo.example\n\tsigningkey = KEYID\n[commit]\n\tgpgsign = true\n[gpg]\n\tformat = x509\n")
	write(t, filepath.Join(home, ".gitconfig-ssh"),
		"[user]\n\temail = c@demo.example\n\tsigningkey = ~/.ssh/c.pub\n[commit]\n\tgpgsign = true\n[gpg]\n\tformat = ssh\n")
	write(t, gitconfig, "[includeIf \"gitdir:~/a/\"]\n\tpath = ~/.gitconfig-tags\n"+
		"[includeIf \"gitdir:~/b/\"]\n\tpath = ~/.gitconfig-x509\n"+
		"[includeIf \"gitdir:~/c/\"]\n\tpath = ~/.gitconfig-ssh\n")

	keep := map[string]string{}
	for _, r := range IdentityRules(context.Background(), home, gitconfig, provider.ExecRunner) {
		keep[r.Dir] = r.Keep
	}
	ids, err := Existing(context.Background(), home, gitconfig, provider.ExecRunner)

	if keep["~/a"] == "" || keep["~/b"] == "" || keep["~/c"] != "" {
		t.Errorf("keep reasons = %+v; ~/a (key without signing) and ~/b (x509) must stay, ~/c can be replaced", keep)
	}
	if err != nil {
		t.Fatalf("Existing() error: %v", err)
	}
	var sshSigning Signing
	for _, id := range ids {
		if id.Email == "c@demo.example" {
			sshSigning = id.Signing
		}
	}
	if sshSigning.Method != "ssh" || sshSigning.Key != "~/.ssh/c.pub" {
		t.Errorf("ssh identity signing = %+v, want ssh with its key", sshSigning)
	}
}

func TestPlainSSHCommand(t *testing.T) {
	tests := []struct {
		command string
		want    bool
	}{
		{"ssh -i ~/.ssh/acme -o IdentitiesOnly=yes", true},
		{"ssh -i~/.ssh/acme", true},
		{"ssh -oIdentitiesOnly=yes", true},
		{"ssh -i", false},
		{"ssh -o", false},
		{"ssh -o StrictHostKeyChecking=no", false},
		{"ssh -p 2222", false},
		{"scp -i ~/.ssh/acme", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := plainSSHCommand(tc.command); got != tc.want {
			t.Errorf("plainSSHCommand(%q) = %v, want %v", tc.command, got, tc.want)
		}
	}
}

func TestIncludedPathResolvesHomeRelativeAndAbsolutePaths(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "demo", "home")
	gitconfig := filepath.Join(home, ".config", "git", "config")
	abs := filepath.Join(string(filepath.Separator), "srv", "rules.gitconfig")
	tests := []struct{ path, want string }{
		{"~/work.gitconfig", filepath.Join(home, "work.gitconfig")},
		{"work.gitconfig", filepath.Join(home, ".config", "git", "work.gitconfig")},
		{abs, abs},
	}
	for _, tc := range tests {
		if got := includedPath(tc.path, home, gitconfig); got != tc.want {
			t.Errorf("includedPath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}
