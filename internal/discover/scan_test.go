package discover

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func repo(t *testing.T, dir, remoteURL string) {
	t.Helper()
	cfg := "[core]\n\trepositoryformatversion = 0\n"
	if remoteURL != "" {
		cfg += "[remote \"origin\"]\n\turl = " + remoteURL + "\n"
	}
	write(t, filepath.Join(dir, ".git", "config"), cfg)
	write(t, filepath.Join(dir, ".git", "HEAD"), "ref: refs/heads/main\n")
}

func TestScanSuggestsDirectoriesPerAccount(t *testing.T) {
	root := t.TempDir()
	for _, r := range []struct{ dir, url string }{
		{"personal/blog", "https://github.com/octo-alex/blog.git"},
		{"personal/dots", "git@github.com:octo-alex/dots.git"},
		{"personal/fork", "https://github.com/someone-else/fork.git"},
		{"acme/billing", "https://github.com/acme-alex/billing"},
		{"acme/web", "ssh://git@github.com/acme-alex/web.git"},
		{"acme/web/node_modules/dep", "https://github.com/octo-alex/dep"},
		{"misc/local-only", ""},
	} {
		repo(t, filepath.Join(root, r.dir), r.url)
	}
	accounts := []string{"octo-alex", "acme-alex"}

	got := Scan(context.Background(), []string{root}, accounts, ScanLimits{MaxDepth: 4, Timeout: time.Second})

	if len(got) != 2 {
		t.Fatalf("Scan() = %+v, want two suggestions", got)
	}
	if got[0].Dir != filepath.Join(root, "acme") || got[0].Account != "acme-alex" || got[0].Repos != 2 || got[0].Total != 2 {
		t.Errorf("acme suggestion = %+v", got[0])
	}
	if got[1].Dir != filepath.Join(root, "personal") || got[1].Account != "octo-alex" || got[1].Repos != 2 || got[1].Total != 3 {
		t.Errorf("personal suggestion = %+v (2 of 3 repositories belong to octo-alex)", got[1])
	}
}

func TestScanRespectsDepthAndSkipsMissingRoots(t *testing.T) {
	root := t.TempDir()
	repo(t, filepath.Join(root, "a", "b", "c", "deep"), "https://github.com/octo-alex/deep")

	got := Scan(context.Background(), []string{root, filepath.Join(root, "absent")}, []string{"octo-alex"}, ScanLimits{MaxDepth: 2, Timeout: time.Second})

	if len(got) != 0 {
		t.Fatalf("Scan() went deeper than allowed: %+v", got)
	}
}

func TestAFolderThatIsItselfARepositorySuggestsNoFolder(t *testing.T) {
	root := t.TempDir()
	repo(t, root, "https://github.com/octo-alex/everything")
	repo(t, filepath.Join(root, "nested"), "https://github.com/octo-alex/nested")

	if got := Scan(context.Background(), []string{root}, []string{"octo-alex"}, ScanLimits{MaxDepth: 4, Timeout: time.Second}); len(got) != 0 {
		t.Fatalf("Scan() = %+v; a root that is a repository would suggest its parent, often the home", got)
	}
}
func TestScanReportsDirectoriesOwnedByAnOrganization(t *testing.T) {
	root := t.TempDir()
	for _, r := range []struct{ dir, url string }{
		{"work/api", "https://github.com/acme-org/api.git"},
		{"work/web", "git@github.com:Acme-Org/web.git"},
		{"work/tool", "https://github.com/acme-alex/tool"},
		{"mixed/a", "https://github.com/one/a"},
		{"mixed/b", "https://github.com/two/b"},
	} {
		repo(t, filepath.Join(root, r.dir), r.url)
	}

	got := Scan(context.Background(), []string{root}, []string{"Acme-Alex"}, ScanLimits{MaxDepth: 4, Timeout: time.Second})

	if len(got) != 1 {
		t.Fatalf("Scan() = %+v, want one suggestion (no owner has most of mixed/)", got)
	}
	if s := got[0]; s.Dir != filepath.Join(root, "work") || s.Account != "" || s.Owner != "acme-org" || s.Repos != 2 || s.Total != 3 {
		t.Errorf("an organization's directory should be suggested without an account, got %+v", s)
	}
}

func TestScanMatchesAccountsWhateverTheirCase(t *testing.T) {
	root := t.TempDir()
	repo(t, filepath.Join(root, "p", "blog"), "https://github.com/octo-alex/blog")

	got := Scan(context.Background(), []string{root}, []string{"Octo-Alex"}, ScanLimits{MaxDepth: 4, Timeout: time.Second})

	if len(got) != 1 || got[0].Account != "Octo-Alex" || got[0].Owner != "octo-alex" {
		t.Fatalf("Scan() = %+v, want the account as gh knows it", got)
	}
}
