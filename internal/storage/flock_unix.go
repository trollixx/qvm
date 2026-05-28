//go:build !windows

package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// LockFile acquires an exclusive advisory lock on a .lock file next to path.
// Blocks until the lock can be acquired. Returns an unlock function that must
// be called to release the lock.
//
// On Unix the .lock file is removed at unlock time on a best-effort basis.
func LockFile(path string) (func(), error) {
	return lockFile(path)
}

// lockFile is the internal implementation of LockFile.
func lockFile(path string) (func(), error) {
	lockPath := filepath.Clean(path) + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o750); err != nil {
		return nil, fmt.Errorf("creating lock dir: %w", err)
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}

	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("flock: %w", err)
	}

	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
		// Best-effort cleanup. If another process holds (or is racing for) the
		// lock, the unlink either succeeds and the next lockFile recreates the
		// file, or fails harmlessly.
		_ = os.Remove(lockPath)
	}, nil
}
