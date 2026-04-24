//go:build !windows

package cli

import (
	"syscall"
)

// diskSpace returns (freeBytes, totalBytes, error) for the filesystem containing path.
func diskSpace(path string) (uint64, uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	// stat.Bsize may be int32 (Darwin) or int64 (Linux); the kernel-provided
	// block size is always non-negative, so the conversion is safe.
	bsize := uint64(stat.Bsize) //nolint:gosec // kernel block size is non-negative
	free := stat.Bavail * bsize
	total := stat.Blocks * bsize
	return free, total, nil
}
