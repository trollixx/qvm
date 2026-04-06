package platform

import (
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
