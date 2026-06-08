package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trollixx/qvm/internal/config"
	"github.com/trollixx/qvm/internal/storage"
)

func testRegistry(versions ...string) *storage.Registry {
	reg := &storage.Registry{}
	for _, v := range versions {
		reg.Qt = append(reg.Qt, storage.InstalledQt{
			Version: v, Arch: "win64_msvc2022_64", InstallDir: `C:\Qt\` + v,
		})
	}
	return reg
}

func TestSetDefaultQt_Installed(t *testing.T) {
	cfg := &config.Config{}
	err := setDefaultQt(testRegistry("6.10.2", "6.11.1"), cfg, "6.11.1")
	require.NoError(t, err)
	assert.Equal(t, "6.11.1", cfg.Qt.Default)
}

func TestSetDefaultQt_NotInstalled(t *testing.T) {
	cfg := &config.Config{}
	err := setDefaultQt(testRegistry("6.10.2"), cfg, "6.8.3")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not installed")
	assert.Empty(t, cfg.Qt.Default)
}

func TestConfigKey_QtDefault(t *testing.T) {
	cfg := &config.Config{}
	require.NoError(t, configSet(cfg, "qt.default", "6.11.1"))
	val, err := configGet(cfg, "qt.default")
	require.NoError(t, err)
	assert.Equal(t, "6.11.1", val)
}

func TestPrintInstalledQt_MarksDefault(t *testing.T) {
	a, out := newTestApp()
	a.printInstalledQt(testRegistry("6.10.2", "6.11.1"), "6.11.1")

	assert.Contains(t, out.String(), "* 6.11.1", "default version row must be starred")
	assert.Contains(t, out.String(), "  6.10.2", "non-default row must not be starred")
	assert.NotContains(t, out.String(), "* 6.10.2")
	assert.Contains(t, out.String(), "* default version")
}

func TestPrintInstalledQt_NoDefaultNoLegend(t *testing.T) {
	a, out := newTestApp()
	a.printInstalledQt(testRegistry("6.10.2"), "")
	assert.NotContains(t, out.String(), "* default version")
}

func TestPrintInstalledQt_UninstalledDefaultNoLegend(t *testing.T) {
	// The default version can stop being installed (qvm uninstall after
	// qvm use); the legend must not print when no row is starred.
	a, out := newTestApp()
	a.printInstalledQt(testRegistry("6.10.2"), "6.11.1")
	assert.NotContains(t, out.String(), "* default version")
}
