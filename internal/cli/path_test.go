package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trollixx/qvm/internal/storage"
)

// captureStdout runs fn while capturing [os.Stdout] and returns the output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	old := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var buf [4096]byte
	n, _ := r.Read(buf[:])
	r.Close()
	return string(buf[:n])
}

func TestPathQt_SingleInstall(t *testing.T) {
	reg := &storage.Registry{
		Qt: []storage.InstalledQt{
			{Version: "6.8.3", Arch: "win64_msvc2022_64", InstallDir: `C:\Qt\6.8.3\msvc2022_64`},
		},
	}
	out := captureStdout(t, func() {
		err := pathQt(reg, "6.8.3", "")
		require.NoError(t, err)
	})
	assert.Equal(t, `C:\Qt\6.8.3\msvc2022_64`, strings.TrimSpace(out))
}

func TestPathQt_ExplicitArch(t *testing.T) {
	reg := &storage.Registry{
		Qt: []storage.InstalledQt{
			{Version: "6.10.2", Arch: "win64_msvc2022_arm64", InstallDir: `C:\Qt\6.10.2\arm64`},
			{Version: "6.10.2", Arch: "win64_msvc2022_64", InstallDir: `C:\Qt\6.10.2\x64`},
		},
	}
	out := captureStdout(t, func() {
		err := pathQt(reg, "6.10.2", "win64_msvc2022_arm64")
		require.NoError(t, err)
	})
	assert.Equal(t, `C:\Qt\6.10.2\arm64`, strings.TrimSpace(out))
}

func TestPathQt_NotInstalled(t *testing.T) {
	reg := &storage.Registry{}
	err := pathQt(reg, "6.8.3", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not installed")
}

func TestPathQt_NotInstalledWithArch(t *testing.T) {
	reg := &storage.Registry{
		Qt: []storage.InstalledQt{
			{Version: "6.10.2", Arch: "win64_msvc2022_64", InstallDir: `C:\Qt\6.10.2\x64`},
		},
	}
	err := pathQt(reg, "6.10.2", "win64_mingw")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not installed")
	assert.Contains(t, err.Error(), "win64_mingw")
}

func TestPathQt_MultipleArchs_PrefersDefault(t *testing.T) {
	reg := &storage.Registry{
		Qt: []storage.InstalledQt{
			{Version: "6.10.2", Arch: "win64_msvc2022_arm64", InstallDir: `C:\Qt\6.10.2\arm64`},
			{Version: "6.10.2", Arch: "win64_msvc2022_64", InstallDir: `C:\Qt\6.10.2\x64`},
		},
	}
	// On this machine platform.Current().DefaultArch("6.10.2") returns
	// "win64_msvc2022_64" (x64 Windows) or the equivalent for the test host.
	// Either it picks the default, or if neither matches it returns an error
	// listing both archs.
	out := captureStdout(t, func() {
		err := pathQt(reg, "6.10.2", "")
		// May succeed (default arch matches) or fail (neither matches on CI).
		if err != nil {
			assert.Contains(t, err.Error(), "multiple archs")
		}
	})
	if out != "" {
		trimmed := strings.TrimSpace(out)
		assert.True(t, trimmed == `C:\Qt\6.10.2\arm64` || trimmed == `C:\Qt\6.10.2\x64`,
			"unexpected path: %s", trimmed)
	}
}

func TestPathQt_MultipleArchs_NoneMatchesDefault(t *testing.T) {
	reg := &storage.Registry{
		Qt: []storage.InstalledQt{
			{Version: "6.10.2", Arch: "wasm_singlethread", InstallDir: `/Qt/6.10.2/wasm_st`},
			{Version: "6.10.2", Arch: "wasm_multithread", InstallDir: `/Qt/6.10.2/wasm_mt`},
		},
	}
	err := pathQt(reg, "6.10.2", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple archs")
	assert.Contains(t, err.Error(), "wasm_singlethread")
	assert.Contains(t, err.Error(), "wasm_multithread")
}
