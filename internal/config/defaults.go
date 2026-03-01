package config

import (
	"fmt"
	"os"
	"runtime"
)

// DefaultInstallDir returns the platform-appropriate default Qt install directory.
func DefaultInstallDir() string {
	switch runtime.GOOS {
	case "windows":
		return `C:\Qt`
	default: // linux, darwin
		home, err := homeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %v; using /Qt as default install dir\n", err)
			return "/Qt"
		}
		return home + "/Qt"
	}
}

// DefaultConcurrency returns the default number of parallel downloads.
func DefaultConcurrency() int {
	return 4
}

// DefaultTimeoutSeconds returns the per-request HTTP timeout in seconds.
func DefaultTimeoutSeconds() int {
	return 300
}

// DefaultRepositoryURL is the primary Qt mirror base URL.
// It is the mirror root without any repository path; online/qtsdkrepository/ is appended internally.
const DefaultRepositoryURL = "https://download.qt.io/"
