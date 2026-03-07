package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	modkernel32      = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = modkernel32.NewProc("LockFileEx")
	procUnlockFileEx = modkernel32.NewProc("UnlockFileEx")
)

const lockfileExclusiveLock = 0x00000002

// lockFile acquires an exclusive advisory lock on a .lock file next to path.
// Returns an unlock function that must be called to release the lock.
func lockFile(path string) (func(), error) {
	lockPath := filepath.Clean(path) + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("creating lock dir: %w", err)
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}

	ol := new(syscall.Overlapped)
	r1, _, e1 := syscall.SyscallN(
		procLockFileEx.Addr(),
		f.Fd(),
		uintptr(lockfileExclusiveLock),
		0,
		1, 0,
		uintptr(unsafe.Pointer(ol)),
	)
	if r1 == 0 {
		f.Close()
		return nil, fmt.Errorf("LockFileEx: %v", e1)
	}

	return func() {
		ol2 := new(syscall.Overlapped)
		syscall.SyscallN(
			procUnlockFileEx.Addr(),
			f.Fd(),
			0,
			1, 0,
			uintptr(unsafe.Pointer(ol2)),
		)
		f.Close()
	}, nil
}
