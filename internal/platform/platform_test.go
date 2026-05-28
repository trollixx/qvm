package platform

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultArchForHost(t *testing.T) {
	tests := []struct {
		name      string
		host      string
		qtVersion string
		want      string
	}{
		// Qt 6.0-6.5 gets msvc2019.
		{"qt6.2_windows_x86", "windows_x86", "6.2.0", "win64_msvc2019_64"},
		{"qt6.5_windows_x86", "windows_x86", "6.5.3", "win64_msvc2019_64"},

		// Qt 6.6+ gets msvc2022.
		{"qt6.6_windows_x86", "windows_x86", "6.6.0", "win64_msvc2022_64"},
		{"qt6.8_windows_x86", "windows_x86", "6.8.3", "win64_msvc2022_64"},
		{"qt6.10_windows_x86", "windows_x86", "6.10.0", "win64_msvc2022_64"},

		// ARM64 always msvc2022 (only available from Qt 6.8+).
		{"windows_arm64", "windows_arm64", "6.8.3", "win64_msvc2022_arm64"},

		// Non-Windows hosts are version-independent.
		{"linux_x64", "linux_x64", "6.8.3", "gcc_64"},
		{"linux_arm64", "linux_arm64", "6.8.3", "linux_gcc_arm64"},
		{"mac_x64", "mac_x64", "6.8.3", "macos"},

		// Invalid version falls back to msvc2019.
		{"bad_version", "windows_x86", "bad", "win64_msvc2019_64"},
		{"empty_version", "windows_x86", "", "win64_msvc2019_64"},

		// Unknown host.
		{"unknown_host", "freebsd_x64", "6.8.3", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, DefaultArchForHost(tc.host, tc.qtVersion))
		})
	}
}

// TestCurrentDefaultArchMatchesHost exercises the [runtime.GOARCH] auto-detection
// path: Current().DefaultArch must agree with the explicit --host mapping for
// the host equivalent to this machine. It is only meaningful when run natively
// on each arch, so it relies on the arm64 CI runners to cover the arm64 branch.
func TestCurrentDefaultArchMatchesHost(t *testing.T) {
	// Qt 6.8+ so both Windows MSVC paths resolve to 2022 without probing for
	// an installed toolchain, and arm64 builds exist.
	const version = "6.8.3"

	var host string
	switch runtime.GOOS {
	case "windows":
		host = "windows_x86"
		if runtime.GOARCH == "arm64" {
			host = "windows_arm64"
		}
	case "linux":
		host = "linux_x64"
		if runtime.GOARCH == "arm64" {
			host = "linux_arm64"
		}
	case "darwin":
		host = "mac_x64"
	default:
		t.Skipf("unsupported GOOS %q", runtime.GOOS)
	}

	want := DefaultArchForHost(host, version)
	got := Current().DefaultArch(version)
	assert.Equalf(t, want, got,
		"auto-detected arch for %s/%s should match --host %s", runtime.GOOS, runtime.GOARCH, host)
}
