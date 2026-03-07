package cli

import "golang.org/x/sys/windows"

// diskSpace returns (freeBytes, totalBytes, error) for the filesystem containing path.
func diskSpace(path string) (uint64, uint64, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}

	var freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes uint64
	err = windows.GetDiskFreeSpaceEx(
		pathPtr,
		&freeBytesAvailable,
		&totalNumberOfBytes,
		&totalNumberOfFreeBytes,
	)
	if err != nil {
		return 0, 0, err
	}

	return freeBytesAvailable, totalNumberOfBytes, nil
}
