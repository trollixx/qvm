package repository

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/trollixx/qvm/pkg/qtmeta"
)

// Mirror holds a base URL for the Qt repository.
type Mirror struct {
	URL string
}

// ValidHosts lists the recognized host platform identifiers for Qt repositories.
var ValidHosts = []string{ //nolint:gochecknoglobals // exported package-level data used by callers
	"windows_x86",
	"windows_arm64",
	"linux_x64",
	"linux_arm64",
	"mac_x64",
}

// MirrorList manages the ordered list of mirrors to try.
type MirrorList struct {
	primary   string
	fallbacks []string
	host      string
}

// NewMirrorList creates a MirrorList from a primary URL, fallbacks, and host platform.
func NewMirrorList(primary string, fallbacks []string, host string) *MirrorList {
	return &MirrorList{primary: primary, fallbacks: fallbacks, host: host}
}

// URLsFor returns an ordered list of URLs to try for the given Qt SDK parameters.
// base is the mirror base URL, host is "windows_x86"/"linux_x64"/"mac_x64", etc.
func (m *MirrorList) URLsFor(version string, major int) []string {
	host := m.host
	var urls []string

	// Primary mirror first.
	urls = append(urls, buildURL(m.primary, host, version, major))

	// Fallback mirrors.
	for _, fb := range m.fallbacks {
		urls = append(urls, buildURL(fb, host, version, major))
	}
	return urls
}

// URLsForMasterList returns URLs for the master list of available Qt versions.
// This fetches a listing that enumerates available versions.
func (m *MirrorList) URLsForMasterList() []string {
	host := m.host
	all := append([]string{m.primary}, m.fallbacks...)
	urls := make([]string, 0, len(all))
	for _, base := range all {
		urls = append(urls, fmt.Sprintf("%sonline/qtsdkrepository/%s/desktop/Updates.xml", base, host))
	}
	return urls
}

// buildURL constructs a Qt SDK repository Updates.xml URL.
// base is the mirror root (e.g. "https://download.qt.io/"); online/qtsdkrepository/ is inserted automatically.
// Qt 6.8+ uses a two-level folder structure:
//
//	.../qt6_680/qt6_680/Updates.xml
//
// Earlier versions use a single-level structure:
//
//	.../qt6_690/Updates.xml   (Qt 6 < 6.8)
func buildURL(base, host, version string, major int) string {
	verStr := versionToRepoStr(version, major)
	folder := fmt.Sprintf("qt%d_%s", major, verStr)
	if isQt68Plus(version) {
		// Two-level: qt6_680/qt6_680/Updates.xml
		return fmt.Sprintf("%sonline/qtsdkrepository/%s/desktop/%s/%s/Updates.xml", base, host, folder, folder)
	}
	return fmt.Sprintf("%sonline/qtsdkrepository/%s/desktop/%s/Updates.xml", base, host, folder)
}

// isQt68Plus reports whether version is Qt 6.8.0 or later.
func isQt68Plus(version string) bool {
	v, err := qtmeta.ParseVersion(version)
	if err != nil {
		return false
	}
	return v.GTE(qtmeta.MustParseVersion("6.8.0"))
}

// isQt611Plus reports whether version is Qt 6.11.0 or later.
// Starting with Qt 6.11, some platforms (windows_x86) use per-arch subfolders
// instead of a combined Updates.xml.
func isQt611Plus(version string) bool {
	v, err := qtmeta.ParseVersion(version)
	if err != nil {
		return false
	}
	return v.GTE(qtmeta.MustParseVersion("6.11.0"))
}

// ProbeURL returns a URL used to detect whether a version is released or
// preview. For Qt 6.11+ it probes the version directory listing (which exists
// for released versions); for Qt 6.8-6.10 it probes the combined Updates.xml.
// Returns "" for pre-6.8 versions.
func (m *MirrorList) ProbeURL(version string, major int) string {
	if !isQt68Plus(version) {
		return ""
	}
	host := m.host
	verStr := versionToRepoStr(version, major)
	folder := fmt.Sprintf("qt%d_%s", major, verStr)
	if isQt611Plus(version) {
		// Qt 6.11+: probe the version directory listing.
		return fmt.Sprintf("%sonline/qtsdkrepository/%s/desktop/%s/", m.primary, host, folder)
	}
	return fmt.Sprintf("%sonline/qtsdkrepository/%s/desktop/%s/%s/Updates.xml", m.primary, host, folder, folder)
}

// VersionDirURLs returns URLs for the version directory listing.
// Used to discover per-arch subfolders for Qt 6.11+ on platforms that use them.
func (m *MirrorList) VersionDirURLs(version string, major int) []string {
	host := m.host
	verStr := versionToRepoStr(version, major)
	folder := fmt.Sprintf("qt%d_%s", major, verStr)
	all := append([]string{m.primary}, m.fallbacks...)
	urls := make([]string, 0, len(all))
	for _, base := range all {
		urls = append(urls, fmt.Sprintf("%sonline/qtsdkrepository/%s/desktop/%s/", base, host, folder))
	}
	return urls
}

// PerArchURL returns the Updates.xml URL for a specific arch subfolder.
func (m *MirrorList) PerArchURL(version string, major int, archFolder string) []string {
	host := m.host
	verStr := versionToRepoStr(version, major)
	folder := fmt.Sprintf("qt%d_%s", major, verStr)
	all := append([]string{m.primary}, m.fallbacks...)
	urls := make([]string, 0, len(all))
	for _, base := range all {
		urls = append(urls, fmt.Sprintf("%sonline/qtsdkrepository/%s/desktop/%s/%s/Updates.xml",
			base, host, folder, archFolder))
	}
	return urls
}

// DirectoryURLs returns directory listing URLs for all mirrors in order.
func (m *MirrorList) DirectoryURLs() []string {
	host := m.host
	all := append([]string{m.primary}, m.fallbacks...)
	urls := make([]string, 0, len(all))
	for _, base := range all {
		urls = append(urls, fmt.Sprintf("%sonline/qtsdkrepository/%s/desktop/", base, host))
	}
	return urls
}

// versionToRepoStr converts a version string to the compact form used in Qt repo folder names.
// e.g. "6.10.0" -> "6100", "6.8.3" -> "683".
func versionToRepoStr(version string, major int) string {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 3 {
		// Fallback: strip dots.
		return strings.ReplaceAll(version, ".", "")
	}
	minor := parts[1]
	patch := parts[2]
	return fmt.Sprintf("%d%s%s", major, minor, patch)
}

// ExtensionModuleNames returns the list of known Qt extension modules.
// Starting from Qt 6.8.0, these modules are served from a separate
// extensions/ repository path instead of the main desktop Updates.xml.
func ExtensionModuleNames() []string {
	return []string{"qtwebengine", "qtpdf"}
}

// ExtensionURLsFor returns URLs for an extension module's Updates.xml.
// Extension repos live at:
//
//	{base}online/qtsdkrepository/{host}/extensions/{module}/{compactVer}/{archSubdir}/Updates.xml
func (m *MirrorList) ExtensionURLsFor(moduleName, version string, major int, arch string) []string {
	if !isQt68Plus(version) {
		return nil
	}
	host := m.host
	verStr := versionToRepoStr(version, major)
	archDir := extensionArchSubdir(arch)
	all := append([]string{m.primary}, m.fallbacks...)
	urls := make([]string, 0, len(all))
	for _, base := range all {
		urls = append(urls, fmt.Sprintf("%sonline/qtsdkrepository/%s/extensions/%s/%s/%s/Updates.xml",
			base, host, moduleName, verStr, archDir))
	}
	return urls
}

// extensionArchSubdir maps a Qt arch name to the subdirectory name used in extension repos.
// e.g. "win64_msvc2022_64" -> "msvc2022_64", "linux_x64_gcc" -> "gcc".
func extensionArchSubdir(arch string) string {
	// Strip well-known platform prefixes.
	prefixes := []string{
		"win64_",
		"linux_x64_",
		"linux_arm64_",
		"mac_x64_",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(arch, p) {
			return arch[len(p):]
		}
	}
	return arch
}

// SrcDocExURLsFor returns URLs for the src/doc/examples repository Updates.xml.
// Qt 6.8+ moved these packages to a separate all_os repository:
//
//	{base}online/qtsdkrepository/all_os/qt/qt6_{ver}_unix_line_endings_src/Updates.xml
//
// Pre-6.8 Qt 6 versions used per-platform directories:
//
//	{base}online/qtsdkrepository/{host}/desktop/qt6_{ver}_src_doc_examples/Updates.xml
func (m *MirrorList) SrcDocExURLsFor(version string, major int) []string {
	verStr := versionToRepoStr(version, major)
	all := append([]string{m.primary}, m.fallbacks...)
	urls := make([]string, 0, len(all))
	if isQt68Plus(version) {
		folder := fmt.Sprintf("qt%d_%s_unix_line_endings_src", major, verStr)
		for _, base := range all {
			urls = append(urls, fmt.Sprintf("%sonline/qtsdkrepository/all_os/qt/%s/Updates.xml", base, folder))
		}
	} else {
		host := m.host
		folder := fmt.Sprintf("qt%d_%s_src_doc_examples", major, verStr)
		for _, base := range all {
			urls = append(urls, fmt.Sprintf("%sonline/qtsdkrepository/%s/desktop/%s/Updates.xml", base, host, folder))
		}
	}
	return urls
}

// PlatformHost returns the repository host path component for the current OS/arch.
func PlatformHost() string {
	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH == "arm64" {
			return "windows_arm64"
		}
		return "windows_x86"
	case "darwin":
		return "mac_x64"
	case "linux":
		if runtime.GOARCH == "arm64" {
			return "linux_arm64"
		}
		return "linux_x64"
	default:
		return "linux_x64"
	}
}
