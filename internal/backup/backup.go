// Package backup snapshots files before GitPersona changes them, so every
// change can be rolled back. A snapshot is a timestamped directory holding a
// copy of each file that existed and a manifest that also records the files
// that did not exist yet.
package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/weikxs/gitpersona/internal/fsutil"
)

const (
	manifestFile    = "manifest.json"
	timestampLayout = "20060102T150405.000000000Z"
)

// Manifest describes one snapshot.
type Manifest struct {
	Created time.Time `json:"created"`
	Entries []Entry   `json:"entries"`
}

// Entry records one file as it was before the change.
type Entry struct {
	Path    string `json:"path"`             // absolute path of the original file
	Existed bool   `json:"existed"`          // false: the change created the file
	Stored  string `json:"stored,omitempty"` // copy inside the snapshot, if Existed
}

// maxSnapshotsPerInstant bounds the suffixes tried when timestamps collide.
const maxSnapshotsPerInstant = 100

// Snapshot copies files into a new directory under root named after now and
// returns that directory. An existing snapshot is never reused: when the name
// is taken, a numeric suffix is added.
func Snapshot(root string, now time.Time, files []string) (string, error) {
	if err := os.MkdirAll(root, fsutil.DirPerm); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}
	dir, err := newSnapshotDir(root, now.UTC().Format(timestampLayout))
	if err != nil {
		return "", err
	}

	manifest := Manifest{Created: now, Entries: make([]Entry, 0, len(files))}
	for i, path := range files {
		entry, err := saveOne(dir, i, path)
		if err != nil {
			return "", err
		}
		manifest.Entries = append(manifest.Entries, entry)
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode backup manifest: %w", err)
	}
	if err := fsutil.WriteAtomic(filepath.Join(dir, manifestFile), data, fsutil.FilePerm); err != nil {
		return "", err
	}
	return dir, nil
}

func newSnapshotDir(root, name string) (string, error) {
	for i := 1; i <= maxSnapshotsPerInstant; i++ {
		candidate := name
		if i > 1 {
			candidate = fmt.Sprintf("%s-%d", name, i)
		}
		dir := filepath.Join(root, candidate)
		err := os.Mkdir(dir, fsutil.DirPerm)
		if err == nil {
			return dir, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("create snapshot %s: %w", dir, err)
		}
	}
	return "", fmt.Errorf("create snapshot in %s: too many snapshots at %s", root, name)
}

func saveOne(dir string, index int, path string) (Entry, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Entry{Path: path}, nil
	}
	if err != nil {
		return Entry{}, fmt.Errorf("back up %s: %w", path, err)
	}
	stored := fmt.Sprintf("%04d-%s", index+1, filepath.Base(path))
	if err := fsutil.WriteAtomic(filepath.Join(dir, stored), data, fsutil.FilePerm); err != nil {
		return Entry{}, err
	}
	return Entry{Path: path, Existed: true, Stored: stored}, nil
}

// ReadManifest loads the manifest of the snapshot in dir.
func ReadManifest(dir string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		return Manifest{}, fmt.Errorf("read backup manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parse backup manifest: %w", err)
	}
	return m, nil
}

// Prune removes the oldest snapshots under root so that at most keep remain.
// Snapshot names sort chronologically; entries that are not directories are
// left alone.
func Prune(root string, keep int) error {
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list backups: %w", err)
	}
	var snapshots []string
	for _, e := range entries {
		if e.IsDir() {
			snapshots = append(snapshots, e.Name())
		}
	}
	for len(snapshots) > keep {
		if err := os.RemoveAll(filepath.Join(root, snapshots[0])); err != nil {
			return fmt.Errorf("remove old backup: %w", err)
		}
		snapshots = snapshots[1:]
	}
	return nil
}
