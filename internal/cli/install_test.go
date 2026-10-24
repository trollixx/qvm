package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/trollixx/qvm/internal/platform"
)

// --host picks that host's default, as for install and list-remote <version>.
func TestDefaultArch(t *testing.T) {
	assert.Equal(t, "win64_msvc2022_arm64", defaultArch("6.8.3", "windows_arm64"))
	assert.Equal(t, "win64_msvc2019_64", defaultArch("6.7.3", "windows_x86"))
	assert.Equal(t, "clang_64", defaultArch("6.8.3", "mac_x64"))

	// Without --host, or with one that has no default, it's this machine's.
	assert.Equal(t, platform.Current().DefaultArch("6.8.3"), defaultArch("6.8.3", ""))
	assert.Equal(t, platform.Current().DefaultArch("6.8.3"), defaultArch("6.8.3", "all_os"))
}
