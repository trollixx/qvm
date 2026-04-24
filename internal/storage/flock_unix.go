//go:build !windows

package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// lockFile acquires an exclusive advisory lock on a .lock file next to path.
// Returns an unlock function that must be called to release the lock.
func lockFile(path string) (func(), error) {
	lockPath := filepath.Clean(path) + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o750); err != nil {
		return nil, fmt.Errorf("creating lock dir: %w", err)
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}

	//nolint:gosec // f.Fd() is a valid file descriptor that always fits in int
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("flock: %w", err)
	}

	return func() {
		//nolint:gosec // f.Fd() is a valid file descriptor that always fits in int
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
