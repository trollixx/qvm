package platform

import (
	"runtime"
)

// Current returns the Platform implementation for the current OS.
func Current() Platform {
	switch runtime.GOOS {
	case "windows":
		return &Windows{}
	case "darwin":
		return &Darwin{}
	default:
		return &Linux{}
	}
}

// GOOS returns the current operating system name (mirrors [runtime.GOOS]).
func GOOS() string {
	return runtime.GOOS
}

// GOARCH returns the current architecture (mirrors [runtime.GOARCH]).
func GOARCH() string {
	return runtime.GOARCH
}
