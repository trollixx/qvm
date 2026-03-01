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
	// stat.Bsize may be int32 (Darwin) or int64 (Linux); cast safely.
	bsize := uint64(stat.Bsize)
	free := stat.Bavail * bsize
	total := stat.Blocks * bsize
	return free, total, nil
}
