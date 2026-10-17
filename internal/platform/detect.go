package platform

import (
	"debug/pe"
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

// NativeArch returns this machine's architecture as a GOARCH value, such as
// "amd64" or "arm64". On Windows it can differ from qvm's own
// [runtime.GOARCH]: Windows on ARM runs the x64 build under emulation.
func NativeArch() string {
	return nativeArch()
}

// machineArch returns the GOARCH value for a PE machine type, or "" for one
// Qt has no builds for.
func machineArch(machine uint16) string {
	switch machine {
	case pe.IMAGE_FILE_MACHINE_AMD64:
		return "amd64"
	case pe.IMAGE_FILE_MACHINE_ARM64:
		return "arm64"
	case pe.IMAGE_FILE_MACHINE_I386:
		return "386"
	}
	return ""
}
