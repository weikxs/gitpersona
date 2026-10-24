package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// lockedDir returns an existing directory that cannot be written to, and
// skips the test where permissions are not enforced.
func lockedDir(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	return dir
}

func writeManifest(t *testing.T, dir string, m Manifest) {
	t.Helper()
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, manifestFile), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotAndRestoreKeepASymlinkAsALink(t *testing.T) {
	work := t.TempDir()
	target := filepath.Join(work, "real.gitconfig")
	link := filepath.Join(work, "link.gitconfig")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	dir, err := Snapshot(filepath.Join(work, "backups"), fixedTime, []string{link})
	if err != nil {
		t.Fatalf("Snapshot() error: %v", err)
	}
	m, err := ReadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if e := m.Entries[0]; !e.Existed || e.Link != target || e.Stored != "" {
		t.Fatalf("entry = %+v, want a symlink record pointing at %s", e, target)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := Restore(dir); err != nil {
		t.Fatalf("Restore() error: %v", err)
	}
	if got, err := os.Readlink(link); err != nil || got != target {
		t.Errorf("restored link = %q, %v; want %q", got, err, target)
	}
}

func TestSnapshotFailsWhenTheBackupRootIsReadOnly(t *testing.T) {
	root := lockedDir(t)

	if _, err := Snapshot(root, fixedTime, nil); err == nil {
		t.Fatal("Snapshot() into a read-only root succeeded, want an error")
	}
}

func TestSnapshotFailsWhenEverySuffixForTheInstantIsTaken(t *testing.T) {
	root := t.TempDir()
	name := fixedTime.UTC().Format(timestampLayout)
	for i := 1; i <= maxSnapshotsPerInstant; i++ {
		candidate := name
		if i > 1 {
			candidate = fmt.Sprintf("%s-%d", name, i)
		}
		if err := os.Mkdir(filepath.Join(root, candidate), 0o700); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := Snapshot(root, fixedTime, nil); err == nil {
		t.Fatal("Snapshot() with all suffixes taken succeeded, want an error")
	}
}

func TestPruneAndListFailWhenRootIsNotADirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(root, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Prune(root, 1); err == nil {
		t.Error("Prune() of a regular file succeeded, want an error")
	}
	if _, err := List(root); err == nil {
		t.Error("List() of a regular file succeeded, want an error")
	}
}

func TestListOfMissingRootIsEmpty(t *testing.T) {
	infos, err := List(filepath.Join(t.TempDir(), "absent"))

	if err != nil || len(infos) != 0 {
		t.Fatalf("List() = %v, %v; want nothing and no error", infos, err)
	}
}

func TestListSkipsFilesAndDirectoriesWithoutAManifest(t *testing.T) {
	root := t.TempDir()
	good, err := Snapshot(root, fixedTime, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "zz-notes.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "zz-empty"), 0o700); err != nil {
		t.Fatal(err)
	}

	infos, err := List(root)

	if err != nil || len(infos) != 1 || infos[0].Dir != good {
		t.Fatalf("List() = %+v, %v; want only %s", infos, err, good)
	}
}

func TestPruneFailsWhenAnOldSnapshotCannotBeRemoved(t *testing.T) {
	root := lockedDir(t)
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b", "c"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}

	if err := Prune(root, 0); err == nil {
		t.Fatal("Prune() in a read-only root succeeded, want an error")
	}
}

func TestRestoreFailsWithoutAManifest(t *testing.T) {
	if err := Restore(t.TempDir()); err == nil {
		t.Fatal("Restore() of an empty directory succeeded, want an error")
	}
}

func TestRestoreFailsAndChangesNothingWhenAStoredCopyIsMissing(t *testing.T) {
	work := t.TempDir()
	dir := filepath.Join(work, "snap")
	target := filepath.Join(work, "gitconfig")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, dir, Manifest{Entries: []Entry{{Path: target, Existed: true, Stored: "0001-gone"}}})

	if err := Restore(dir); err == nil {
		t.Fatal("Restore() with a missing stored copy succeeded, want an error")
	}
	if got, _ := os.ReadFile(target); string(got) != "current" {
		t.Errorf("target = %q, want it untouched", got)
	}
}

func TestRestoreFailsWhenACreatedPathCannotBeRemoved(t *testing.T) {
	work := t.TempDir()
	dir := filepath.Join(work, "snap")
	notEmpty := filepath.Join(work, "folder")
	if err := os.MkdirAll(filepath.Join(notEmpty, "child"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, dir, Manifest{Entries: []Entry{{Path: notEmpty}}})

	if err := Restore(dir); err == nil {
		t.Fatal("Restore() removing a non-empty folder succeeded, want an error")
	}
}

func TestRestoreFailsWhenTheParentFolderCannotBeCreated(t *testing.T) {
	work := t.TempDir()
	dir := filepath.Join(work, "snap")
	blocker := filepath.Join(work, "file")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "0001-x"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, dir, Manifest{Entries: []Entry{{Path: filepath.Join(blocker, "x"), Existed: true, Stored: "0001-x"}}})

	if err := Restore(dir); err == nil {
		t.Fatal("Restore() below a regular file succeeded, want an error")
	}
}

func TestRestoreFailsWhenALinkCannotBeRecreated(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "snap")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	locked := lockedDir(t)
	writeManifest(t, dir, Manifest{Entries: []Entry{{Path: filepath.Join(locked, "link"), Existed: true, Link: "/elsewhere"}}})

	if err := Restore(dir); err == nil {
		t.Fatal("Restore() of a link into a read-only folder succeeded, want an error")
	}
}

func TestRestoreFailsWhenAFileCannotBeWritten(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "snap")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "0001-x"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	locked := lockedDir(t)
	writeManifest(t, dir, Manifest{Entries: []Entry{{Path: filepath.Join(locked, "x"), Existed: true, Stored: "0001-x"}}})

	if err := Restore(dir); err == nil {
		t.Fatal("Restore() into a read-only folder succeeded, want an error")
	}
}

func TestRestoreUsesTheDefaultModeWhenTheManifestHasNone(t *testing.T) {
	work := t.TempDir()
	dir := filepath.Join(work, "snap")
	target := filepath.Join(work, "restored")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "0001-x"), []byte("body"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, dir, Manifest{Entries: []Entry{{Path: target, Existed: true, Stored: "0001-x"}}})

	if err := Restore(dir); err != nil {
		t.Fatalf("Restore() error: %v", err)
	}
	assertPerm(t, target, 0o600)
}
