package repository

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testVersionInfo builds a QtVersionInfo with the given addon packages registered.
func testVersionInfo(addonModules ...string) *QtVersionInfo {
	const arch = "win64_msvc2022_64"
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
	vi := testVersionInfo("qtcharts", "qtwebengine")
	archives, _, err := resolveArchives(vi, ResolveOptions{
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

func TestFetchChecksums(t *testing.T) {
	const digest = "e78e16b31fa82d5c67b9c16304da15b59eb6016b"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/good.7z.sha1" {
			_, _ = io.WriteString(w, digest+"  good.7z\n")
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := NewResolver(NewMetadataFetcher(NewClient(5), nil, nil))
	archives := []ResolvedArchive{
		{Name: "good", Ref: ArchiveRef{URL: srv.URL + "/good.7z", Filename: "good.7z"}},
		{Name: "missing", Ref: ArchiveRef{URL: srv.URL + "/missing.7z", Filename: "missing.7z"}},
		{Name: "preset", Ref: ArchiveRef{URL: srv.URL + "/preset.7z", Filename: "preset.7z", SHA1: "keepme"}},
	}
	r.FetchChecksums(context.Background(), archives)

	assert.Equal(t, digest, archives[0].Ref.SHA1, "sidecar digest should be fetched")
	assert.Empty(t, archives[1].Ref.SHA1, "missing sidecar should leave SHA1 empty")
	assert.Equal(t, "keepme", archives[2].Ref.SHA1, "preset SHA1 should be left untouched")
}

func TestResolveArchives_ExcludesBundledDebugSymbols(t *testing.T) {
	// QtWebEngine's add-on package bundles a debug-symbols archive alongside
	// the module archive. It must not be downloaded unless requested.
	const arch = "win64_msvc2022_64"
	vi := testVersionInfo("qtwebengine")
	pkg := "qt.qt6.6102.addons.qtwebengine." + arch
	vi.PackageArchives[pkg] = []ArchiveRef{
		{URL: "http://x/we.7z", Filename: "6.10.2-0-202601qtwebengine-Windows-ARM64.7z"},
		{URL: "http://x/we-dbg.7z", Filename: "6.10.2-0-202601qtwebengine-Windows-ARM64-debug-symbols.7z"},
	}

	filenames := func(archives []ResolvedArchive) []string {
		var out []string
		for _, a := range archives {
			out = append(out, a.Ref.Filename)
		}
		return out
	}

	// Default: debug-symbols excluded.
	archives, _, err := resolveArchives(vi, ResolveOptions{
		Version: "6.10.2", Arch: arch, Modules: []string{"qtwebengine"},
	})
	require.NoError(t, err)
	names := filenames(archives)
	assert.Contains(t, names, "6.10.2-0-202601qtwebengine-Windows-ARM64.7z")
	assert.NotContains(t, names, "6.10.2-0-202601qtwebengine-Windows-ARM64-debug-symbols.7z")

	// With --debug-symbols: bundled symbols are kept.
	archives, _, err = resolveArchives(vi, ResolveOptions{
		Version: "6.10.2", Arch: arch, Modules: []string{"qtwebengine"}, DebugInfo: true,
	})
	require.NoError(t, err)
	assert.Contains(t, filenames(archives), "6.10.2-0-202601qtwebengine-Windows-ARM64-debug-symbols.7z")
}

func TestResolveArchives_AutoPrefixQt(t *testing.T) {
	vi := testVersionInfo("qtcharts", "qtwebengine", "qtimageformats")
	archives, _, err := resolveArchives(vi, ResolveOptions{
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
	vi := testVersionInfo("qtcharts", "qthttpserver")
	archives, _, err := resolveArchives(vi, ResolveOptions{
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
	vi := testVersionInfo("qtcharts")
	_, _, err := resolveArchives(vi, ResolveOptions{
		Version: "6.10.2",
		Arch:    "win64_msvc2022_64",
		Modules: []string{"nonexistent"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
	assert.Contains(t, err.Error(), "is not known")
}

func TestResolveArchives_MultipleUnknown_FailsOnFirst(t *testing.T) {
	vi := testVersionInfo("qtcharts")
	_, _, err := resolveArchives(vi, ResolveOptions{
		Version: "6.10.2",
		Arch:    "win64_msvc2022_64",
		Modules: []string{"foo", "bar"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "foo")
	assert.Contains(t, err.Error(), "is not known")
}

func TestResolveArchives_NoPrefixForAlreadyPrefixed(t *testing.T) {
	// "qtcharts" is already prefixed - should not try "qtqtcharts".
	vi := testVersionInfo("qtcharts")
	archives, _, err := resolveArchives(vi, ResolveOptions{
		Version: "6.10.2",
		Arch:    "win64_msvc2022_64",
		Modules: []string{"qtcharts"},
	})
	require.NoError(t, err)
	assert.Len(t, archives, 2) // essentials + qtcharts
}

func TestResolveArchives_EssentialModuleSkipped(t *testing.T) {
	// "qtimageformats" is an essential module - requesting it should not error.
	vi := testVersionInfo("qtcharts")
	vi.Archs[0].EssentialModules = []string{"qtbase", "qtimageformats", "qtwebchannel"}

	archives, _, err := resolveArchives(vi, ResolveOptions{
		Version: "6.10.2",
		Arch:    "win64_msvc2022_64",
		Modules: []string{"imageformats", "charts", "webchannel"},
	})
	require.NoError(t, err)

	var names []string
	for _, a := range archives {
		names = append(names, a.Name)
	}
	// qtcharts should be resolved as an addon.
	assert.Contains(t, names, "qt.qt6.6102.addons.qtcharts.win64_msvc2022_64")
	// imageformats and webchannel are essentials - skipped, no error.
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
		{
			URL:      "http://x/qtimageformats-dbg.7z",
			Filename: "6.10.2-0-202503010000qtimageformats-Windows-debug-symbols.7z",
		},
		{URL: "http://x/qt3d-dbg.7z", Filename: "6.10.2-0-202503010000qt3d-Windows-debug-symbols.7z"},
		{URL: "http://x/qtdeclarative-dbg.7z", Filename: "6.10.2-0-202503010000qtdeclarative-Windows-debug-symbols.7z"},
	}
	vi.Archs[0].EssentialModules = []string{"qtbase", "qtdeclarative"}

	archives, _, err := resolveArchives(vi, ResolveOptions{
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

func TestResolveArchives_ExpandsDependencyModules(t *testing.T) {
	const arch = "win64_msvc2022_64"
	vi := testVersionInfo("qthttpserver", "qtwebsockets")
	vi.PackageDependencies = map[string][]string{
		"qt.qt6.6102.addons.qthttpserver": {
			"qt.qt6.6102.doc.qthttpserver",
			"qt.qt6.6102.examples.qthttpserver",
			"qt.qt6.6102.addons.qtwebsockets",
			"qt.tools.qtcreator",
		},
	}

	archives, modules, err := resolveArchives(vi, ResolveOptions{
		Version: "6.10.2", Arch: arch, Modules: []string{"httpserver"},
	})
	require.NoError(t, err)

	var names []string
	for _, a := range archives {
		names = append(names, a.Name)
	}
	assert.Contains(t, names, "qt.qt6.6102.addons.qthttpserver."+arch)
	assert.Contains(t, names, "qt.qt6.6102.addons.qtwebsockets."+arch)
	assert.Len(t, archives, 3, "essentials + 2 addons; doc/examples/tools dependencies must be ignored")
	assert.Equal(t, []string{"httpserver", "qtwebsockets"}, modules)
}

func TestResolveArchives_TransitiveDependencies(t *testing.T) {
	vi := testVersionInfo("qtquick3d", "qtshadertools", "qtquicktimeline")
	vi.PackageDependencies = map[string][]string{
		"qt.qt6.6102.addons.qtquick3d":     {"qt.qt6.6102.addons.qtshadertools"},
		"qt.qt6.6102.addons.qtshadertools": {"qt.qt6.6102.addons.qtquicktimeline"},
	}

	_, modules, err := resolveArchives(vi, ResolveOptions{
		Version: "6.10.2", Arch: "win64_msvc2022_64", Modules: []string{"qtquick3d"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"qtquick3d", "qtshadertools", "qtquicktimeline"}, modules)
}

func TestResolveArchives_NoDeps(t *testing.T) {
	vi := testVersionInfo("qthttpserver", "qtwebsockets")
	vi.PackageDependencies = map[string][]string{
		"qt.qt6.6102.addons.qthttpserver": {"qt.qt6.6102.addons.qtwebsockets"},
	}

	archives, modules, err := resolveArchives(vi, ResolveOptions{
		Version: "6.10.2", Arch: "win64_msvc2022_64", Modules: []string{"qthttpserver"}, NoDeps: true,
	})
	require.NoError(t, err)
	assert.Len(t, archives, 2, "essentials + qthttpserver only")
	assert.Equal(t, []string{"qthttpserver"}, modules)
}

func TestResolveArchives_DependencyAlreadyInstalled(t *testing.T) {
	vi := testVersionInfo("qthttpserver", "qtwebsockets")
	vi.PackageDependencies = map[string][]string{
		"qt.qt6.6102.addons.qthttpserver": {"qt.qt6.6102.addons.qtwebsockets"},
	}

	archives, modules, err := resolveArchives(vi, ResolveOptions{
		Version:        "6.10.2",
		Arch:           "win64_msvc2022_64",
		Modules:        []string{"qthttpserver"},
		AllModules:     []string{"qtwebsockets", "qthttpserver"},
		SkipEssentials: true,
	})
	require.NoError(t, err)
	assert.Len(t, archives, 1, "installed dependency must not be re-downloaded")
	assert.Equal(t, []string{"qthttpserver"}, modules)
}

func TestResolveArchives_DependencyWithoutArchPackageSkipped(t *testing.T) {
	vi := testVersionInfo("qthttpserver")
	vi.PackageDependencies = map[string][]string{
		"qt.qt6.6102.addons.qthttpserver": {"qt.qt6.6102.addons.qtnotforthisarch"},
	}

	archives, modules, err := resolveArchives(vi, ResolveOptions{
		Version: "6.10.2", Arch: "win64_msvc2022_64", Modules: []string{"qthttpserver"},
	})
	require.NoError(t, err)
	assert.Len(t, archives, 2)
	assert.Equal(t, []string{"qthttpserver"}, modules)
}

func TestResolveArchives_DependencyAlsoRequested_NoDuplicate(t *testing.T) {
	vi := testVersionInfo("qtquick3d", "qtshadertools")
	vi.PackageDependencies = map[string][]string{
		"qt.qt6.6102.addons.qtquick3d": {"qt.qt6.6102.addons.qtshadertools"},
	}

	archives, modules, err := resolveArchives(vi, ResolveOptions{
		Version: "6.10.2", Arch: "win64_msvc2022_64", Modules: []string{"quick3d", "shadertools"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"quick3d", "shadertools"}, modules)

	count := 0
	for _, a := range archives {
		if a.Name == "qt.qt6.6102.addons.qtshadertools.win64_msvc2022_64" {
			count++
		}
	}
	assert.Equal(t, 1, count)
}

func TestResolveArchives_CyclicDependenciesTerminate(t *testing.T) {
	vi := testVersionInfo("qta", "qtb")
	vi.PackageDependencies = map[string][]string{
		"qt.qt6.6102.addons.qta": {"qt.qt6.6102.addons.qtb"},
		"qt.qt6.6102.addons.qtb": {"qt.qt6.6102.addons.qta"},
	}

	_, modules, err := resolveArchives(vi, ResolveOptions{
		Version: "6.10.2", Arch: "win64_msvc2022_64", Modules: []string{"qta"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"qta", "qtb"}, modules)
}

func TestResolveArchives_DocsCoverDependencyModules(t *testing.T) {
	vi := testVersionInfo("qthttpserver", "qtwebsockets")
	vi.PackageDependencies = map[string][]string{
		"qt.qt6.6102.addons.qthttpserver": {"qt.qt6.6102.addons.qtwebsockets"},
	}
	vi.PackageArchives["qt.qt6.6102.doc.qtwebsockets"] = []ArchiveRef{
		{URL: "http://x/doc.7z", Filename: "qtwebsockets-doc.7z"},
	}

	archives, _, err := resolveArchives(vi, ResolveOptions{
		Version: "6.10.2", Arch: "win64_msvc2022_64", Modules: []string{"qthttpserver"}, Docs: true,
	})
	require.NoError(t, err)

	var names []string
	for _, a := range archives {
		names = append(names, a.Name)
	}
	assert.Contains(t, names, "qt.qt6.6102.doc.qtwebsockets")
}

func TestResolveArchives_SkipEssentials(t *testing.T) {
	vi := testVersionInfo("qtcharts")
	archives, _, err := resolveArchives(vi, ResolveOptions{
		Version:        "6.10.2",
		Arch:           "win64_msvc2022_64",
		Modules:        []string{"qtcharts"},
		SkipEssentials: true,
	})
	require.NoError(t, err)
	assert.Len(t, archives, 1)
	assert.Equal(t, "qt.qt6.6102.addons.qtcharts.win64_msvc2022_64", archives[0].Name)
}
