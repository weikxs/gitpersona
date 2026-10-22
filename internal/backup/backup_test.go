package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

var fixedTime = time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)

func TestSnapshotCopiesExistingFilesAndRecordsMissingOnes(t *testing.T) {
	work := t.TempDir()
	existing := filepath.Join(work, "gitconfig")
	missing := filepath.Join(work, "not-yet.gitconfig")
	if err := os.WriteFile(existing, []byte("[user]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(work, "backups")

	dir, err := Snapshot(root, fixedTime, []string{existing, missing})
	if err != nil {
		t.Fatalf("Snapshot() error: %v", err)
	}

	if filepath.Dir(dir) != root || filepath.Base(dir) != "20260102T030405.000000006Z" {
		t.Errorf("snapshot dir = %s, want a UTC timestamp under %s", dir, root)
	}
	m, err := ReadManifest(dir)
	if err != nil {
		t.Fatalf("ReadManifest() error: %v", err)
	}
	if len(m.Entries) != 2 {
		t.Fatalf("manifest has %d entries, want 2", len(m.Entries))
	}
	saved := m.Entries[0]
	if saved.Path != existing || !saved.Existed {
		t.Errorf("first entry = %+v, want the existing file", saved)
	}
	copied, err := os.ReadFile(filepath.Join(dir, saved.Stored))
	if err != nil || string(copied) != "[user]\n" {
		t.Errorf("stored copy = %q, %v", copied, err)
	}
	if absent := m.Entries[1]; absent.Path != missing || absent.Existed || absent.Stored != "" {
		t.Errorf("second entry = %+v, want a record that the file did not exist", absent)
	}
	assertPerm(t, dir, 0o700)
	assertPerm(t, filepath.Join(dir, saved.Stored), 0o600)
	if !m.Created.Equal(fixedTime) {
		t.Errorf("manifest created = %v, want %v", m.Created, fixedTime)
	}
}

func TestSnapshotWithTheSameTimestampUsesANewDirectory(t *testing.T) {
	root := t.TempDir()
	first, err := Snapshot(root, fixedTime, nil)
	if err != nil {
		t.Fatal(err)
	}

	second, err := Snapshot(root, fixedTime, nil)

	if err != nil || second == first || filepath.Base(second) != filepath.Base(first)+"-2" {
		t.Fatalf("second Snapshot() = %s, %v; want %s-2", second, err, first)
	}
}

func TestSnapshotFailsWhenRootIsAFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(root, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Snapshot(root, fixedTime, nil); err == nil {
		t.Fatal("Snapshot() under a regular file succeeded, want an error")
	}
}

func TestSnapshotFailsWhenAFileCannotBeRead(t *testing.T) {
	work := t.TempDir()
	dirInsteadOfFile := filepath.Join(work, "a-directory")
	if err := os.Mkdir(dirInsteadOfFile, 0o700); err != nil {
		t.Fatal(err)
	}

	if _, err := Snapshot(filepath.Join(work, "backups"), fixedTime, []string{dirInsteadOfFile}); err == nil {
		t.Fatal("Snapshot() of a directory succeeded, want an error")
	}
}

func TestReadManifestMissing(t *testing.T) {
	if _, err := ReadManifest(t.TempDir()); err == nil {
		t.Fatal("ReadManifest() of an empty directory succeeded, want an error")
	}
}

func assertPerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s mode = %o, want %o", filepath.Base(path), got, want)
	}
}
