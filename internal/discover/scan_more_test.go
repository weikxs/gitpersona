package discover

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReposSkipsARepositoryWhoseConfigCannotBeRead(t *testing.T) {
	root := t.TempDir()
	repo(t, filepath.Join(root, "ok"), "https://github.com/demo-work/ok")
	broken := filepath.Join(root, "broken")
	// A directory named config passes the repository check but cannot be read.
	if err := os.MkdirAll(filepath.Join(broken, ".git", "config"), 0o700); err != nil {
		t.Fatal(err)
	}

	found := Repos(context.Background(), []string{root}, ScanLimits{MaxDepth: 4, Timeout: testScanTimeout})

	if len(found.Repos) != 1 || filepath.Base(found.Repos[0].Dir) != "ok" {
		t.Fatalf("Repos() = %+v, want only the readable repository", found.Repos)
	}
}

func TestWalkReposSkipsHiddenFolders(t *testing.T) {
	root := t.TempDir()
	repo(t, filepath.Join(root, ".cache", "hidden"), "")
	repo(t, filepath.Join(root, "visible"), "")
	var seen []string

	walkRepos(context.Background(), root, 4, func(dir string, _ bool) { seen = append(seen, filepath.Base(dir)) })

	if len(seen) != 1 || seen[0] != "visible" {
		t.Fatalf("walkRepos() found %v, want only visible", seen)
	}
}

func TestHasRepoBelowIgnoresFilesHiddenAndSkippedFolders(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "notes.txt"), "x")
	repo(t, filepath.Join(root, ".hidden", "r"), "")
	repo(t, filepath.Join(root, "node_modules", "r"), "")
	if err := os.MkdirAll(filepath.Join(root, "empty", "deeper"), 0o700); err != nil {
		t.Fatal(err)
	}

	if hasRepoBelow(root, deepLook) {
		t.Error("hasRepoBelow() = true, want false: nothing it may look at is a repository")
	}
	if hasRepoBelow(root, 0) {
		t.Error("hasRepoBelow() with no levels left = true, want false")
	}
}

func TestHasRepoBelowIsFalseForAnUnreadableFolder(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads any folder")
	}
	dir := filepath.Join(t.TempDir(), "closed")
	if err := os.Mkdir(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if hasRepoBelow(dir, deepLook) {
		t.Fatal("hasRepoBelow() of an unreadable folder = true, want false")
	}
}

func TestNestedReposIgnoresAFolderItCannotList(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads any folder")
	}
	outer := filepath.Join(t.TempDir(), "outer")
	repo(t, outer, "")
	if err := os.Chmod(outer, 0o100); err != nil { // enter, but not list
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(outer, 0o700) })
	var seen []string

	nestedRepos(context.Background(), outer, maxNesting, func(dir string, _ bool) { seen = append(seen, dir) })

	if len(seen) != 0 {
		t.Fatalf("nestedRepos() = %v, want nothing", seen)
	}
}
