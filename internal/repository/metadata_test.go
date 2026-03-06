package repository

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// packagesXML builds a minimal Updates.xml body from a slice of package descriptors.
func packagesXML(pkgs []struct{ name, virtual, archives string }) []byte {
	return packagesXMLFull(func() (out []struct {
		name, displayName, virtual, archives string
	},
	) {
		for _, p := range pkgs {
			out = append(out, struct{ name, displayName, virtual, archives string }{
				p.name, "", p.virtual, p.archives,
			})
		}
		return out
	}())
}

func packagesXMLFull(pkgs []struct{ name, displayName, virtual, archives string }) []byte {
	body := []byte(`<?xml version="1.0" encoding="UTF-8"?><Updates>`)
	for _, p := range pkgs {
		dn := p.displayName
		if dn == "" {
			dn = p.name
		}
		body = append(body, []byte(
			`<PackageUpdate>`+
				`<Name>`+p.name+`</Name>`+
				`<DisplayName>`+dn+`</DisplayName>`+
				`<Virtual>`+p.virtual+`</Virtual>`+
				`<DownloadableArchives>`+p.archives+`</DownloadableArchives>`+
				`<ReleaseDate>2024-11-14</ReleaseDate>`+
				`</PackageUpdate>`)...)
	}
	body = append(body, []byte(`</Updates>`)...)
	return body
}

// --- repoSuffixToVersion ---

func TestRepoSuffixToVersion(t *testing.T) {
	tests := []struct {
		suffix string
		major  int
		want   string
	}{
		{"6100", 6, "6.10.0"},
		{"683", 6, "6.8.3"},
		{"680", 6, "6.8.0"},
		{"690", 6, "6.9.0"},
		{"51518", 5, "5.15.18"},
		{"5150", 5, "5.15.0"},
		{"590", 5, "5.9.0"},
		{"59", 5, "5.9.0"}, // Qt 5.9.0 special case: patch omitted in folder name
		// wrong major prefix
		{"6100", 5, ""},
		{"51518", 6, ""},
		// too short
		{"6", 6, ""},
	}
	for _, tc := range tests {
		t.Run(tc.suffix, func(t *testing.T) {
			assert.Equal(t, tc.want, repoSuffixToVersion(tc.suffix, tc.major))
		})
	}
}

// --- extractTarget ---

func TestExtractTarget(t *testing.T) {
	tests := []struct {
		parts []string
		want  string
	}{
		{[]string{"qt", "qt6", "683", "win64_msvc2022_64"}, "win64_msvc2022_64"},
		{[]string{"qt", "qt6", "683", "addons", "qtcharts", "win64_msvc2022_64"}, "win64_msvc2022_64"},
		{[]string{"qt", "qt6", "683", "addons", "qtcharts"}, ""}, // module name, no target
		{[]string{"qt", "qt6", "683", "addons"}, ""},             // no last target
		{[]string{"qt", "qt6", "683", "win64_mingw"}, "win64_mingw"},
		{[]string{"qt", "qt6", "683", "macos"}, "macos"}, // macos special case
		{[]string{"qt", "qt6", "683", "doc"}, ""},        // skip list
		{[]string{"qt", "qt6", "683", "examples"}, ""},
		{[]string{"qt", "qt6", "683", "sources"}, ""},
		{[]string{"qt", "qt6", "683", "debug_info"}, ""},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, extractTarget(tc.parts), "parts=%v", tc.parts)
	}
}

// --- parseRepoIndex ---

// Qt 6.8.3 structure:
//
//	4-part non-virtual: essential target packages (have archives)
//	5-part non-virtual: addon module meta-packages (no archives) ← key case
//	6-part virtual:     addon target packages (have archives, but are virtual)
func TestParseRepoIndex_Qt683Modules(t *testing.T) {
	xml := packagesXML([]struct{ name, virtual, archives string }{
		{"qt.qt6.683.win64_msvc2022_64", "false", "qtbase-Windows-msvc.7z"},
		{"qt.qt6.683.win64_mingw", "false", "qtbase-Windows-mingw.7z"},
		// 5-part non-virtual module meta-packages — module discovery depends on these.
		{"qt.qt6.683.addons.qtcharts", "false", ""},
		{"qt.qt6.683.addons.qt3d", "false", ""},
		{"qt.qt6.683.addons.qtwebengine", "false", ""},
		// 6-part virtual addon+target packages (skipped by isVirtual, but module already registered above).
		{"qt.qt6.683.addons.qtcharts.win64_msvc2022_64", "true", "qtcharts-Windows-msvc.7z"},
		{"qt.qt6.683.addons.qtcharts.win64_mingw", "true", "qtcharts-Windows-mingw.7z"},
		{"qt.qt6.683.addons.qt3d.win64_msvc2022_64", "true", "qt3d-Windows-msvc.7z"},
	})

	idx, err := parseRepoIndex(xml, "")
	require.NoError(t, err)
	require.Len(t, idx.QtVersions, 1)

	vi := idx.QtVersions[0]
	assert.Equal(t, "6.8.3", vi.Version)
	assert.Equal(t, 6, vi.Major)

	assert.ElementsMatch(t,
		[]string{"win64_msvc2022_64", "win64_mingw"},
		archNames(vi.Archs),
	)
	// Essentials are now per-target; vi.Modules contains only add-ons.
	assert.Empty(t, essentialModuleNames(vi.Modules))
	assert.ElementsMatch(t, []string{"qtbase"}, essentialModuleNamesForArch(vi.Archs, "win64_msvc2022_64"))
	assert.ElementsMatch(t, []string{"qtbase"}, essentialModuleNamesForArch(vi.Archs, "win64_mingw"))
	assert.ElementsMatch(t, []string{"qt3d", "qtcharts", "qtwebengine"}, addonModuleNames(vi.Modules))
}

// Qt 6 < 6.8 uses 6-part non-virtual addon+target packages (no 5-part meta-packages).
func TestParseRepoIndex_Qt65ModulesNonVirtual(t *testing.T) {
	xml := packagesXML([]struct{ name, virtual, archives string }{
		{"qt.qt6.653.win64_msvc2019_64", "false", "qtbase-Windows-msvc.7z"},
		{"qt.qt6.653.addons.qtcharts.win64_msvc2019_64", "false", "qtcharts-Windows-msvc.7z"},
		{"qt.qt6.653.addons.qtwebengine.win64_msvc2019_64", "false", "qtwebengine-Windows-msvc.7z"},
	})

	idx, err := parseRepoIndex(xml, "")
	require.NoError(t, err)
	require.Len(t, idx.QtVersions, 1)

	vi := idx.QtVersions[0]
	assert.Equal(t, "6.5.3", vi.Version)
	assert.Empty(t, essentialModuleNames(vi.Modules))
	assert.ElementsMatch(t, []string{"qtbase"}, essentialModuleNamesForArch(vi.Archs, "win64_msvc2019_64"))
	assert.ElementsMatch(t, []string{"qtcharts", "qtwebengine"}, addonModuleNames(vi.Modules))
}

// Without a 5-part module meta-package, 6-part virtual packages contribute no addon modules.
// Essential modules are still derived from the 4-part target package's DownloadableArchives.
func TestParseRepoIndex_VirtualSkipped(t *testing.T) {
	xml := packagesXML([]struct{ name, virtual, archives string }{
		{"qt.qt6.683.win64_msvc2022_64", "false", "qtbase-Windows-msvc.7z"},
		// Virtual addon package only — no 5-part module meta-package present.
		{"qt.qt6.683.addons.qtcharts.win64_msvc2022_64", "true", "qtcharts-Windows-msvc.7z"},
	})

	idx, err := parseRepoIndex(xml, "")
	require.NoError(t, err)
	require.Len(t, idx.QtVersions, 1)
	assert.Empty(t, addonModuleNames(idx.QtVersions[0].Modules))
	assert.Empty(t, essentialModuleNames(idx.QtVersions[0].Modules))
	assert.ElementsMatch(t, []string{"qtbase"}, essentialModuleNamesForArch(idx.QtVersions[0].Archs, "win64_msvc2022_64"))
}

// Qt 5 package naming.
func TestParseRepoIndex_Qt5(t *testing.T) {
	xml := packagesXML([]struct{ name, virtual, archives string }{
		{"qt.qt5.51518.win64_msvc2019_64", "false", "qtbase-Windows-msvc.7z"},
		{"qt.qt5.51518.addons.qtcharts", "false", ""},
		{"qt.qt5.51518.addons.qtwebengine", "false", ""},
		{"qt.qt5.51518.addons.qtcharts.win64_msvc2019_64", "true", "qtcharts-Windows-msvc.7z"},
	})

	idx, err := parseRepoIndex(xml, "")
	require.NoError(t, err)
	require.Len(t, idx.QtVersions, 1)

	vi := idx.QtVersions[0]
	assert.Equal(t, "5.15.18", vi.Version)
	assert.Equal(t, 5, vi.Major)
	assert.Empty(t, essentialModuleNames(vi.Modules))
	assert.ElementsMatch(t, []string{"qtbase"}, essentialModuleNamesForArch(vi.Archs, "win64_msvc2019_64"))
	assert.ElementsMatch(t, []string{"qtcharts", "qtwebengine"}, addonModuleNames(vi.Modules))
}

// IsLTS is set correctly.
func TestParseRepoIndex_LTSFlag(t *testing.T) {
	xml := packagesXML([]struct{ name, virtual, archives string }{
		{"qt.qt6.683.win64_msvc2022_64", "false", "qtbase.7z"},  // 6.8 = LTS
		{"qt.qt6.6100.win64_msvc2022_64", "false", "qtbase.7z"}, // 6.10 = not LTS
	})

	idx, err := parseRepoIndex(xml, "")
	require.NoError(t, err)

	byVersion := map[string]bool{}
	for _, vi := range idx.QtVersions {
		byVersion[vi.Version] = vi.IsLTS
	}
	assert.True(t, byVersion["6.8.3"], "6.8.3 should be LTS")
	assert.False(t, byVersion["6.10.0"], "6.10.0 should not be LTS")
}

// ReleaseDate is parsed from the XML field.
func TestParseRepoIndex_ReleaseDate(t *testing.T) {
	body := []byte(`<?xml version="1.0" encoding="UTF-8"?><Updates>
		<PackageUpdate>
			<Name>qt.qt6.683.win64_msvc2022_64</Name>
			<Virtual>false</Virtual>
			<DownloadableArchives>qtbase.7z</DownloadableArchives>
			<ReleaseDate>2024-11-14</ReleaseDate>
		</PackageUpdate>
	</Updates>`)

	idx, err := parseRepoIndex(body, "")
	require.NoError(t, err)
	require.Len(t, idx.QtVersions, 1)

	want := time.Date(2024, 11, 14, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, want, idx.QtVersions[0].ReleaseDate)
}

// Multiple Qt versions in one file are all parsed.
func TestParseRepoIndex_MultipleVersions(t *testing.T) {
	xml := packagesXML([]struct{ name, virtual, archives string }{
		{"qt.qt6.683.win64_msvc2022_64", "false", "qtbase.7z"},
		{"qt.qt6.6100.win64_msvc2022_64", "false", "qtbase.7z"},
		{"qt.qt5.51518.win64_msvc2019_64", "false", "qtbase.7z"},
	})

	idx, err := parseRepoIndex(xml, "")
	require.NoError(t, err)

	versions := make([]string, len(idx.QtVersions))
	for i, vi := range idx.QtVersions {
		versions[i] = vi.Version
	}
	assert.ElementsMatch(t, []string{"6.8.3", "6.10.0", "5.15.18"}, versions)
}

// Tool packages are parsed into ToolInfo entries.
func TestParseRepoIndex_Tools(t *testing.T) {
	body := []byte(`<?xml version="1.0" encoding="UTF-8"?><Updates>
		<PackageUpdate>
			<Name>qt.tools.qtcreator</Name>
			<DisplayName>Qt Creator</DisplayName>
			<Virtual>false</Virtual>
			<Version>14.0.2-0-202501151209</Version>
			<ReleaseDate>2025-01-15</ReleaseDate>
		</PackageUpdate>
		<PackageUpdate>
			<Name>qt.tools.cmake.win64</Name>
			<DisplayName>CMake 3.28.0</DisplayName>
			<Virtual>false</Virtual>
			<Version>3.28.0-0-202401010000</Version>
			<ReleaseDate>2024-01-01</ReleaseDate>
		</PackageUpdate>
	</Updates>`)

	idx, err := parseRepoIndex(body, "")
	require.NoError(t, err)
	require.NotEmpty(t, idx.Tools)

	names := make([]string, len(idx.Tools))
	for i, tool := range idx.Tools {
		names[i] = tool.Name
	}
	assert.Contains(t, names, "qtcreator")
	assert.Contains(t, names, "cmake")
}

// Essential modules are derived from the DownloadableArchives of the 4-part target package.
// Archive names follow "moduleName-OS-...-Arch.7z"; module name is the prefix before the first "-".
func TestParseRepoIndex_EssentialModulesFromArchives(t *testing.T) {
	// A realistic set of archives for the essential bundle.
	archives := "qtbase-Windows-msvc.7z, qtdeclarative-Windows-msvc.7z, qttools-Windows-msvc.7z, qttranslations-Windows-msvc.7z"
	xml := packagesXML([]struct{ name, virtual, archives string }{
		{"qt.qt6.683.win64_msvc2022_64", "false", archives},
	})

	idx, err := parseRepoIndex(xml, "")
	require.NoError(t, err)
	require.Len(t, idx.QtVersions, 1)

	vi := idx.QtVersions[0]
	assert.Empty(t, essentialModuleNames(vi.Modules))
	assert.ElementsMatch(t,
		[]string{"qtbase", "qtdeclarative", "qttools", "qttranslations"},
		essentialModuleNamesForArch(vi.Archs, "win64_msvc2022_64"),
	)
	assert.Empty(t, addonModuleNames(vi.Modules))
}

// Each target tracks its own essential modules independently. Archives from
// win64_mingw (e.g. mingw1310_64-...) must not appear in win64_msvc2022_64's list.
func TestParseRepoIndex_EssentialModulesPerTarget(t *testing.T) {
	xml := packagesXML([]struct{ name, virtual, archives string }{
		{"qt.qt6.683.win64_msvc2022_64", "false", "qtbase-Windows-msvc.7z, qtdeclarative-Windows-msvc.7z"},
		{"qt.qt6.683.win64_mingw", "false", "qtbase-Windows-mingw.7z, mingw1310_64-Windows-mingw.7z"},
	})

	idx, err := parseRepoIndex(xml, "")
	require.NoError(t, err)
	require.Len(t, idx.QtVersions, 1)

	vi := idx.QtVersions[0]

	// vi.Modules must contain no essentials — they live on each Target.
	assert.Empty(t, essentialModuleNames(vi.Modules))

	// MSVC target sees only its own archives — no mingw1310_64 leak.
	assert.ElementsMatch(t,
		[]string{"qtbase", "qtdeclarative"},
		essentialModuleNamesForArch(vi.Archs, "win64_msvc2022_64"),
	)

	// MinGW target sees both its Qt archives and its toolchain archive.
	assert.ElementsMatch(t,
		[]string{"mingw1310_64", "qtbase"},
		essentialModuleNamesForArch(vi.Archs, "win64_mingw"),
	)
}

// DisplayName from the XML <DisplayName> field is preserved on modules.
func TestParseRepoIndex_ModuleDisplayName(t *testing.T) {
	body := packagesXMLFull([]struct{ name, displayName, virtual, archives string }{
		{"qt.qt6.683.win64_msvc2022_64", "Qt 6.8.3 MSVC 2022", "false", "qtbase.7z"},
		{"qt.qt6.683.addons.qtcharts", "Qt Charts", "false", ""},
		{"qt.qt6.683.addons.qtwebengine", "Qt WebEngine", "false", ""},
		// Virtual addon+target package — should not overwrite already-registered DisplayName.
		{"qt.qt6.683.addons.qtcharts.win64_msvc2022_64", "Qt Charts MSVC", "true", "qtcharts.7z"},
	})

	idx, err := parseRepoIndex(body, "")
	require.NoError(t, err)
	require.Len(t, idx.QtVersions, 1)

	byName := map[string]string{}
	for _, m := range idx.QtVersions[0].Modules {
		byName[m.Name] = m.DisplayName
	}
	assert.Equal(t, "Qt Charts", byName["qtcharts"], "DisplayName should come from 5-part meta-package")
	assert.Equal(t, "Qt WebEngine", byName["qtwebengine"])
}

// Archive refs are stored in PackageArchives with correct URLs.
// For Qt 6.8+, the 6-part virtual addon+target package provides the real archives.
func TestParseRepoIndex_PackageArchivesURLs(t *testing.T) {
	base := "https://download.qt.io/online/qtsdkrepository/windows_x86/desktop/qt6_683/qt6_683/"
	xml := packagesXML([]struct{ name, virtual, archives string }{
		// Essential bundle (non-virtual).
		{"qt.qt6.683.win64_msvc2022_64", "false", "qtbase-Windows-msvc.7z, qtdeclarative-Windows-msvc.7z"},
		// Addon meta-package (non-virtual, no archives in Qt 6.8+).
		{"qt.qt6.683.addons.qtcharts", "false", ""},
		// Addon target package (VIRTUAL in Qt 6.8+, has the real archives).
		{"qt.qt6.683.addons.qtcharts.win64_msvc2022_64", "true", "qtcharts-Windows-msvc.7z"},
	})

	idx, err := parseRepoIndex(xml, base)
	require.NoError(t, err)
	require.Len(t, idx.QtVersions, 1)

	vi := idx.QtVersions[0]

	// Essential bundle archives.
	essArcs := vi.PackageArchives["qt.qt6.683.win64_msvc2022_64"]
	require.Len(t, essArcs, 2)
	assert.Equal(t, base+"qt.qt6.683.win64_msvc2022_64/qtbase-Windows-msvc.7z", essArcs[0].URL)
	assert.Equal(t, "qtbase-Windows-msvc.7z", essArcs[0].Filename)
	assert.Equal(t, base+"qt.qt6.683.win64_msvc2022_64/qtdeclarative-Windows-msvc.7z", essArcs[1].URL)

	// Addon archives come from the virtual 6-part package.
	addonArcs := vi.PackageArchives["qt.qt6.683.addons.qtcharts.win64_msvc2022_64"]
	require.Len(t, addonArcs, 1)
	assert.Equal(t, base+"qt.qt6.683.addons.qtcharts.win64_msvc2022_64/qtcharts-Windows-msvc.7z", addonArcs[0].URL)

	// The 5-part meta-package has no archives.
	assert.Empty(t, vi.PackageArchives["qt.qt6.683.addons.qtcharts"])
}

func TestParseRepoIndex_DocArchivesStored(t *testing.T) {
	base := "https://download.qt.io/online/qtsdkrepository/windows_x86/desktop/qt6_683/qt6_683/"
	xml := packagesXML([]struct{ name, virtual, archives string }{
		{"qt.qt6.683.win64_msvc2022_64", "false", "qtbase-Windows-msvc.7z"},
		// Doc package — no arch, has archives.
		{"qt.qt6.683.doc.qtcharts", "false", "qtcharts-doc.7z"},
		{"qt.qt6.683.doc.qtbase", "false", "qtbase-doc.7z"},
		// Examples package.
		{"qt.qt6.683.examples.qtcharts", "false", "qtcharts-examples.7z"},
	})

	idx, err := parseRepoIndex(xml, base)
	require.NoError(t, err)
	require.Len(t, idx.QtVersions, 1)

	vi := idx.QtVersions[0]
	assert.True(t, vi.HasDocs)
	assert.True(t, vi.HasExamples)

	// Doc archives must be stored in PackageArchives.
	docArcs := vi.PackageArchives["qt.qt6.683.doc.qtcharts"]
	require.Len(t, docArcs, 1, "doc archives should be stored in PackageArchives")
	assert.Equal(t, "qtcharts-doc.7z", docArcs[0].Filename)

	docBaseArcs := vi.PackageArchives["qt.qt6.683.doc.qtbase"]
	require.Len(t, docBaseArcs, 1)

	// Example archives must be stored too.
	exArcs := vi.PackageArchives["qt.qt6.683.examples.qtcharts"]
	require.Len(t, exArcs, 1, "example archives should be stored in PackageArchives")
	assert.Equal(t, "qtcharts-examples.7z", exArcs[0].Filename)
}

// --- parseDirectoryListing ---

func TestParseDirectoryListing_IsPreviewNotSet(t *testing.T) {
	// parseDirectoryListing only parses folder names; IsPreview requires network
	// probing and is not set at parse time.
	html := `<html><body>
		<a href="qt6_6110/">qt6_6110/</a>
		<a href="qt6_683/">qt6_683/</a>
		<a href="qt6_6100/">qt6_6100/</a>
		<a href="qt5_51518/">qt5_51518/</a>
	</body></html>`

	versions, err := parseDirectoryListing([]byte(html))
	require.NoError(t, err)

	byVersion := map[string]bool{}
	for _, vi := range versions {
		byVersion[vi.Version] = vi.IsPreview
	}

	// No version should have IsPreview set from parsing alone.
	assert.False(t, byVersion["6.11.0"], "IsPreview should not be set by parseDirectoryListing")
	assert.False(t, byVersion["6.8.3"])
	assert.False(t, byVersion["6.10.0"])
	assert.False(t, byVersion["5.15.18"])
}

// --- probePreviewVersions with mock server ---

func TestProbePreviewVersions(t *testing.T) {
	// Set up a test HTTP server that returns 200 for released versions
	// and 404 for preview versions.
	released := map[string]bool{
		"/online/qtsdkrepository/windows_x86/desktop/qt6_683/qt6_683/Updates.xml":   true,
		"/online/qtsdkrepository/windows_x86/desktop/qt6_6100/qt6_6100/Updates.xml": true,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if released[r.URL.Path] {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewClient(10)
	mirrors := NewMirrorList(srv.URL+"/", nil, "windows_x86")
	cache := &Cache{dir: t.TempDir()}
	fetcher := NewMetadataFetcher(client, cache, mirrors)

	versions := []QtVersionInfo{
		{Version: "6.11.0", Major: 6},  // preview (404)
		{Version: "6.10.0", Major: 6},  // released (200)
		{Version: "6.8.3", Major: 6},   // released (200)
		{Version: "5.15.18", Major: 5}, // pre-6.8, always not preview
	}

	fetcher.probePreviewVersions(context.Background(), versions)

	assert.True(t, versions[0].IsPreview, "6.11.0 should be detected as preview")
	assert.False(t, versions[1].IsPreview, "6.10.0 should not be preview")
	assert.False(t, versions[2].IsPreview, "6.8.3 should not be preview")
	assert.False(t, versions[3].IsPreview, "5.15.18 should not be preview (pre-6.8)")
}

// --- cacheKeyFromURL ---

func TestCacheKeyFromURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			"primary mirror",
			"https://download.qt.io/online/qtsdkrepository/windows_x86/desktop/qt6_683/Updates.xml",
			"online/qtsdkrepository/windows_x86/desktop/qt6_683/Updates.xml",
		},
		{
			"fallback mirror with extra path",
			"https://mirrors.ocf.berkeley.edu/qt/online/qtsdkrepository/windows_x86/desktop/qt6_683/Updates.xml",
			"online/qtsdkrepository/windows_x86/desktop/qt6_683/Updates.xml",
		},
		{
			"extension URL",
			"https://download.qt.io/online/qtsdkrepository/windows_x86/extensions/qtwebengine/6102/msvc2022_64/Updates.xml",
			"online/qtsdkrepository/windows_x86/extensions/qtwebengine/6102/msvc2022_64/Updates.xml",
		},
		{
			"directory listing URL",
			"https://download.qt.io/online/qtsdkrepository/windows_x86/desktop/",
			"online/qtsdkrepository/windows_x86/desktop/",
		},
		{
			"no marker fallback",
			"https://example.com/something/else",
			"https://example.com/something/else",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, cacheKeyFromURL(tc.url))
		})
	}
}

// --- helpers ---

func archNames(archs []Arch) []string {
	names := make([]string, len(archs))
	for i, a := range archs {
		names[i] = a.Name
	}
	sort.Strings(names)
	return names
}

func moduleNames(modules []Module) []string {
	names := make([]string, len(modules))
	for i, m := range modules {
		names[i] = m.Name
	}
	sort.Strings(names)
	return names
}

func addonModuleNames(modules []Module) []string {
	var names []string
	for _, m := range modules {
		if !m.IsEssential {
			names = append(names, m.Name)
		}
	}
	sort.Strings(names)
	return names
}

func essentialModuleNames(modules []Module) []string {
	var names []string
	for _, m := range modules {
		if m.IsEssential {
			names = append(names, m.Name)
		}
	}
	sort.Strings(names)
	return names
}

func essentialModuleNamesForArch(archs []Arch, archName string) []string {
	for _, a := range archs {
		if a.Name == archName {
			names := make([]string, len(a.EssentialModules))
			copy(names, a.EssentialModules)
			sort.Strings(names)
			return names
		}
	}
	return nil
}

// --- fetchExtensions integration ---

func TestFetchExtensions_Integration(t *testing.T) {
	// Extension Updates.xml for qtwebengine with one package.
	extensionXML := []byte(`<?xml version="1.0" encoding="UTF-8"?><Updates>
		<PackageUpdate>
			<Name>extensions.qtwebengine.6102.win64_msvc2022_64</Name>
			<DisplayName>Qt WebEngine</DisplayName>
			<Virtual>false</Virtual>
			<Version>6.10.2-0-202503010000</Version>
			<DownloadableArchives>qtwebengine-Windows-msvc.7z</DownloadableArchives>
			<ReleaseDate>2025-03-01</ReleaseDate>
		</PackageUpdate>
	</Updates>`)

	extensionPdfXML := []byte(`<?xml version="1.0" encoding="UTF-8"?><Updates>
		<PackageUpdate>
			<Name>extensions.qtpdf.6102.win64_msvc2022_64</Name>
			<DisplayName>Qt PDF</DisplayName>
			<Virtual>false</Virtual>
			<Version>6.10.2-0-202503010000</Version>
			<DownloadableArchives>qtpdf-Windows-msvc.7z</DownloadableArchives>
			<ReleaseDate>2025-03-01</ReleaseDate>
		</PackageUpdate>
	</Updates>`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/extensions/qtwebengine/6102/msvc2022_64/"):
			w.Write(extensionXML)
		case strings.Contains(r.URL.Path, "/extensions/qtpdf/6102/msvc2022_64/"):
			w.Write(extensionPdfXML)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	vi := &QtVersionInfo{
		Version: "6.10.2",
		Major:   6,
		Archs: []Arch{
			{Name: "win64_msvc2022_64", DisplayName: "MSVC 2022"},
		},
		PackageArchives: make(map[string][]ArchiveRef),
	}

	client := NewClient(10)
	mirrors := NewMirrorList(srv.URL+"/", nil, "windows_x86")
	cache := &Cache{dir: t.TempDir()}
	fetcher := NewMetadataFetcher(client, cache, mirrors)

	fetcher.fetchExtensions(context.Background(), vi)

	// Check modules were registered.
	moduleNames := addonModuleNames(vi.Modules)
	assert.Contains(t, moduleNames, "qtwebengine")
	assert.Contains(t, moduleNames, "qtpdf")

	// Check display names.
	byName := map[string]string{}
	for _, m := range vi.Modules {
		byName[m.Name] = m.DisplayName
	}
	assert.Equal(t, "Qt WebEngine", byName["qtwebengine"])
	assert.Equal(t, "Qt PDF", byName["qtpdf"])

	// Check archives stored under standard addon key.
	weKey := "qt.qt6.6102.addons.qtwebengine.win64_msvc2022_64"
	weArcs := vi.PackageArchives[weKey]
	require.NotEmpty(t, weArcs, "expected archives for %s", weKey)
	assert.Equal(t, "6.10.2-0-202503010000qtwebengine-Windows-msvc.7z", weArcs[0].Filename)

	pdfKey := "qt.qt6.6102.addons.qtpdf.win64_msvc2022_64"
	pdfArcs := vi.PackageArchives[pdfKey]
	require.NotEmpty(t, pdfArcs, "expected archives for %s", pdfKey)
	assert.Equal(t, "6.10.2-0-202503010000qtpdf-Windows-msvc.7z", pdfArcs[0].Filename)
}

func TestFetchExtensions_SkipsPreQt68(t *testing.T) {
	vi := &QtVersionInfo{
		Version: "6.7.3",
		Major:   6,
		Archs:   []Arch{{Name: "win64_msvc2022_64"}},
	}

	client := NewClient(10)
	mirrors := NewMirrorList("https://example.com/", nil, "windows_x86")
	cache := &Cache{dir: t.TempDir()}
	fetcher := NewMetadataFetcher(client, cache, mirrors)

	// Should be a no-op for pre-6.8 versions (no network calls).
	fetcher.fetchExtensions(context.Background(), vi)
	assert.Empty(t, vi.Modules)
}

// --- fetchSrcDocExamples integration ---

func TestFetchSrcDocExamples_Qt68Plus(t *testing.T) {
	// Simulate the all_os src/doc/examples Updates.xml for Qt 6.10.2.
	srcDocXML := []byte(`<?xml version="1.0" encoding="UTF-8"?><Updates>
		<PackageUpdate>
			<Name>qt.qt6.6102.doc.qtcharts</Name>
			<Virtual>false</Virtual>
			<Version>6.10.2-0-202503010000</Version>
			<DownloadableArchives>qtcharts-doc.7z</DownloadableArchives>
			<ReleaseDate>2025-03-01</ReleaseDate>
		</PackageUpdate>
		<PackageUpdate>
			<Name>qt.qt6.6102.doc.qtbase</Name>
			<Virtual>false</Virtual>
			<Version>6.10.2-0-202503010000</Version>
			<DownloadableArchives>qtbase-doc.7z</DownloadableArchives>
			<ReleaseDate>2025-03-01</ReleaseDate>
		</PackageUpdate>
		<PackageUpdate>
			<Name>qt.qt6.6102.examples</Name>
			<Virtual>false</Virtual>
			<Version>6.10.2-0-202503010000</Version>
			<DownloadableArchives>qtbase-examples.7z</DownloadableArchives>
			<ReleaseDate>2025-03-01</ReleaseDate>
		</PackageUpdate>
		<PackageUpdate>
			<Name>qt.qt6.6102.examples.qtcharts</Name>
			<Virtual>false</Virtual>
			<Version>6.10.2-0-202503010000</Version>
			<DownloadableArchives>qtcharts-examples.7z</DownloadableArchives>
			<ReleaseDate>2025-03-01</ReleaseDate>
		</PackageUpdate>
		<PackageUpdate>
			<Name>qt.qt6.6102.src</Name>
			<Virtual>false</Virtual>
			<Version>6.10.2-0-202503010000</Version>
			<DownloadableArchives>qtbase-src.7z</DownloadableArchives>
			<ReleaseDate>2025-03-01</ReleaseDate>
		</PackageUpdate>
		<PackageUpdate>
			<Name>qt.qt6.6102.doc.virtual_only</Name>
			<Virtual>false</Virtual>
			<Version>6.10.2-0-202503010000</Version>
			<DownloadableArchives></DownloadableArchives>
			<ReleaseDate>2025-03-01</ReleaseDate>
		</PackageUpdate>
	</Updates>`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/all_os/qt/qt6_6102_unix_line_endings_src/") {
			w.Write(srcDocXML)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	vi := &QtVersionInfo{
		Version:         "6.10.2",
		Major:           6,
		PackageArchives: make(map[string][]ArchiveRef),
	}

	client := NewClient(10)
	mirrors := NewMirrorList(srv.URL+"/", nil, "windows_x86")
	cache := &Cache{dir: t.TempDir()}
	fetcher := NewMetadataFetcher(client, cache, mirrors)

	fetcher.fetchSrcDocExamples(context.Background(), vi)

	// Feature flags.
	assert.True(t, vi.HasDocs)
	assert.True(t, vi.HasExamples)
	assert.True(t, vi.HasSources)

	// Doc archives.
	docArcs := vi.PackageArchives["qt.qt6.6102.doc.qtcharts"]
	require.Len(t, docArcs, 1)
	assert.Contains(t, docArcs[0].Filename, "qtcharts-doc.7z")

	docBaseArcs := vi.PackageArchives["qt.qt6.6102.doc.qtbase"]
	require.Len(t, docBaseArcs, 1)

	// Example archives.
	exArcs := vi.PackageArchives["qt.qt6.6102.examples.qtcharts"]
	require.Len(t, exArcs, 1)
	assert.Contains(t, exArcs[0].Filename, "qtcharts-examples.7z")

	// Source archives.
	srcArcs := vi.PackageArchives["qt.qt6.6102.src"]
	require.Len(t, srcArcs, 1)
	assert.Contains(t, srcArcs[0].Filename, "qtbase-src.7z")

	// Virtual-only package (empty DownloadableArchives) should not be stored.
	assert.Empty(t, vi.PackageArchives["qt.qt6.6102.doc.virtual_only"])
}

func TestFetchSrcDocExamples_PreQt68(t *testing.T) {
	srcDocXML := []byte(`<?xml version="1.0" encoding="UTF-8"?><Updates>
		<PackageUpdate>
			<Name>qt.qt6.673.doc.qtcharts</Name>
			<Virtual>false</Virtual>
			<Version>6.7.3-0-202410010000</Version>
			<DownloadableArchives>qtcharts-doc.7z</DownloadableArchives>
			<ReleaseDate>2024-10-01</ReleaseDate>
		</PackageUpdate>
	</Updates>`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/desktop/qt6_673_src_doc_examples/") {
			w.Write(srcDocXML)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	vi := &QtVersionInfo{
		Version:         "6.7.3",
		Major:           6,
		PackageArchives: make(map[string][]ArchiveRef),
	}

	client := NewClient(10)
	mirrors := NewMirrorList(srv.URL+"/", nil, "windows_x86")
	cache := &Cache{dir: t.TempDir()}
	fetcher := NewMetadataFetcher(client, cache, mirrors)

	fetcher.fetchSrcDocExamples(context.Background(), vi)

	assert.True(t, vi.HasDocs)
	docArcs := vi.PackageArchives["qt.qt6.673.doc.qtcharts"]
	require.Len(t, docArcs, 1)
}

func TestFetchSrcDocExamples_SkipsQt5(t *testing.T) {
	vi := &QtVersionInfo{
		Version: "5.15.18",
		Major:   5,
	}

	client := NewClient(10)
	mirrors := NewMirrorList("https://example.com/", nil, "windows_x86")
	cache := &Cache{dir: t.TempDir()}
	fetcher := NewMetadataFetcher(client, cache, mirrors)

	// Should be a no-op for Qt 5.
	fetcher.fetchSrcDocExamples(context.Background(), vi)
	assert.False(t, vi.HasDocs)
	assert.False(t, vi.HasExamples)
	assert.False(t, vi.HasSources)
}

func TestFetchSrcDocExamples_GracefulOnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	vi := &QtVersionInfo{
		Version:         "6.10.2",
		Major:           6,
		PackageArchives: make(map[string][]ArchiveRef),
	}

	client := NewClient(10)
	mirrors := NewMirrorList(srv.URL+"/", nil, "windows_x86")
	cache := &Cache{dir: t.TempDir()}
	fetcher := NewMetadataFetcher(client, cache, mirrors)

	// Should not panic when server returns 404.
	fetcher.fetchSrcDocExamples(context.Background(), vi)
	assert.False(t, vi.HasDocs)
	assert.Empty(t, vi.PackageArchives)
}

func TestFetchExtensions_GracefulOnMissingExtension(t *testing.T) {
	// Server that always returns 404.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	vi := &QtVersionInfo{
		Version:         "6.10.2",
		Major:           6,
		Archs:           []Arch{{Name: "win64_msvc2022_64"}},
		PackageArchives: make(map[string][]ArchiveRef),
	}

	client := NewClient(10)
	mirrors := NewMirrorList(srv.URL+"/", nil, "windows_x86")
	cache := &Cache{dir: t.TempDir()}
	fetcher := NewMetadataFetcher(client, cache, mirrors)

	// Should not panic or add modules when extensions are unavailable.
	fetcher.fetchExtensions(context.Background(), vi)
	assert.Empty(t, vi.Modules)
}
