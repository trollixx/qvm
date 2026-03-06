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
