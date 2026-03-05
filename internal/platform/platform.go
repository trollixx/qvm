package platform

// Platform provides platform-specific information and defaults.
type Platform interface {
	// DefaultInstallDir returns the default Qt install directory.
	DefaultInstallDir() string
	// DefaultArch returns the recommended Qt arch for this machine.
	DefaultArch(qtVersion string) string
	// CheckCompilerPresent reports whether the expected compiler for arch is available.
	CheckCompilerPresent(arch string) (bool, string)
}

// DefaultArchForHost returns the default Qt arch for a given host platform
// identifier (e.g. "windows_arm64"). This is used when --host overrides
// auto-detection so the default arch matches the target host, not the
// local machine. Returns "" if the host is unrecognized.
func DefaultArchForHost(host, qtVersion string) string {
	isQt6 := len(qtVersion) > 0 && qtVersion[0] == '6'
	switch host {
	case "windows_x86":
		if isQt6 {
			return "win64_msvc2022_64"
		}
		return "win64_msvc2019_64"
	case "windows_arm64":
		return "win64_msvc2022_arm64"
	case "linux_x64":
		return "gcc_64"
	case "linux_arm64":
		return "linux_gcc_arm64"
	case "mac_x64":
		return "macos"
	default:
		return ""
	}
}
