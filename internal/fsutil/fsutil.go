// Package fsutil writes files safely: atomically and with private permissions.
package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// Permissions for everything GitPersona creates.
const (
	FilePerm os.FileMode = 0o600
	DirPerm  os.FileMode = 0o700
)

// WriteAtomic writes data to path through a temporary file in the same
// directory and a rename, so readers never see a partially written file.
// Missing parent directories are created with DirPerm.
func WriteAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, DirPerm); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup; it fails harmlessly after a successful rename.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close() // the write error is the one worth reporting
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close() // the chmod error is the one worth reporting
		return fmt.Errorf("set permissions on %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
