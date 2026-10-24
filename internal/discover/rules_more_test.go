package discover

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/weikxs/gitpersona/internal/provider"
)

func TestRemoteHostOwnerReadsHostAndOwnerOfEveryRemoteForm(t *testing.T) {
	for url, want := range map[string][2]string{
		"https://github.com/octo-alex/blog.git":     {"github.com", "octo-alex"},
		"git@github.com:octo-alex/blog.git":         {"github.com", "octo-alex"},
		"ssh://git@gitlab.demo.example/acme/web":    {"gitlab.demo.example", "acme"},
		"git@github-work:*/**":                      {"github-work", "*"},
		"https://user:pw@github.com/octo-alex/blog": {"github.com", "octo-alex"},
		"/srv/git/blog.git":                         {"", ""},
		"file:///srv/git/blog.git":                  {"", ""},
		"https://github.com":                        {"", ""},
	} {
		host, owner := RemoteHostOwner(url)
		if host != want[0] || owner != want[1] {
			t.Errorf("RemoteHostOwner(%q) = %q, %q; want %q, %q", url, host, owner, want[0], want[1])
		}
	}
}

func TestRulesSettingsListsWhatEachIncludedFileSets(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".gitconfig-tags"), "[user]\n\tsigningkey = ~/.ssh/tags.pub\n[core]\n\tsshCommand = ssh -i ~/.ssh/id_tags\n")
	write(t, filepath.Join(home, ".gitconfig-plain"), "[pull]\n\trebase = true\n")
	write(t, filepath.Join(home, ".gitconfig"), "[includeIf \"gitdir:~/tags/\"]\n\tpath = ~/.gitconfig-tags\n"+
		"[includeIf \"onbranch:main\"]\n\tpath = ~/.gitconfig-plain\n"+
		"[includeIf \"gitdir:~/self/\"]\n\tpath = ~/.gitconfig\n")

	got := RulesSettings(context.Background(), home, filepath.Join(home, ".gitconfig"), provider.ExecRunner)

	if len(got) != 2 {
		t.Fatalf("RulesSettings() = %+v, want two rules (a rule including the file itself is skipped)", got)
	}
	tags, branch := got[0], got[1]
	if tags.Dir != "~/tags" || !tags.Keys["user.signingkey"] || !tags.Keys["core.sshcommand"] || !tags.SSHKey {
		t.Errorf("tags rule = %+v, want a directory, its keys and an SSH key", tags)
	}
	if branch.Dir != "" || !branch.Keys["pull.rebase"] || branch.SSHKey {
		t.Errorf("branch rule = %+v, want no directory and no SSH key", branch)
	}
}

func TestRulesSettingsOfAMissingFileIsEmpty(t *testing.T) {
	home := t.TempDir()

	if got := RulesSettings(context.Background(), home, filepath.Join(home, ".gitconfig"), provider.ExecRunner); len(got) != 0 {
		t.Fatalf("RulesSettings() = %+v, want none", got)
	}
}

func TestOtherIdentityRulesFindsBranchAndWholeHomeRulesButNotFolderRules(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".gitconfig-main"), "[user]\n\tname = Alex Octo\n\temail = main@demo.example\n")
	write(t, filepath.Join(home, ".gitconfig-folder"), "[user]\n\temail = folder@demo.example\n")
	write(t, filepath.Join(home, ".gitconfig-none"), "[pull]\n\trebase = true\n")
	write(t, filepath.Join(home, ".gitconfig"), "[includeIf \"onbranch:main\"]\n\tpath = ~/.gitconfig-main\n"+
		"[includeIf \"gitdir:~/folder/\"]\n\tpath = ~/.gitconfig-folder\n"+
		"[includeIf \"onbranch:dev\"]\n\tpath = ~/.gitconfig-none\n"+
		"[includeIf \"onbranch:self\"]\n\tpath = ~/.gitconfig\n")

	got := OtherIdentityRules(context.Background(), home, filepath.Join(home, ".gitconfig"), provider.ExecRunner)

	if len(got) != 1 || got[0].Condition != "onbranch:main" || got[0].Email != "main@demo.example" || got[0].Name != "Alex Octo" || got[0].Dir != "" {
		t.Fatalf("OtherIdentityRules() = %+v, want only the onbranch rule that sets an identity", got)
	}
}

func TestIncludedPathIsReadTheWayGitReadsIt(t *testing.T) {
	gitconfig := "/tmp/home/.config/git/config"
	for path, want := range map[string]string{
		"~/rules.gitconfig":  "/tmp/home/rules.gitconfig",
		"/etc/shared.config": "/etc/shared.config",
		"work.gitconfig":     "/tmp/home/.config/git/work.gitconfig",
	} {
		if got := includedPath(path, "/tmp/home", gitconfig); got != want {
			t.Errorf("includedPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestHasRepoBelowLooksOnlyTheGivenLevelsDeepAndSkipsHiddenAndVendoredFolders(t *testing.T) {
	root := t.TempDir()
	repo(t, filepath.Join(root, "one", "two", "found"), "")
	repo(t, filepath.Join(root, ".hidden", "repo"), "")
	repo(t, filepath.Join(root, "node_modules", "dep"), "")
	if err := os.WriteFile(filepath.Join(root, "file.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		dir    string
		levels int
		want   bool
	}{
		"zero levels sees nothing":               {root, 0, false},
		"one level is too shallow":               {root, 1, false},
		"two levels reach a folder's child":      {root, 2, false},
		"three levels reach the repository":      {root, 3, true},
		"one level below a folder with the repo": {filepath.Join(root, "one", "two"), 1, true},
		"a missing folder holds nothing":         {filepath.Join(root, "absent"), 3, false},
	} {
		if got := hasRepoBelow(tc.dir, tc.levels); got != tc.want {
			t.Errorf("%s: hasRepoBelow(%d levels) = %v, want %v", name, tc.levels, got, tc.want)
		}
	}
}

func TestReposSaysTooDeepOnlyWhenARepositoryIsWithinReachBelowTheLimit(t *testing.T) {
	near := t.TempDir()
	repo(t, filepath.Join(near, "a", "b", "c", "d"), "")
	far := t.TempDir()
	repo(t, filepath.Join(far, "a", "b", "c", "d", "e", "f", "g"), "")
	limits := ScanLimits{MaxDepth: 2, Timeout: testScanTimeout}

	gotNear := Repos(context.Background(), []string{near}, limits)
	gotFar := Repos(context.Background(), []string{far}, limits)

	if !gotNear.TooDeep {
		t.Error("a repository just below the limit should be reported as left out")
	}
	if gotFar.TooDeep {
		t.Error("a repository far below the limit is not looked for")
	}
	if len(gotNear.Repos) != 0 {
		t.Errorf("Repos = %+v, want none within the limit", gotNear.Repos)
	}
}
