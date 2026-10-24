package backup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSnapshotRecordsASymlinkByItsTargetAndRestoreRecreatesIt(t *testing.T) {
	work := t.TempDir()
	link := filepath.Join(work, "gitconfig")
	if err := os.Symlink("dotfiles/gitconfig", link); err != nil {
		t.Fatal(err)
	}
	dir, err := Snapshot(filepath.Join(work, "backups"), fixedTime, []string{link})
	if err != nil {
		t.Fatalf("Snapshot() error: %v", err)
	}
	m, _ := ReadManifest(dir)
	if len(m.Entries) != 1 || m.Entries[0].Link != "dotfiles/gitconfig" || m.Entries[0].Stored != "" {
		t.Fatalf("entry = %+v, want the link target and no stored copy", m.Entries)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(link, []byte("replaced by a plain file"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Restore(dir); err != nil {
		t.Fatalf("Restore() error: %v", err)
	}

	got, err := os.Readlink(link)
	if err != nil || got != "dotfiles/gitconfig" {
		t.Fatalf("Readlink() = %q, %v; want the original link back", got, err)
	}
}

func TestRestoreRefusesABackupWithAMissingFileAndChangesNothing(t *testing.T) {
	work := t.TempDir()
	target := filepath.Join(work, "gitconfig")
	if err := os.WriteFile(target, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir, err := Snapshot(filepath.Join(work, "backups"), fixedTime, []string{target})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := ReadManifest(dir)
	if err := os.Remove(filepath.Join(dir, m.Entries[0].Stored)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Restore(dir); err == nil {
		t.Fatal("Restore() with a damaged backup succeeded, want an error")
	}

	if got, _ := os.ReadFile(target); string(got) != "changed\n" {
		t.Errorf("target = %q, want it untouched by the failed restore", got)
	}
}

func TestRestoreFailsWithoutAManifest(t *testing.T) {
	if err := Restore(t.TempDir()); err == nil {
		t.Fatal("Restore() of an empty directory succeeded, want an error")
	}
}

func TestRestoreRecreatesAMissingParentFolder(t *testing.T) {
	work := t.TempDir()
	target := filepath.Join(work, "config", "git", "rules")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("rules\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir, err := Snapshot(filepath.Join(work, "backups"), fixedTime, []string{target})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(work, "config")); err != nil {
		t.Fatal(err)
	}

	if err := Restore(dir); err != nil {
		t.Fatalf("Restore() error: %v", err)
	}

	if got, _ := os.ReadFile(target); string(got) != "rules\n" {
		t.Errorf("restored content = %q", got)
	}
}

func TestRestoreFailsWhenAFileToRemoveCannotBeRemoved(t *testing.T) {
	work := t.TempDir()
	created := filepath.Join(work, "created")
	dir, err := Snapshot(filepath.Join(work, "backups"), fixedTime, []string{created})
	if err != nil {
		t.Fatal(err)
	}
	// The change left a non-empty directory where the file was recorded as new.
	if err := os.MkdirAll(filepath.Join(created, "inner"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := Restore(dir); err == nil {
		t.Fatal("Restore() succeeded although the new path cannot be removed")
	}
}

func TestRestoreFailsWhenTheParentFolderCannotBeRecreated(t *testing.T) {
	work := t.TempDir()
	target := filepath.Join(work, "gitconfig")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir, err := Snapshot(filepath.Join(work, "backups"), fixedTime, []string{target})
	if err != nil {
		t.Fatal(err)
	}
	// Point the entry under a regular file, where no folder can be created.
	blocker := filepath.Join(work, "blocker")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	m, _ := ReadManifest(dir)
	m.Entries[0].Path = filepath.Join(blocker, "gitconfig")
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, manifestFile), data, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Restore(dir); err == nil {
		t.Fatal("Restore() under a regular file succeeded, want an error")
	}
}

func TestSnapshotFailsWhenTheBackupRootIsReadOnly(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	work := t.TempDir()
	root := filepath.Join(work, "backups")
	if err := os.Mkdir(root, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })

	if _, err := Snapshot(root, fixedTime, nil); err == nil {
		t.Fatal("Snapshot() into a read-only backup root succeeded, want an error")
	}
}

func TestSnapshotGivesUpAfterTooManySnapshotsAtOneInstant(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < maxSnapshotsPerInstant; i++ {
		if _, err := Snapshot(root, fixedTime, nil); err != nil {
			t.Fatalf("Snapshot() #%d failed early: %v", i+1, err)
		}
	}

	if _, err := Snapshot(root, fixedTime, nil); err == nil {
		t.Fatal("Snapshot() succeeded although every suffix was taken")
	}
}

func TestListSkipsFilesAndSnapshotsWithoutAManifest(t *testing.T) {
	root := t.TempDir()
	good, _ := Snapshot(root, fixedTime, nil)
	if err := os.Mkdir(filepath.Join(root, "99990101T000000.000000000Z"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "stray.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := List(root)

	if err != nil || len(got) != 1 || got[0].Dir != good {
		t.Fatalf("List() = %+v, %v; want only the snapshot with a manifest", got, err)
	}
}

func TestListOfAMissingRootIsEmpty(t *testing.T) {
	got, err := List(filepath.Join(t.TempDir(), "absent"))

	if err != nil || len(got) != 0 {
		t.Fatalf("List() = %+v, %v; want nothing and no error", got, err)
	}
}

func TestListAndPruneFailWhenRootIsAFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(root, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := List(root); err == nil {
		t.Error("List() of a regular file succeeded, want an error")
	}
	if err := Prune(root, 1); err == nil {
		t.Error("Prune() of a regular file succeeded, want an error")
	}
}

func TestPruneLeavesStrayFilesAlone(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 4; i++ {
		if _, err := Snapshot(root, fixedTime.Add(time.Duration(i)*time.Hour), nil); err != nil {
			t.Fatal(err)
		}
	}
	stray := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(stray, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Prune(root, 1); err != nil {
		t.Fatalf("Prune() error: %v", err)
	}

	if _, err := os.Stat(stray); err != nil {
		t.Errorf("stray file was removed: %v", err)
	}
}
