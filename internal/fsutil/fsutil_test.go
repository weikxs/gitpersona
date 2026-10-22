package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomicCreatesFileAndParentsWithPrivateModes(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "nested", "config.toml")

	if err := WriteAtomic(target, []byte("hello\n"), FilePerm); err != nil {
		t.Fatalf("WriteAtomic() error: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil || string(got) != "hello\n" {
		t.Fatalf("content = %q, %v; want %q", got, err, "hello\n")
	}
	assertMode(t, target, FilePerm)
	assertMode(t, filepath.Dir(target), DirPerm)
}

func TestWriteAtomicReplacesExistingFileWithoutLeftovers(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := WriteAtomic(target, []byte("new"), FilePerm); err != nil {
		t.Fatalf("WriteAtomic() error: %v", err)
	}

	got, _ := os.ReadFile(target)
	if string(got) != "new" {
		t.Fatalf("content = %q, want %q", got, "new")
	}
	assertMode(t, target, FilePerm)
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("directory has %d entries, want only the target (no temp files left)", len(entries))
	}
}

func TestWriteAtomicFailsWhenParentIsAFile(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := WriteAtomic(filepath.Join(blocker, "child"), []byte("x"), FilePerm); err == nil {
		t.Fatal("WriteAtomic() under a regular file succeeded, want an error")
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %o, want %o", path, got, want)
	}
}

func TestWriteAtomicFailsInReadOnlyDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if err := WriteAtomic(filepath.Join(dir, "config.toml"), []byte("x"), FilePerm); err == nil {
		t.Fatal("WriteAtomic() in a read-only directory succeeded, want an error")
	}
}
