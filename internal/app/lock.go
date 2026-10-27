package app

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/weikxs/gitpersona/internal/fsutil"
	"github.com/weikxs/gitpersona/internal/shell"
)

const (
	// lockFile, in the configuration folder, is locked while a command
	// reads, plans and writes a change, so the page and the command line (or
	// two requests of the page) never overwrite each other's edits.
	lockFile = ".lock"
	// defaultLockWait is how long a change waits for another to finish.
	defaultLockWait = 30 * time.Second
	// noLockWait makes lockConfig give up at once (tests).
	noLockWait = -1
	lockPoll   = 50 * time.Millisecond
)

// lockConfig takes the configuration lock and returns e marked as holding
// it, with the function that releases it. A command that already holds it
// (rollback runs apply, say) does not take it again.
func (e Env) lockConfig() (Env, func(), error) {
	if e.configLocked {
		return e, func() {}, nil
	}
	if err := os.MkdirAll(e.ConfigDir, fsutil.DirPerm); err != nil {
		return e, nil, e.trErr("create %s: %w", e.display(e.ConfigDir), err)
	}
	path := filepath.Join(e.ConfigDir, lockFile)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, fsutil.FilePerm)
	if err != nil {
		return e, nil, e.trErr("open the lock %s: %w", e.display(path), err)
	}
	wait := e.lockWait
	if wait == 0 {
		wait = defaultLockWait
	}
	deadline := time.Now().Add(wait)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		retry := errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EINTR)
		if !retry || !time.Now().Before(deadline) {
			_ = f.Close()
			if errors.Is(err, syscall.EWOULDBLOCK) {
				return e, nil, e.trErr("another gitpersona command (or the configuration page) is changing the configuration; "+
					"try again when it has finished (lock: %s)", e.display(path))
			}
			return e, nil, e.trErr("lock %s: %w", e.display(path), err)
		}
		time.Sleep(lockPoll)
	}
	held := e
	held.configLocked = true
	// Closing the file releases the lock; so does the process ending. Before
	// that, the generation changes, so shell prompts ask again (see
	// shell.GenerationFile).
	return held, func() {
		e.bumpGeneration()
		_ = f.Close()
	}, nil
}

// bumpGeneration writes a new value to the generation file, which the shell
// prompt functions compare to decide whether their last answer still holds.
// A failure only costs prompts their cache until the next change.
func (e Env) bumpGeneration() {
	value := strconv.FormatInt(time.Now().UnixNano(), 10) + "\n"
	_ = fsutil.WriteAtomic(filepath.Join(e.ConfigDir, shell.GenerationFile), []byte(value), fsutil.FilePerm)
}
