package platform

import "github.com/trollixx/qvm/pkg/qtmeta"

// qt66 is the version threshold where Qt switched to MSVC 2022.
var qt66 = qtmeta.MustParseVersion("6.6.0")

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
	switch host {
	case "windows_x86":
		return windowsDefaultMSVC(qtVersion)
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

// windowsDefaultMSVC returns the default MSVC arch for the given Qt version.
// Qt 6.6+ requires MSVC 2022; older versions use MSVC 2019.
func windowsDefaultMSVC(qtVersion string) string {
	v, err := qtmeta.ParseVersion(qtVersion)
	if err != nil {
		return "win64_msvc2019_64"
	}
	if v.GTE(qt66) {
		return "win64_msvc2022_64"
	}
	return "win64_msvc2019_64"
}
