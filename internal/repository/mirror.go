package repository

import (
	"fmt"
	"runtime"
	"strings"
)

// Mirror holds a base URL for the Qt repository.
type Mirror struct {
	URL string
}

// MirrorList manages the ordered list of mirrors to try.
type MirrorList struct {
	primary  string
	fallbacks []string
}

// NewMirrorList creates a MirrorList from a primary URL and fallbacks.
func NewMirrorList(primary string, fallbacks []string) *MirrorList {
	return &MirrorList{primary: primary, fallbacks: fallbacks}
}

// URLsFor returns an ordered list of URLs to try for the given Qt SDK parameters.
// base is the mirror base URL, host is "windows_x86"/"linux_x64"/"mac_x64", etc.
func (m *MirrorList) URLsFor(version string, major int) []string {
	host := platformHost()
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
	host := platformHost()
	all := append([]string{m.primary}, m.fallbacks...)
	urls := make([]string, 0, len(all))
	for _, base := range all {
		urls = append(urls, fmt.Sprintf("%sonline/qtsdkrepository/%s/desktop/Updates.xml", base, host))
	}
	return urls
}

// ToolURLsFor returns URLs for tools metadata.
func (m *MirrorList) ToolURLsFor(toolName string) []string {
	host := platformHost()
	all := append([]string{m.primary}, m.fallbacks...)
	urls := make([]string, 0, len(all))
	for _, base := range all {
		urls = append(urls, fmt.Sprintf("%sonline/qtsdkrepository/%s/desktop/tools_%s/Updates.xml", base, host, toolName))
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
//	.../qt5_5150/Updates.xml  (Qt 5)
func buildURL(base, host, version string, major int) string {
	verStr := versionToRepoStr(version, major)
	folder := fmt.Sprintf("qt%d_%s", major, verStr)
	if isQt68Plus(version, major) {
		// Two-level: qt6_680/qt6_680/Updates.xml
		return fmt.Sprintf("%sonline/qtsdkrepository/%s/desktop/%s/%s/Updates.xml", base, host, folder, folder)
	}
	return fmt.Sprintf("%sonline/qtsdkrepository/%s/desktop/%s/Updates.xml", base, host, folder)
}

// isQt68Plus reports whether version is Qt 6.8.0 or later.
func isQt68Plus(version string, major int) bool {
	if major < 6 {
		return false
	}
	// Parse minor version from the compact string.
	// "6.8.0" → minor=8, "6.7.3" → minor=7
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return false
	}
	minor := 0
	for _, ch := range parts[1] {
		if ch >= '0' && ch <= '9' {
			minor = minor*10 + int(ch-'0')
		}
	}
	return minor >= 8
}

// ProbeURL returns the combined Updates.xml URL used to detect whether a
// version is released or preview. Released Qt 6.8+ versions have a combined
// Updates.xml at qt6_XXXX/qt6_XXXX/Updates.xml; preview versions do not.
// Returns "" for pre-6.8 versions.
func (m *MirrorList) ProbeURL(version string, major int) string {
	if !isQt68Plus(version, major) {
		return ""
	}
	host := platformHost()
	verStr := versionToRepoStr(version, major)
	folder := fmt.Sprintf("qt%d_%s", major, verStr)
	return fmt.Sprintf("%sonline/qtsdkrepository/%s/desktop/%s/%s/Updates.xml", m.primary, host, folder, folder)
}

// DirectoryURL returns the HTML directory listing URL for discovering available versions.
// Deprecated: prefer DirectoryURLs which includes fallback mirrors.
func (m *MirrorList) DirectoryURL() string {
	host := platformHost()
	return fmt.Sprintf("%sonline/qtsdkrepository/%s/desktop/", m.primary, host)
}

// DirectoryURLs returns directory listing URLs for all mirrors in order.
func (m *MirrorList) DirectoryURLs() []string {
	host := platformHost()
	all := append([]string{m.primary}, m.fallbacks...)
	urls := make([]string, 0, len(all))
	for _, base := range all {
		urls = append(urls, fmt.Sprintf("%sonline/qtsdkrepository/%s/desktop/", base, host))
	}
	return urls
}

// versionToRepoStr converts a version string to the compact form used in Qt repo folder names.
// e.g. "6.10.0" → "6100", "5.15.18" → "51518", "5.9.0" → "59" (patch omitted for 5.9.0).
func versionToRepoStr(version string, major int) string {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 3 {
		// Fallback: strip dots.
		return strings.ReplaceAll(version, ".", "")
	}
	minor := parts[1]
	patch := parts[2]
	// Qt 5.9.0 special case: patch omitted.
	if major == 5 && minor == "9" && patch == "0" {
		return major_minor(major, minor)
	}
	// Qt 5 with patch 0: patch omitted (e.g. 5.12.0 → "5120", but 5.12.3 → "5123").
	// aqtinstall omits patch only for prerelease; for stable releases patch is always included
	// except for the Qt 5.9.0 special case above.
	return fmt.Sprintf("%d%s%s", major, minor, patch)
}

func major_minor(major int, minor string) string {
	return fmt.Sprintf("%d%s", major, minor)
}

// platformHost returns the repository host path component for the current OS/arch.
func platformHost() string {
	switch runtime.GOOS {
	case "windows":
		return "windows_x86"
	case "darwin":
		return "mac_x64"
	default:
		return "linux_x64"
	}
}
