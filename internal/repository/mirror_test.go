package repository

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildURL_Qt68Plus_TwoLevel(t *testing.T) {
	base := "https://download.qt.io/"
	host := "windows_x86"

	tests := []struct {
		version string
		major   int
		want    string
	}{
		{
			"6.8.0", 6,
			"https://download.qt.io/online/qtsdkrepository/windows_x86/desktop/qt6_680/qt6_680/Updates.xml",
		},
		{
			"6.8.3", 6,
			"https://download.qt.io/online/qtsdkrepository/windows_x86/desktop/qt6_683/qt6_683/Updates.xml",
		},
		{
			"6.10.0", 6,
			"https://download.qt.io/online/qtsdkrepository/windows_x86/desktop/qt6_6100/qt6_6100/Updates.xml",
		},
		{
			"6.11.0", 6,
			"https://download.qt.io/online/qtsdkrepository/windows_x86/desktop/qt6_6110/qt6_6110/Updates.xml",
		},
	}
	for _, tc := range tests {
		t.Run(tc.version, func(t *testing.T) {
			assert.Equal(t, tc.want, buildURL(base, host, tc.version, tc.major))
		})
	}
}

func TestBuildURL_Qt6Pre68_SingleLevel(t *testing.T) {
	base := "https://download.qt.io/"
	host := "windows_x86"

	tests := []struct {
		version string
		major   int
		want    string
	}{
		{
			"6.7.3", 6,
			"https://download.qt.io/online/qtsdkrepository/windows_x86/desktop/qt6_673/Updates.xml",
		},
		{
			"6.5.3", 6,
			"https://download.qt.io/online/qtsdkrepository/windows_x86/desktop/qt6_653/Updates.xml",
		},
		{
			"6.2.0", 6,
			"https://download.qt.io/online/qtsdkrepository/windows_x86/desktop/qt6_620/Updates.xml",
		},
	}
	for _, tc := range tests {
		t.Run(tc.version, func(t *testing.T) {
			assert.Equal(t, tc.want, buildURL(base, host, tc.version, tc.major))
		})
	}
}

func TestBuildURL_Qt5_SingleLevel(t *testing.T) {
	base := "https://download.qt.io/"
	host := "windows_x86"

	tests := []struct {
		version string
		major   int
		want    string
	}{
		{
			"5.15.18", 5,
			"https://download.qt.io/online/qtsdkrepository/windows_x86/desktop/qt5_51518/Updates.xml",
		},
		{
			"5.15.0", 5,
			"https://download.qt.io/online/qtsdkrepository/windows_x86/desktop/qt5_5150/Updates.xml",
		},
		{
			"5.9.0", 5,
			"https://download.qt.io/online/qtsdkrepository/windows_x86/desktop/qt5_59/Updates.xml",
		},
	}
	for _, tc := range tests {
		t.Run(tc.version, func(t *testing.T) {
			assert.Equal(t, tc.want, buildURL(base, host, tc.version, tc.major))
		})
	}
}

func TestIsQt68Plus(t *testing.T) {
	trueFor := []string{"6.8.0", "6.8.3", "6.9.0", "6.10.0", "6.11.0"}
	falseFor := []string{"6.7.3", "6.5.3", "6.2.0", "5.15.18"}

	for _, v := range trueFor {
		assert.True(t, isQt68Plus(v), "%s should be Qt 6.8+", v)
	}
	for _, v := range falseFor {
		assert.False(t, isQt68Plus(v), "%s should not be Qt 6.8+", v)
	}
}

func TestVersionToRepoStr(t *testing.T) {
	tests := []struct {
		version string
		major   int
		want    string
	}{
		{"6.10.0", 6, "6100"},
		{"6.8.3", 6, "683"},
		{"6.8.0", 6, "680"},
		{"5.15.18", 5, "51518"},
		{"5.15.0", 5, "5150"},
		{"5.9.0", 5, "59"}, // Qt 5.9.0 special case: patch omitted
	}
	for _, tc := range tests {
		t.Run(tc.version, func(t *testing.T) {
			assert.Equal(t, tc.want, versionToRepoStr(tc.version, tc.major))
		})
	}
}

func TestMirrorList_URLsFor_TwoMirrors(t *testing.T) {
	m := NewMirrorList(
		"https://primary.example.com/",
		[]string{"https://mirror1.example.com/"},
		"windows_x86",
	)
	urls := m.URLsFor("6.8.3", 6)

	require.Len(t, urls, 2, "expected primary + 1 fallback")
	for _, u := range urls {
		assert.True(t, strings.Contains(u, "qt6_683/qt6_683/Updates.xml"),
			"URL %q should use two-level structure for Qt 6.8.3", u)
	}
	assert.Contains(t, urls[0], "primary.example.com")
	assert.Contains(t, urls[1], "mirror1.example.com")
}

func TestExtractFolderNames(t *testing.T) {
	html := `<html><body>
		<a href="../">../</a>
		<a href="qt6_683/">qt6_683/</a>
		<a href="qt6_6100/">qt6_6100/</a>
		<a href="qt5_51518/">qt5_51518/</a>
		<a href="tools_qtcreator/">tools_qtcreator/</a>
		<a href="http://external.com/path/ignored">external</a>
	</body></html>`

	got := extractFolderNames(html)

	assert.Contains(t, got, "qt6_683")
	assert.Contains(t, got, "qt6_6100")
	assert.Contains(t, got, "qt5_51518")
	assert.Contains(t, got, "tools_qtcreator")
	// External URL with a path separator in the href should not appear.
	for _, name := range got {
		assert.False(t, strings.HasPrefix(name, "http"), "unexpected external URL %q", name)
	}
}

func TestFolderToVersionInfo(t *testing.T) {
	t.Run("valid folders", func(t *testing.T) {
		tests := []struct {
			folder  string
			wantVer string
			wantLTS bool
		}{
			{"qt6_683", "6.8.3", true},
			{"qt6_6100", "6.10.0", false},
			{"qt5_51518", "5.15.18", true},
			{"qt5_59", "5.9.0", true},
		}
		for _, tc := range tests {
			t.Run(tc.folder, func(t *testing.T) {
				vi, ok := folderToVersionInfo(tc.folder)
				require.True(t, ok, "expected ok=true for %q", tc.folder)
				assert.Equal(t, tc.wantVer, vi.Version)
				assert.Equal(t, tc.wantLTS, vi.IsLTS)
			})
		}
	})

	t.Run("rejected folders", func(t *testing.T) {
		rejected := []string{
			"qt6_680_wasm_singlethread", // extended variant
			"tools_qtcreator",           // tools folder
			"random_folder",             // unknown prefix
		}
		for _, folder := range rejected {
			t.Run(folder, func(t *testing.T) {
				_, ok := folderToVersionInfo(folder)
				assert.False(t, ok, "expected ok=false for %q", folder)
			})
		}
	})
}

func TestExtensionArchSubdir(t *testing.T) {
	tests := []struct {
		arch string
		want string
	}{
		{"win64_msvc2022_64", "msvc2022_64"},
		{"win64_msvc2022_arm64_cross_compiled", "msvc2022_arm64_cross_compiled"},
		{"win64_mingw", "mingw"},
		{"win64_llvm_mingw", "llvm_mingw"},
		{"linux_x64_gcc", "gcc"},
		{"linux_arm64_gcc_arm64", "gcc_arm64"},
		{"mac_x64_clang_64", "clang_64"},
		{"macos", "macos"}, // no prefix match, returned as-is
	}
	for _, tc := range tests {
		t.Run(tc.arch, func(t *testing.T) {
			assert.Equal(t, tc.want, extensionArchSubdir(tc.arch))
		})
	}
}

func TestExtensionURLsFor(t *testing.T) {
	m := NewMirrorList(
		"https://download.qt.io/",
		[]string{"https://mirror.example.com/"},
		"windows_x86",
	)

	t.Run("Qt 6.10.2", func(t *testing.T) {
		urls := m.ExtensionURLsFor("qtwebengine", "6.10.2", 6, "win64_msvc2022_64")
		require.Len(t, urls, 2)
		assert.Equal(
			t,
			"https://download.qt.io/online/qtsdkrepository/windows_x86/extensions/qtwebengine/6102/msvc2022_64/Updates.xml",
			urls[0],
		)
		assert.Equal(
			t,
			"https://mirror.example.com/online/qtsdkrepository/windows_x86/extensions/qtwebengine/6102/msvc2022_64/Updates.xml",
			urls[1],
		)
	})

	t.Run("Qt 6.8.3 mingw", func(t *testing.T) {
		urls := m.ExtensionURLsFor("qtpdf", "6.8.3", 6, "win64_mingw")
		require.Len(t, urls, 2)
		assert.Contains(t, urls[0], "/extensions/qtpdf/683/mingw/Updates.xml")
	})

	t.Run("pre-6.8 returns nil", func(t *testing.T) {
		urls := m.ExtensionURLsFor("qtwebengine", "6.7.3", 6, "win64_msvc2022_64")
		assert.Nil(t, urls)
	})

	t.Run("Qt 5 returns nil", func(t *testing.T) {
		urls := m.ExtensionURLsFor("qtwebengine", "5.15.18", 5, "win64_msvc2019_64")
		assert.Nil(t, urls)
	})
}

func TestSrcDocExURLsFor(t *testing.T) {
	m := NewMirrorList(
		"https://download.qt.io/",
		[]string{"https://mirror.example.com/"},
		"windows_x86",
	)

	t.Run("Qt 6.10.2 uses all_os", func(t *testing.T) {
		urls := m.SrcDocExURLsFor("6.10.2", 6)
		require.Len(t, urls, 2)
		assert.Equal(t,
			"https://download.qt.io/online/qtsdkrepository/all_os/qt/qt6_6102_unix_line_endings_src/Updates.xml",
			urls[0])
		assert.Equal(t,
			"https://mirror.example.com/online/qtsdkrepository/all_os/qt/qt6_6102_unix_line_endings_src/Updates.xml",
			urls[1])
	})

	t.Run("Qt 6.8.3 uses all_os", func(t *testing.T) {
		urls := m.SrcDocExURLsFor("6.8.3", 6)
		require.Len(t, urls, 2)
		assert.Contains(t, urls[0], "/all_os/qt/qt6_683_unix_line_endings_src/Updates.xml")
	})

	t.Run("Qt 6.7.3 uses per-platform", func(t *testing.T) {
		urls := m.SrcDocExURLsFor("6.7.3", 6)
		require.Len(t, urls, 2)
		assert.Equal(t,
			"https://download.qt.io/online/qtsdkrepository/windows_x86/desktop/qt6_673_src_doc_examples/Updates.xml",
			urls[0])
		assert.Equal(
			t,
			"https://mirror.example.com/online/qtsdkrepository/windows_x86/desktop/qt6_673_src_doc_examples/Updates.xml",
			urls[1],
		)
	})

	t.Run("Qt 5 returns nil", func(t *testing.T) {
		urls := m.SrcDocExURLsFor("5.15.18", 5)
		assert.Nil(t, urls)
	})
}

func TestExtensionModuleNames(t *testing.T) {
	names := ExtensionModuleNames()
	assert.Contains(t, names, "qtwebengine")
	assert.Contains(t, names, "qtpdf")
}
