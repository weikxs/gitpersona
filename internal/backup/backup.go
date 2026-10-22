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
	Command string    `json:"command,omitempty"` // the gitpersona command that made it
	Entries []Entry   `json:"entries"`
}

// Entry records one file as it was before the change.
type Entry struct {
	Path    string `json:"path"`             // absolute path of the original file
	Existed bool   `json:"existed"`          // false: the change created the file
	Stored  string `json:"stored,omitempty"` // copy inside the snapshot, if Existed
	Mode    uint32 `json:"mode,omitempty"`   // permission bits, if Existed
	Link    string `json:"link,omitempty"`   // symlink target, if the path was a symlink
}

// maxSnapshotsPerInstant bounds the suffixes tried when timestamps collide.
const maxSnapshotsPerInstant = 100

// Snapshot copies files into a new directory under root named after now and
// returns that directory. An existing snapshot is never reused: when the name
// is taken, a numeric suffix is added.
func Snapshot(root string, now time.Time, files []string) (string, error) {
	return SnapshotFor(root, now, "", files)
}

// SnapshotFor is Snapshot with the command that makes the change recorded.
func SnapshotFor(root string, now time.Time, command string, files []string) (string, error) {
	if err := os.MkdirAll(root, fsutil.DirPerm); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}
	dir, err := newSnapshotDir(root, now.UTC().Format(timestampLayout))
	if err != nil {
		return "", err
	}

	manifest := Manifest{Created: now, Command: command, Entries: make([]Entry, 0, len(files))}
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
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return Entry{}, fmt.Errorf("back up %s: %w", path, err)
		}
		return Entry{Path: path, Existed: true, Link: target}, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Entry{Path: path}, nil
	}
	if err != nil {
		return Entry{}, fmt.Errorf("back up %s: %w", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return Entry{}, fmt.Errorf("back up %s: %w", path, err)
	}
	stored := fmt.Sprintf("%04d-%s", index+1, filepath.Base(path))
	if err := fsutil.WriteAtomic(filepath.Join(dir, stored), data, fsutil.FilePerm); err != nil {
		return Entry{}, err
	}
	return Entry{Path: path, Existed: true, Stored: stored, Mode: uint32(info.Mode().Perm())}, nil
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

// Prune removes old snapshots under root so that the keep newest remain, plus
// the oldest one, which holds the state from before GitPersona changed
// anything. Snapshot names sort chronologically; entries that are not
// directories are left alone.
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
	for len(snapshots) > keep+1 {
		if err := os.RemoveAll(filepath.Join(root, snapshots[1])); err != nil {
			return fmt.Errorf("remove old backup: %w", err)
		}
		snapshots = append(snapshots[:1], snapshots[2:]...)
	}
	return nil
}

// Info describes one snapshot for listing.
type Info struct {
	Dir      string
	Manifest Manifest
}

// List returns the snapshots under root, newest first. Directories without a
// readable manifest are skipped.
func List(root string) ([]Info, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	var infos []Info
	for i := len(entries) - 1; i >= 0; i-- {
		if !entries[i].IsDir() {
			continue
		}
		dir := filepath.Join(root, entries[i].Name())
		if m, err := ReadManifest(dir); err == nil {
			infos = append(infos, Info{Dir: dir, Manifest: m})
		}
	}
	return infos, nil
}

// Restore puts every file of the snapshot back: files that existed get their
// content and mode back, and files that did not exist are removed.
func Restore(dir string) error {
	m, err := ReadManifest(dir)
	if err != nil {
		return err
	}
	for _, e := range m.Entries {
		if !e.Existed || e.Link != "" {
			if err := os.Remove(e.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("remove %s: %w", e.Path, err)
			}
		}
		if !e.Existed {
			continue
		}
		if e.Link != "" {
			if err := os.Symlink(e.Link, e.Path); err != nil {
				return fmt.Errorf("restore link %s: %w", e.Path, err)
			}
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Stored))
		if err != nil {
			return fmt.Errorf("read backup of %s: %w", e.Path, err)
		}
		mode := fs.FileMode(e.Mode)
		if mode == 0 {
			mode = fsutil.FilePerm
		}
		if err := fsutil.WriteAtomic(e.Path, data, mode); err != nil {
			return err
		}
	}
	return nil
}
