package storage

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// LockFile acquires an exclusive advisory lock on a .lock file next to path.
// Blocks until the lock can be acquired. Returns an unlock function that must
// be called to release the lock. On Windows the .lock file is left behind:
// Windows cannot delete a held lock, and a stale lock file is harmless.
func LockFile(path string) (func(), error) {
	return lockFile(path)
}

// lockFile is the internal implementation of LockFile.
func lockFile(path string) (func(), error) {
	lockPath := filepath.Clean(path) + ".lock"
	err := os.MkdirAll(filepath.Dir(lockPath), 0o750)
	if err != nil {
		return nil, fmt.Errorf("creating lock dir: %w", err)
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}

	ol := new(windows.Overlapped)
	err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, ol)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("LockFileEx: %w", err)
	}

	return func() {
		ol2 := new(windows.Overlapped)
		_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol2)
		_ = f.Close()
	}, nil
}
