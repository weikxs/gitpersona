package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const uiLockName = "ui.lock"

// lockUI makes sure only one configuration page runs, so two pages never edit
// the configuration at the same time. A lock left by a process that is gone
// is taken over. The returned function releases the lock.
func lockUI(configDir string) (func(), error) {
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(configDir, uiLockName)
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			// The process id only helps the user find a stale lock.
			_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
			_ = f.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if pid := lockOwner(path); pid > 0 && processRuns(pid) {
			return nil, fmt.Errorf("the configuration page is already open (process %d); use that page, "+
				"or close it with Close page or Ctrl-C first. If that tab or terminal is gone, stop it with `kill %d`", pid, pid)
		}
		_ = os.Remove(path) // left by a page that did not stop cleanly
	}
	return nil, errors.New("could not take over the configuration page lock; try again")
}

func lockOwner(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return pid
}

// processRuns reports whether a process exists; signal 0 checks without
// sending anything. EPERM means it exists but belongs to someone else.
func processRuns(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
