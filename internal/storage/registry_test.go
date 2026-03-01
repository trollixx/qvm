package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestManager(t *testing.T) *RegistryManager {
	t.Helper()
	dir := t.TempDir()
	return NewRegistryManagerAt(filepath.Join(dir, "registry.json"))
}

func TestLoad_MissingFile(t *testing.T) {
	mgr := newTestManager(t)

	reg, err := mgr.Load()
	require.NoError(t, err)
	assert.Equal(t, 1, reg.Version)
	assert.Empty(t, reg.Qt)
	assert.Empty(t, reg.Tools)
}

func TestLoad_ValidJSON(t *testing.T) {
	mgr := newTestManager(t)

	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	data := Registry{
		Version: 1,
		Qt: []InstalledQt{
			{
				Version:     "6.7.0",
				Arch:        "win64_msvc2022_64",
				InstallDir:  `C:\Qt\6.7.0\msvc2022_64`,
				Modules:     []string{"qtcharts", "qtwebengine"},
				InstalledAt: now,
				SizeBytes:   1024000,
			},
		},
		Tools: []InstalledTool{
			{
				Name:        "qtcreator",
				Version:     "13.0.0",
				InstallDir:  `C:\Qt\Tools\QtCreator`,
				InstalledAt: now,
				SizeBytes:   512000,
			},
		},
	}

	raw, err := json.MarshalIndent(data, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(mgr.Path()), 0o755))
	require.NoError(t, os.WriteFile(mgr.Path(), raw, 0o644))

	reg, err := mgr.Load()
	require.NoError(t, err)
	assert.Equal(t, 1, reg.Version)
	require.Len(t, reg.Qt, 1)
	assert.Equal(t, "6.7.0", reg.Qt[0].Version)
	assert.Equal(t, "win64_msvc2022_64", reg.Qt[0].Arch)
	assert.Equal(t, []string{"qtcharts", "qtwebengine"}, reg.Qt[0].Modules)
	require.Len(t, reg.Tools, 1)
	assert.Equal(t, "qtcreator", reg.Tools[0].Name)
	assert.Equal(t, "13.0.0", reg.Tools[0].Version)
}

func TestLoad_LegacyTargetMigration(t *testing.T) {
	mgr := newTestManager(t)

	// Write JSON with legacy "target" field instead of "arch".
	legacyJSON := `{
  "version": 1,
  "qt": [
    {
      "version": "6.5.3",
      "target": "win64_mingw",
      "install_dir": "C:\\Qt\\6.5.3\\mingw",
      "installed_at": "2025-01-01T00:00:00Z"
    }
  ]
}`
	require.NoError(t, os.MkdirAll(filepath.Dir(mgr.Path()), 0o755))
	require.NoError(t, os.WriteFile(mgr.Path(), []byte(legacyJSON), 0o644))

	reg, err := mgr.Load()
	require.NoError(t, err)
	require.Len(t, reg.Qt, 1)
	assert.Equal(t, "win64_mingw", reg.Qt[0].Arch,
		"legacy 'target' field should be migrated to 'arch'")
}

func TestLoad_FutureVersion(t *testing.T) {
	mgr := newTestManager(t)

	futureJSON := `{"version": 99}`
	require.NoError(t, os.MkdirAll(filepath.Dir(mgr.Path()), 0o755))
	require.NoError(t, os.WriteFile(mgr.Path(), []byte(futureJSON), 0o644))

	_, err := mgr.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "newer qvm version")
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	mgr := newTestManager(t)

	now := time.Now().Truncate(time.Second).UTC()
	original := &Registry{
		Version: 1,
		Qt: []InstalledQt{
			{
				Version:     "6.8.0",
				Arch:        "linux_gcc_64",
				InstallDir:  "/home/user/Qt/6.8.0/gcc_64",
				Modules:     []string{"qtcharts"},
				InstalledAt: now,
				SizeBytes:   2048000,
			},
		},
		Tools: []InstalledTool{
			{
				Name:        "cmake",
				Version:     "3.28.0",
				InstallDir:  "/home/user/Qt/Tools/CMake",
				InstalledAt: now,
			},
		},
	}

	require.NoError(t, mgr.Save(original))

	loaded, err := mgr.Load()
	require.NoError(t, err)
	assert.Equal(t, original.Version, loaded.Version)
	require.Len(t, loaded.Qt, 1)
	assert.Equal(t, original.Qt[0].Version, loaded.Qt[0].Version)
	assert.Equal(t, original.Qt[0].Arch, loaded.Qt[0].Arch)
	assert.Equal(t, original.Qt[0].InstallDir, loaded.Qt[0].InstallDir)
	assert.Equal(t, original.Qt[0].Modules, loaded.Qt[0].Modules)
	assert.Equal(t, original.Qt[0].SizeBytes, loaded.Qt[0].SizeBytes)
	require.Len(t, loaded.Tools, 1)
	assert.Equal(t, original.Tools[0].Name, loaded.Tools[0].Name)
}

func TestAddQt_New(t *testing.T) {
	mgr := newTestManager(t)

	entry := InstalledQt{
		Version:     "6.7.0",
		Arch:        "win64_msvc2022_64",
		InstallDir:  `C:\Qt\6.7.0\msvc2022_64`,
		InstalledAt: time.Now().UTC(),
	}
	require.NoError(t, mgr.AddQt(entry))

	reg, err := mgr.Load()
	require.NoError(t, err)
	require.Len(t, reg.Qt, 1)
	assert.Equal(t, "6.7.0", reg.Qt[0].Version)
	assert.Equal(t, "win64_msvc2022_64", reg.Qt[0].Arch)
}

func TestAddQt_Replace(t *testing.T) {
	mgr := newTestManager(t)

	original := InstalledQt{
		Version:     "6.7.0",
		Arch:        "win64_msvc2022_64",
		InstallDir:  `C:\Qt\6.7.0\msvc2022_64`,
		Modules:     []string{"qtcharts"},
		InstalledAt: time.Now().UTC(),
		SizeBytes:   100,
	}
	require.NoError(t, mgr.AddQt(original))

	replacement := InstalledQt{
		Version:     "6.7.0",
		Arch:        "win64_msvc2022_64",
		InstallDir:  `C:\Qt\6.7.0\msvc2022_64`,
		Modules:     []string{"qtcharts", "qtwebengine"},
		InstalledAt: time.Now().UTC(),
		SizeBytes:   200,
	}
	require.NoError(t, mgr.AddQt(replacement))

	reg, err := mgr.Load()
	require.NoError(t, err)
	require.Len(t, reg.Qt, 1, "should replace, not append")
	assert.Equal(t, []string{"qtcharts", "qtwebengine"}, reg.Qt[0].Modules)
	assert.Equal(t, int64(200), reg.Qt[0].SizeBytes)
}

func TestAddQt_MultipleArchs(t *testing.T) {
	mgr := newTestManager(t)

	require.NoError(t, mgr.AddQt(InstalledQt{
		Version: "6.7.0", Arch: "win64_msvc2022_64",
		InstallDir: `C:\Qt\6.7.0\msvc2022_64`, InstalledAt: time.Now().UTC(),
	}))
	require.NoError(t, mgr.AddQt(InstalledQt{
		Version: "6.7.0", Arch: "win64_mingw",
		InstallDir: `C:\Qt\6.7.0\mingw`, InstalledAt: time.Now().UTC(),
	}))

	reg, err := mgr.Load()
	require.NoError(t, err)
	assert.Len(t, reg.Qt, 2, "same version, different arch = two entries")
}

func TestRemoveQt_ByVersionAndArch(t *testing.T) {
	mgr := newTestManager(t)

	require.NoError(t, mgr.AddQt(InstalledQt{
		Version: "6.7.0", Arch: "win64_msvc2022_64",
		InstallDir: `C:\Qt\6.7.0\msvc2022_64`, InstalledAt: time.Now().UTC(),
	}))
	require.NoError(t, mgr.AddQt(InstalledQt{
		Version: "6.7.0", Arch: "win64_mingw",
		InstallDir: `C:\Qt\6.7.0\mingw`, InstalledAt: time.Now().UTC(),
	}))

	require.NoError(t, mgr.RemoveQt("6.7.0", "win64_msvc2022_64"))

	reg, err := mgr.Load()
	require.NoError(t, err)
	require.Len(t, reg.Qt, 1)
	assert.Equal(t, "win64_mingw", reg.Qt[0].Arch)
}

func TestRemoveQt_ByVersionOnly(t *testing.T) {
	mgr := newTestManager(t)

	require.NoError(t, mgr.AddQt(InstalledQt{
		Version: "6.7.0", Arch: "win64_msvc2022_64",
		InstallDir: `C:\Qt\6.7.0\msvc2022_64`, InstalledAt: time.Now().UTC(),
	}))
	require.NoError(t, mgr.AddQt(InstalledQt{
		Version: "6.7.0", Arch: "win64_mingw",
		InstallDir: `C:\Qt\6.7.0\mingw`, InstalledAt: time.Now().UTC(),
	}))

	// Empty arch removes all entries for that version.
	require.NoError(t, mgr.RemoveQt("6.7.0", ""))

	reg, err := mgr.Load()
	require.NoError(t, err)
	assert.Empty(t, reg.Qt)
}

func TestAddTool(t *testing.T) {
	mgr := newTestManager(t)

	entry := InstalledTool{
		Name:        "qtcreator",
		Version:     "13.0.0",
		InstallDir:  `C:\Qt\Tools\QtCreator`,
		InstalledAt: time.Now().UTC(),
		SizeBytes:   512000,
	}
	require.NoError(t, mgr.AddTool(entry))

	reg, err := mgr.Load()
	require.NoError(t, err)
	require.Len(t, reg.Tools, 1)
	assert.Equal(t, "qtcreator", reg.Tools[0].Name)
	assert.Equal(t, "13.0.0", reg.Tools[0].Version)
}

func TestAddTool_Replace(t *testing.T) {
	mgr := newTestManager(t)

	require.NoError(t, mgr.AddTool(InstalledTool{
		Name: "qtcreator", Version: "13.0.0",
		InstallDir: `C:\Qt\Tools\QtCreator`, InstalledAt: time.Now().UTC(), SizeBytes: 100,
	}))
	require.NoError(t, mgr.AddTool(InstalledTool{
		Name: "qtcreator", Version: "13.0.0",
		InstallDir: `C:\Qt\Tools\QtCreator`, InstalledAt: time.Now().UTC(), SizeBytes: 200,
	}))

	reg, err := mgr.Load()
	require.NoError(t, err)
	require.Len(t, reg.Tools, 1, "same name+version should replace")
	assert.Equal(t, int64(200), reg.Tools[0].SizeBytes)
}

func TestRemoveTool(t *testing.T) {
	mgr := newTestManager(t)

	require.NoError(t, mgr.AddTool(InstalledTool{
		Name: "qtcreator", Version: "13.0.0",
		InstallDir: `C:\Qt\Tools\QtCreator`, InstalledAt: time.Now().UTC(),
	}))
	require.NoError(t, mgr.AddTool(InstalledTool{
		Name: "cmake", Version: "3.28.0",
		InstallDir: `C:\Qt\Tools\CMake`, InstalledAt: time.Now().UTC(),
	}))

	require.NoError(t, mgr.RemoveTool("qtcreator", "13.0.0"))

	reg, err := mgr.Load()
	require.NoError(t, err)
	require.Len(t, reg.Tools, 1)
	assert.Equal(t, "cmake", reg.Tools[0].Name)
}

func TestRemoveTool_ByNameOnly(t *testing.T) {
	mgr := newTestManager(t)

	require.NoError(t, mgr.AddTool(InstalledTool{
		Name: "cmake", Version: "3.27.0",
		InstallDir: `C:\Qt\Tools\CMake327`, InstalledAt: time.Now().UTC(),
	}))
	require.NoError(t, mgr.AddTool(InstalledTool{
		Name: "cmake", Version: "3.28.0",
		InstallDir: `C:\Qt\Tools\CMake328`, InstalledAt: time.Now().UTC(),
	}))

	// Empty version removes all entries for that name.
	require.NoError(t, mgr.RemoveTool("cmake", ""))

	reg, err := mgr.Load()
	require.NoError(t, err)
	assert.Empty(t, reg.Tools)
}

func TestLoad_InvalidJSON(t *testing.T) {
	mgr := newTestManager(t)

	require.NoError(t, os.MkdirAll(filepath.Dir(mgr.Path()), 0o755))
	require.NoError(t, os.WriteFile(mgr.Path(), []byte(`{invalid`), 0o644))

	_, err := mgr.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing registry")
}
