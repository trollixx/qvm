package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testVersionInfo builds a QtVersionInfo with the given addon packages registered.
func testVersionInfo(arch string, addonModules ...string) *QtVersionInfo {
	vi := &QtVersionInfo{
		Version: "6.10.2",
		Major:   6,
		Archs:   []Arch{{Name: arch}},
		PackageArchives: map[string][]ArchiveRef{
			"qt.qt6.6102." + arch: {{URL: "http://x/qtbase.7z", Filename: "qtbase.7z"}},
		},
	}
	for _, mod := range addonModules {
		pkg := "qt.qt6.6102.addons." + mod + "." + arch
		vi.PackageArchives[pkg] = []ArchiveRef{{URL: "http://x/" + mod + ".7z", Filename: mod + ".7z"}}
		vi.Modules = append(vi.Modules, Module{Name: mod})
	}
	return vi
}

func TestResolveArchives_ExactModuleName(t *testing.T) {
	vi := testVersionInfo("win64_msvc2022_64", "qtcharts", "qtwebengine")
	archives, err := resolveArchives(vi, ResolveOptions{
		Version: "6.10.2",
		Arch:    "win64_msvc2022_64",
		Modules: []string{"qtcharts"},
	})
	require.NoError(t, err)

	var names []string
	for _, a := range archives {
		names = append(names, a.Name)
	}
	assert.Contains(t, names, "qt.qt6.6102.addons.qtcharts.win64_msvc2022_64")
}

func TestResolveArchives_AutoPrefixQt(t *testing.T) {
	vi := testVersionInfo("win64_msvc2022_64", "qtcharts", "qtwebengine", "qtimageformats")
	archives, err := resolveArchives(vi, ResolveOptions{
		Version: "6.10.2",
		Arch:    "win64_msvc2022_64",
		Modules: []string{"charts", "webengine", "imageformats"},
	})
	require.NoError(t, err)

	var names []string
	for _, a := range archives {
		names = append(names, a.Name)
	}
	assert.Contains(t, names, "qt.qt6.6102.addons.qtcharts.win64_msvc2022_64")
	assert.Contains(t, names, "qt.qt6.6102.addons.qtwebengine.win64_msvc2022_64")
	assert.Contains(t, names, "qt.qt6.6102.addons.qtimageformats.win64_msvc2022_64")
}

func TestResolveArchives_MixedPrefixed(t *testing.T) {
	vi := testVersionInfo("win64_msvc2022_64", "qtcharts", "qthttpserver")
	archives, err := resolveArchives(vi, ResolveOptions{
		Version: "6.10.2",
		Arch:    "win64_msvc2022_64",
		Modules: []string{"charts", "qthttpserver"},
	})
	require.NoError(t, err)

	var names []string
	for _, a := range archives {
		names = append(names, a.Name)
	}
	assert.Contains(t, names, "qt.qt6.6102.addons.qtcharts.win64_msvc2022_64")
	assert.Contains(t, names, "qt.qt6.6102.addons.qthttpserver.win64_msvc2022_64")
}

func TestResolveArchives_UnknownModule_Error(t *testing.T) {
	vi := testVersionInfo("win64_msvc2022_64", "qtcharts")
	_, err := resolveArchives(vi, ResolveOptions{
		Version: "6.10.2",
		Arch:    "win64_msvc2022_64",
		Modules: []string{"nonexistent"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
	assert.Contains(t, err.Error(), "is not known")
}

func TestResolveArchives_MultipleUnknown_FailsOnFirst(t *testing.T) {
	vi := testVersionInfo("win64_msvc2022_64", "qtcharts")
	_, err := resolveArchives(vi, ResolveOptions{
		Version: "6.10.2",
		Arch:    "win64_msvc2022_64",
		Modules: []string{"foo", "bar"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "foo")
	assert.Contains(t, err.Error(), "is not known")
}

func TestResolveArchives_NoPrefixForAlreadyPrefixed(t *testing.T) {
	// "qtcharts" is already prefixed — should not try "qtqtcharts".
	vi := testVersionInfo("win64_msvc2022_64", "qtcharts")
	archives, err := resolveArchives(vi, ResolveOptions{
		Version: "6.10.2",
		Arch:    "win64_msvc2022_64",
		Modules: []string{"qtcharts"},
	})
	require.NoError(t, err)
	assert.Len(t, archives, 2) // essentials + qtcharts
}

// testVersionInfoQt5 builds a Qt 5 QtVersionInfo with the given addon packages
// using the Qt 5 naming scheme (no "addons" segment).
func testVersionInfoQt5(arch string, addonModules ...string) *QtVersionInfo {
	vi := &QtVersionInfo{
		Version: "5.15.2",
		Major:   5,
		Archs:   []Arch{{Name: arch}},
		PackageArchives: map[string][]ArchiveRef{
			"qt.qt5.5152." + arch: {{URL: "http://x/qtbase.7z", Filename: "qtbase.7z"}},
		},
	}
	for _, mod := range addonModules {
		pkg := "qt.qt5.5152." + mod + "." + arch
		vi.PackageArchives[pkg] = []ArchiveRef{{URL: "http://x/" + mod + ".7z", Filename: mod + ".7z"}}
		vi.Modules = append(vi.Modules, Module{Name: mod})
	}
	return vi
}

func TestResolveArchives_Qt5_ExactModuleName(t *testing.T) {
	vi := testVersionInfoQt5("win64_msvc2019_64", "qtcharts", "qtwebengine")
	archives, err := resolveArchives(vi, ResolveOptions{
		Version: "5.15.2",
		Arch:    "win64_msvc2019_64",
		Modules: []string{"qtwebengine"},
	})
	require.NoError(t, err)

	var names []string
	for _, a := range archives {
		names = append(names, a.Name)
	}
	assert.Contains(t, names, "qt.qt5.5152.qtwebengine.win64_msvc2019_64")
}

func TestResolveArchives_Qt5_AutoPrefixQt(t *testing.T) {
	vi := testVersionInfoQt5("win64_msvc2019_64", "qtcharts", "qtwebengine")
	archives, err := resolveArchives(vi, ResolveOptions{
		Version: "5.15.2",
		Arch:    "win64_msvc2019_64",
		Modules: []string{"webengine", "charts"},
	})
	require.NoError(t, err)

	var names []string
	for _, a := range archives {
		names = append(names, a.Name)
	}
	assert.Contains(t, names, "qt.qt5.5152.qtwebengine.win64_msvc2019_64")
	assert.Contains(t, names, "qt.qt5.5152.qtcharts.win64_msvc2019_64")
}

func TestResolveArchives_EssentialModuleSkipped(t *testing.T) {
	// "qtimageformats" is an essential module — requesting it should not error.
	vi := testVersionInfoQt5("win64_msvc2019_64", "qtcharts")
	vi.Archs[0].EssentialModules = []string{"qtbase", "qtimageformats", "qtwebchannel"}

	archives, err := resolveArchives(vi, ResolveOptions{
		Version: "5.15.2",
		Arch:    "win64_msvc2019_64",
		Modules: []string{"imageformats", "charts", "webchannel"},
	})
	require.NoError(t, err)

	var names []string
	for _, a := range archives {
		names = append(names, a.Name)
	}
	// qtcharts should be resolved as an addon.
	assert.Contains(t, names, "qt.qt5.5152.qtcharts.win64_msvc2019_64")
	// imageformats and webchannel are essentials — skipped, no error.
	assert.NotContains(t, names, "qtimageformats")
	assert.NotContains(t, names, "qtwebchannel")
}

func TestArchiveModuleName(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"6.10.2-0-202601261212qtbase-Windows-debug-symbols.7z", "qtbase"},
		{"6.10.2-0-202601261212qtimageformats-Windows-debug-symbols.7z", "qtimageformats"},
		{"6.10.2-0-202601261212qt3d-Windows-debug-symbols.7z", "qt3d"},
		{"qtbase-Windows-msvc.7z", "qtbase"},
		{"", ""},
	}
	for _, tc := range tests {
		t.Run(tc.filename, func(t *testing.T) {
			assert.Equal(t, tc.want, archiveModuleName(tc.filename))
		})
	}
}

func TestResolveArchives_DebugInfoScopedToModules(t *testing.T) {
	arch := "win64_msvc2022_64"
	vi := testVersionInfo(arch, "qtcharts", "qtimageformats", "qt3d")
	// Add a debug_info package with archives for all modules.
	debugPkg := "qt.qt6.6102.debug_info." + arch
	vi.PackageArchives[debugPkg] = []ArchiveRef{
		{URL: "http://x/qtbase-dbg.7z", Filename: "6.10.2-0-202503010000qtbase-Windows-debug-symbols.7z"},
		{URL: "http://x/qtcharts-dbg.7z", Filename: "6.10.2-0-202503010000qtcharts-Windows-debug-symbols.7z"},
		{URL: "http://x/qtimageformats-dbg.7z", Filename: "6.10.2-0-202503010000qtimageformats-Windows-debug-symbols.7z"},
		{URL: "http://x/qt3d-dbg.7z", Filename: "6.10.2-0-202503010000qt3d-Windows-debug-symbols.7z"},
		{URL: "http://x/qtdeclarative-dbg.7z", Filename: "6.10.2-0-202503010000qtdeclarative-Windows-debug-symbols.7z"},
	}
	vi.Archs[0].EssentialModules = []string{"qtbase", "qtdeclarative"}

	archives, err := resolveArchives(vi, ResolveOptions{
		Version:        "6.10.2",
		Arch:           arch,
		Modules:        []string{"qtcharts", "qtimageformats"},
		SkipEssentials: true,
		DebugInfo:      true,
	})
	require.NoError(t, err)

	// Should include: qtcharts addon + qtimageformats addon + debug for
	// essentials (qtbase, qtdeclarative) + requested addons (qtcharts, qtimageformats).
	// Should NOT include qt3d debug symbols.
	var debugFiles []string
	for _, a := range archives {
		if a.Name == debugPkg {
			debugFiles = append(debugFiles, a.Ref.Filename)
		}
	}
	assert.Len(t, debugFiles, 4, "expected debug symbols for 4 modules (2 essential + 2 requested)")
	assert.Contains(t, debugFiles, "6.10.2-0-202503010000qtbase-Windows-debug-symbols.7z")
	assert.Contains(t, debugFiles, "6.10.2-0-202503010000qtdeclarative-Windows-debug-symbols.7z")
	assert.Contains(t, debugFiles, "6.10.2-0-202503010000qtcharts-Windows-debug-symbols.7z")
	assert.Contains(t, debugFiles, "6.10.2-0-202503010000qtimageformats-Windows-debug-symbols.7z")
	assert.NotContains(t, debugFiles, "6.10.2-0-202503010000qt3d-Windows-debug-symbols.7z")
}

func TestResolveArchives_SkipEssentials(t *testing.T) {
	vi := testVersionInfo("win64_msvc2022_64", "qtcharts")
	archives, err := resolveArchives(vi, ResolveOptions{
		Version:        "6.10.2",
		Arch:           "win64_msvc2022_64",
		Modules:        []string{"qtcharts"},
		SkipEssentials: true,
	})
	require.NoError(t, err)
	assert.Len(t, archives, 1)
	assert.Equal(t, "qt.qt6.6102.addons.qtcharts.win64_msvc2022_64", archives[0].Name)
}
