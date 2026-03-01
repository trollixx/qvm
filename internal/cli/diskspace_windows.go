package cli

import (
	"syscall"
	"unsafe"
)

var getDiskFreeSpaceEx = syscall.MustLoadDLL("kernel32.dll").MustFindProc("GetDiskFreeSpaceExW")

// diskSpace returns (freeBytes, totalBytes, error) for the filesystem containing path.
func diskSpace(path string) (uint64, uint64, error) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}

	var freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes uint64

	r1, _, e1 := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalNumberOfBytes)),
		uintptr(unsafe.Pointer(&totalNumberOfFreeBytes)),
	)
	if r1 == 0 {
		return 0, 0, e1
	}

	return freeBytesAvailable, totalNumberOfBytes, nil
}
