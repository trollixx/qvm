package repository

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/trollixx/qvm/pkg/qtmeta"
)

// updatesXML mirrors the top-level Updates.xml structure.
type updatesXML struct {
	XMLName  xml.Name     `xml:"Updates"`
	Packages []packageXML `xml:"PackageUpdate"`
}

type packageXML struct {
	Name                 string          `xml:"Name"`
	DisplayName          string          `xml:"DisplayName"`
	Description          string          `xml:"Description"`
	Version              string          `xml:"Version"`
	ReleaseDate          string          `xml:"ReleaseDate"`
	DownloadableArchives string          `xml:"DownloadableArchives"`
	SHA1                 string          `xml:"ArchiveSHA1"` // Comma-separated SHA1s matching DownloadableArchives
	Dependencies         string          `xml:"Dependencies"`
	Virtual              string          `xml:"Virtual"`
	UpdateFiles          []updateFileXML `xml:"UpdateFile"`
}

type updateFileXML struct {
	CompressedSize   string `xml:"CompressedSize,attr"`
	UncompressedSize string `xml:"UncompressedSize,attr"`
	SHA1             string `xml:"SHA1,attr"`
	FileName         string `xml:"FileName,attr"`
}

// MetadataFetcher fetches and parses Qt repository metadata.
type MetadataFetcher struct {
	client  *Client
	cache   *Cache
	mirrors *MirrorList
}

// NewMetadataFetcher creates a MetadataFetcher.
func NewMetadataFetcher(client *Client, cache *Cache, mirrors *MirrorList) *MetadataFetcher {
	return &MetadataFetcher{client: client, cache: cache, mirrors: mirrors}
}

// FetchQtVersion fetches metadata for a specific Qt version.
// For Qt 6.8+, it also fetches extension modules (qtwebengine, qtpdf) and
// merges them into the result as regular add-on modules.
func (f *MetadataFetcher) FetchQtVersion(ctx context.Context, version string) (*RepoIndex, error) {
	major := qtmeta.MajorVersion(version)
	if major == 0 {
		return nil, fmt.Errorf("invalid version %q", version)
	}
	urls := f.mirrors.URLsFor(version, major)
	idx, err := f.fetchFromURLs(ctx, urls)
	if err != nil {
		return nil, err
	}

	// Augment with extension modules (Qt 6.8+) and src/doc/examples.
	for i := range idx.QtVersions {
		f.fetchExtensions(ctx, &idx.QtVersions[i])
		f.fetchSrcDocExamples(ctx, &idx.QtVersions[i])
	}
	return idx, nil
}

// FetchAllQtVersions fetches the list of available Qt versions by parsing
// the HTML directory listing at the repository base URL. The response is
// cached on disk (with ETag revalidation) and all configured mirrors are
// tried in order before falling back to stale cache.
func (f *MetadataFetcher) FetchAllQtVersions(ctx context.Context) ([]QtVersionInfo, error) {
	urls := f.mirrors.DirectoryURLs()
	body, _, err := f.fetchRaw(ctx, urls)
	if err != nil {
		return nil, fmt.Errorf("fetching Qt version directory: %w", err)
	}
	versions := parseDirectoryListing(body)

	f.probePreviewVersions(ctx, versions)
	return versions, nil
}

// probePreviewVersions checks each Qt 6.8+ version with a HEAD request to the
// combined Updates.xml URL. Released versions have this file; unreleased/preview
// versions return 404. Pre-6.8 versions are always considered released.
func (f *MetadataFetcher) probePreviewVersions(ctx context.Context, versions []QtVersionInfo) {
	// Collect indices of versions that need probing (Qt 6.8+ only).
	var indices []int
	for i, v := range versions {
		if isQt68Plus(v.Version) {
			indices = append(indices, i)
		}
	}
	if len(indices) == 0 {
		return
	}

	// Bounded concurrency for HEAD requests.
	const maxConcurrency = 8
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	for _, idx := range indices {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if ctx.Err() != nil {
				return
			}

			if f.isPreviewVersion(ctx, versions[i].Version, versions[i].Major) {
				versions[i].IsPreview = true
			}
		}(idx)
	}
	wg.Wait()
}

// isPreviewVersion performs a HEAD request to the combined Updates.xml URL for
// a Qt 6.8+ version. Returns true if the URL returns 404 (preview/unreleased).
// Callers must ensure version is Qt 6.8+.
func (f *MetadataFetcher) isPreviewVersion(ctx context.Context, version string, major int) bool {
	probeURL := f.mirrors.ProbeURL(version, major)
	if probeURL == "" {
		return false
	}

	status, err := f.client.Head(ctx, probeURL)
	if err != nil {
		// Network error - assume not preview (conservative).
		return false
	}
	return status == http.StatusNotFound
}

// parseDirectoryListing parses an HTML directory page from the Qt repository
// and returns QtVersionInfo stubs for each Qt SDK folder found.
func parseDirectoryListing(body []byte) []QtVersionInfo {
	html := string(body)
	var versions []QtVersionInfo

	// Extract href values from <a href="..."> links.
	// We look for folders named qt5_NNN or qt6_NNN (top-level version folders).
	for _, folder := range extractFolderNames(html) {
		vi, ok := folderToVersionInfo(folder)
		if ok {
			versions = append(versions, vi)
		}
	}
	return versions
}

// extractFolderNames extracts href folder names from a plain HTML directory listing.
// Qt's download server returns simple Apache-style index pages.
func extractFolderNames(html string) []string {
	var folders []string
	// Look for href="name/" patterns (Apache directory listing format).
	rest := html
	for {
		idx := strings.Index(rest, `href="`)
		if idx < 0 {
			break
		}
		rest = rest[idx+6:]
		end := strings.IndexByte(rest, '"')
		if end < 0 {
			break
		}
		href := rest[:end]
		rest = rest[end:]
		// Folder hrefs end with "/" and have no path separator.
		if strings.HasSuffix(href, "/") && !strings.Contains(href[:len(href)-1], "/") {
			folders = append(folders, href[:len(href)-1])
		}
	}
	return folders
}

// folderToVersionInfo converts a repository folder name like "qt6_680" into a QtVersionInfo.
// Returns (info, true) on success; (_, false) for non-Qt-version folders (e.g. tools_*).
func folderToVersionInfo(folder string) (QtVersionInfo, bool) {
	// Accept: qt5_NNN or qt6_NNN (but not qt5_NNN_src_doc_examples etc.)
	var major int
	switch {
	case strings.HasPrefix(folder, "qt6_"):
		major = 6
	case strings.HasPrefix(folder, "qt5_"):
		major = 5
	default:
		return QtVersionInfo{}, false
	}

	suffix := folder[4:] // strip "qt5_" or "qt6_"

	// Reject extended folders like "qt6_680_wasm_singlethread".
	if strings.Contains(suffix, "_") {
		return QtVersionInfo{}, false
	}

	version := repoSuffixToVersion(suffix, major)
	if version == "" {
		return QtVersionInfo{}, false
	}

	return QtVersionInfo{
		Version: version,
		Major:   major,
		IsLTS:   qtmeta.IsLTS(version),
	}, true
}

// FetchTool fetches metadata for a named tool.
func (f *MetadataFetcher) FetchTool(ctx context.Context, toolName string) (*ToolInfo, error) {
	urls := f.mirrors.ToolURLsFor(toolName)
	body, successURL, err := f.fetchRaw(ctx, urls)
	if err != nil {
		return nil, err
	}
	return parseToolIndex(body, toolName, dirURL(successURL))
}

// FetchAllTools fetches metadata for all known tools.
func (f *MetadataFetcher) FetchAllTools(ctx context.Context) ([]ToolInfo, error) {
	toolNames := []string{"qtcreator", "cmake", "ifw", "mingw", "llvm_mingw", "ninja", "openssl", "vcredist"}
	var tools []ToolInfo
	for _, name := range toolNames {
		ctxErr := ctx.Err()
		if ctxErr != nil {
			return tools, ctxErr
		}
		tool, err := f.FetchTool(ctx, name)
		if err != nil {
			// Non-fatal: skip tools that fail to fetch, but warn so the failure is diagnosable.
			fmt.Fprintf(os.Stderr, "warning: fetching tool %s: %v\n", name, err)
			continue
		}
		tools = append(tools, *tool)
	}
	return tools, nil
}

// fetchExtensions fetches extension modules (qtwebengine, qtpdf) for Qt 6.8+
// and merges them into vi as regular add-on modules.
func (f *MetadataFetcher) fetchExtensions(ctx context.Context, vi *QtVersionInfo) {
	if !isQt68Plus(vi.Version) {
		return
	}
	if vi.PackageArchives == nil {
		vi.PackageArchives = make(map[string][]ArchiveRef)
	}

	verStr := versionToRepoStr(vi.Version, vi.Major)
	prefix := fmt.Sprintf("qt.qt%d.%s", vi.Major, verStr)

	for _, moduleName := range ExtensionModuleNames() {
		for _, arch := range vi.Archs {
			urls := f.mirrors.ExtensionURLsFor(moduleName, vi.Version, vi.Major, arch.Name)
			if len(urls) == 0 {
				continue
			}
			body, successURL, err := f.fetchRaw(ctx, urls)
			if err != nil {
				// Extension not available for this arch - skip silently.
				continue
			}
			f.processExtensionXML(body, dirURL(successURL), vi, moduleName, arch.Name, prefix)
		}

		// Register the module in vi.Modules if any archives were found.
		addonKeyPrefix := prefix + ".addons." + moduleName + "."
		hasArchives := false
		for key := range vi.PackageArchives {
			if strings.HasPrefix(key, addonKeyPrefix) {
				hasArchives = true
				break
			}
		}
		if hasArchives {
			found := false
			for _, m := range vi.Modules {
				if m.Name == moduleName {
					found = true
					break
				}
			}
			if !found {
				vi.Modules = append(vi.Modules, Module{
					Name:        moduleName,
					DisplayName: extensionDisplayName(moduleName),
				})
			}
		}
	}

	// Keep modules sorted alphabetically.
	slices.SortFunc(vi.Modules, func(a, b Module) int {
		return strings.Compare(a.Name, b.Name)
	})
}

// processExtensionXML parses an extension Updates.xml and stores archives
// in vi.PackageArchives using the standard addon key format so the resolver
// works unchanged.
func (f *MetadataFetcher) processExtensionXML(
	body []byte,
	baseURL string,
	vi *QtVersionInfo,
	moduleName, arch, prefix string,
) {
	var upd updatesXML
	err := xml.Unmarshal(body, &upd)
	if err != nil {
		return
	}

	for _, pkg := range upd.Packages {
		if pkg.DownloadableArchives == "" {
			continue
		}
		refs := buildArchiveRefs(pkg, baseURL)
		if len(refs) == 0 {
			continue
		}
		// Map extension package key to standard addon key.
		// e.g. store as "qt.qt6.6102.addons.qtwebengine.win64_msvc2022_64"
		addonKey := prefix + ".addons." + moduleName + "." + arch
		vi.PackageArchives[addonKey] = append(vi.PackageArchives[addonKey], refs...)
	}
}

// extensionDisplayName returns a human-readable name for an extension module.
func extensionDisplayName(name string) string {
	switch name {
	case "qtwebengine":
		return "Qt WebEngine"
	case "qtpdf":
		return "Qt PDF"
	default:
		return name
	}
}

// fetchSrcDocExamples fetches the separate src/doc/examples Updates.xml and
// populates vi.PackageArchives and feature flags (HasDocs, HasExamples, HasSources).
// This is needed because Qt 6.8+ moved these packages out of the main Updates.xml.
// Pre-6.8 Qt 6 versions also have a separate _src_doc_examples repository.
// Errors are silently ignored (non-fatal, like extensions).
func (f *MetadataFetcher) fetchSrcDocExamples(ctx context.Context, vi *QtVersionInfo) {
	if vi.Major < 6 {
		return
	}
	urls := f.mirrors.SrcDocExURLsFor(vi.Version, vi.Major)
	if len(urls) == 0 {
		return
	}
	body, successURL, err := f.fetchRaw(ctx, urls)
	if err != nil {
		return
	}

	var upd updatesXML
	err = xml.Unmarshal(body, &upd)
	if err != nil {
		return
	}

	if vi.PackageArchives == nil {
		vi.PackageArchives = make(map[string][]ArchiveRef)
	}

	baseURL := dirURL(successURL)
	for _, pkg := range upd.Packages {
		if pkg.DownloadableArchives == "" {
			continue
		}
		refs := buildArchiveRefs(pkg, baseURL)
		if len(refs) == 0 {
			continue
		}
		vi.PackageArchives[pkg.Name] = refs

		// Set feature flags.
		nameLower := strings.ToLower(pkg.Name)
		if strings.Contains(nameLower, ".doc.") || strings.HasSuffix(nameLower, ".doc") {
			vi.HasDocs = true
		}
		if strings.Contains(nameLower, ".examples.") || strings.HasSuffix(nameLower, ".examples") {
			vi.HasExamples = true
		}
		if strings.Contains(nameLower, ".src") || strings.Contains(nameLower, "sources") {
			vi.HasSources = true
		}
	}
}

func (f *MetadataFetcher) fetchFromURLs(ctx context.Context, urls []string) (*RepoIndex, error) {
	body, successURL, err := f.fetchRaw(ctx, urls)
	if err != nil {
		return nil, err
	}
	return parseRepoIndex(body, dirURL(successURL))
}

func (f *MetadataFetcher) fetchRaw(ctx context.Context, urls []string) ([]byte, string, error) {
	if len(urls) == 0 {
		return nil, "", errors.New("no URLs provided")
	}

	// Use a mirror-independent cache key so all mirrors share one cache entry.
	cacheKey := cacheKeyFromURL(urls[0])

	etag := f.cache.ETag(cacheKey)

	var lastErr error
	for _, url := range urls {
		body, newETag, fetchErr := f.client.FetchWithETag(ctx, url, etag)
		if fetchErr != nil {
			lastErr = fmt.Errorf("GET %s: %w", url, fetchErr)
			continue
		}
		if body == nil {
			// 304 Not Modified - serve from cache.
			cached, cacheErr := f.cache.Load(cacheKey)
			if cacheErr == nil && cached != nil {
				return cached, url, nil
			}
		} else {
			_ = f.cache.Store(cacheKey, body, newETag)
			return body, url, nil
		}
	}

	// All mirrors failed - try stale cache.
	stale, cacheErr := f.cache.LoadStale(cacheKey)
	if cacheErr == nil && stale != nil {
		fmt.Fprintf(os.Stderr, "warning: serving stale cached metadata (network unavailable)\n")
		return stale, urls[0], nil
	}

	if lastErr != nil {
		return nil, "", fmt.Errorf("all mirrors failed (last error: %w)\n\nURLs tried:\n%s", lastErr, urlList(urls))
	}
	return nil, "", fmt.Errorf("all mirrors failed and no cached data available\n\nURLs tried:\n%s", urlList(urls))
}

// cacheKeyFromURL strips the mirror base URL, returning the mirror-independent
// path portion starting from "online/". If the marker is not found, the full
// URL is returned as a fallback.
func cacheKeyFromURL(rawURL string) string {
	const marker = "online/"
	if idx := strings.Index(rawURL, marker); idx >= 0 {
		return rawURL[idx:]
	}
	return rawURL
}

// dirURL returns the directory portion of a URL (strips the last path component).
// e.g. "https://host/path/Updates.xml" -> "https://host/path/"
func dirURL(u string) string {
	if idx := strings.LastIndex(u, "/"); idx >= 0 {
		return u[:idx+1]
	}
	return u + "/"
}

func urlList(urls []string) string {
	var sb strings.Builder
	for _, u := range urls {
		sb.WriteString("  ")
		sb.WriteString(u)
		sb.WriteString("\n")
	}
	return sb.String()
}

// parseRepoIndex parses an Updates.xml body into a RepoIndex.
// baseURL is the directory URL of the Updates.xml file (used to construct archive download URLs).
// Pass "" if archive URLs are not needed (e.g. in tests or version-listing paths).
func parseRepoIndex(body []byte, baseURL string) (*RepoIndex, error) {
	var upd updatesXML
	err := xml.Unmarshal(body, &upd)
	if err != nil {
		return nil, fmt.Errorf("parsing Updates.xml: %w", err)
	}

	idx := &RepoIndex{}
	versionMap := map[string]*QtVersionInfo{}
	toolMap := map[string]*ToolInfo{}

	for _, pkg := range upd.Packages {
		virtual := isVirtual(pkg.Virtual)

		switch classifyPackage(pkg.Name) {
		case pkgClassQt:
			// Process all Qt packages - virtual and non-virtual.
			// Virtual packages are skipped for module/arch registration but their
			// archive refs are still collected (needed for Qt 6.8+ addon packages).
			processQtPackage(pkg, versionMap, baseURL, virtual)
		case pkgClassTool:
			if !virtual {
				processToolPackage(pkg, toolMap, baseURL)
			}
		case pkgClassOther:
			// Unknown package class; skip.
		}
	}

	for _, v := range versionMap {
		idx.QtVersions = append(idx.QtVersions, *v)
	}
	for _, t := range toolMap {
		idx.Tools = append(idx.Tools, *t)
	}
	return idx, nil
}

type pkgClass int

const (
	pkgClassQt pkgClass = iota
	pkgClassTool
	pkgClassOther
)

func classifyPackage(name string) pkgClass {
	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return pkgClassOther
	}
	if parts[0] == "qt" && (strings.HasPrefix(parts[1], "qt5") || strings.HasPrefix(parts[1], "qt6")) {
		return pkgClassQt
	}
	if parts[0] == "qt" && parts[1] == "tools" {
		return pkgClassTool
	}
	return pkgClassOther
}

func processQtPackage(pkg packageXML, versionMap map[string]*QtVersionInfo, baseURL string, virtual bool) {
	// Name format: qt.qt6.6100.win64_msvc2022_64
	// or:          qt.qt6.6100.addons.qtcharts.win64_msvc2022_64
	parts := strings.Split(pkg.Name, ".")
	if len(parts) < 3 {
		return
	}
	// Extract version from parts[1]: "qt6" -> 6, parts[2]: "6100" -> "6.10.0"
	var major int
	switch {
	case strings.HasPrefix(parts[1], "qt6"):
		major = 6
	case strings.HasPrefix(parts[1], "qt5"):
		major = 5
	default:
		return
	}

	verStr := repoSuffixToVersion(parts[2], major)
	if verStr == "" {
		return
	}

	vi, ok := versionMap[verStr]
	if !ok {
		vi = &QtVersionInfo{
			Version:         verStr,
			Major:           major,
			IsLTS:           qtmeta.IsLTS(verStr),
			ReleaseDate:     parseDate(pkg.ReleaseDate),
			PackageArchives: make(map[string][]ArchiveRef),
		}
		versionMap[verStr] = vi
	}

	// Store real archive refs for this package (works for both virtual and non-virtual).
	if refs := buildArchiveRefs(pkg, baseURL); len(refs) > 0 {
		if vi.PackageArchives == nil {
			vi.PackageArchives = make(map[string][]ArchiveRef)
		}
		vi.PackageArchives[pkg.Name] = refs
	}

	// The remainder of this function handles metadata registration (modules, targets,
	// feature flags). Virtual packages contribute archives but not metadata.
	if virtual {
		return
	}

	// Register addon module. Two naming schemes exist:
	//   Qt 6:  qt.qt6.683.addons.qt3d          (5-part, module at parts[4])
	//   Qt 5:  qt.qt5.5152.qtcharts             (4-part, module at parts[3])
	// This must happen before the arch check so module-only packages are not skipped.
	var moduleName string
	if len(parts) >= 5 && parts[3] == "addons" {
		moduleName = parts[4]
	} else if len(parts) == 4 && extractTarget(parts) == "" {
		candidate := parts[3]
		skip := map[string]bool{"doc": true, "examples": true, "sources": true, "debug_info": true, "addons": true}
		if !skip[candidate] {
			moduleName = candidate
		}
	}
	if moduleName != "" {
		displayName := pkg.DisplayName
		if displayName == "" {
			displayName = moduleName
		}
		foundM := false
		for i, m := range vi.Modules {
			if m.Name == moduleName {
				foundM = true
				// Update DisplayName if it was previously set to the fallback (raw name).
				if vi.Modules[i].DisplayName == moduleName && displayName != moduleName {
					vi.Modules[i].DisplayName = displayName
				}
				break
			}
		}
		if !foundM {
			vi.Modules = append(vi.Modules, Module{Name: moduleName, DisplayName: displayName})
		}
	}

	// Determine arch (last part after version/addons).
	arch := extractTarget(parts)
	if arch == "" {
		// Check for doc/examples/sources by package name suffix (no arch needed).
		setVersionFeatureFlags(vi, pkg.Name)
		// Store archives so they can be resolved for download.
		if refs := buildArchiveRefs(pkg, baseURL); len(refs) > 0 {
			vi.PackageArchives[pkg.Name] = refs
		}
		return
	}

	// Find or add the arch entry.
	var archEntry *Arch
	for i := range vi.Archs {
		if vi.Archs[i].Name == arch {
			archEntry = &vi.Archs[i]
			break
		}
	}
	if archEntry == nil {
		displayName := arch
		meta, found := qtmeta.LookupTarget(arch)
		if found {
			displayName = meta.DisplayName
		}
		vi.Archs = append(vi.Archs, Arch{Name: arch, DisplayName: displayName})
		archEntry = &vi.Archs[len(vi.Archs)-1]
	}

	// For essential (non-addon) arch packages, derive essential module names from
	// the DownloadableArchives field. Archive names follow the pattern:
	//   "qtbase-Windows-...-X86_64.7z" -> module name = prefix before first "-".
	// Essential modules are stored per-arch so that MinGW/LLVM archives do not
	// appear as essentials when an MSVC target is selected.
	if len(parts) < 5 || parts[3] != "addons" {
		for archive := range strings.SplitSeq(pkg.DownloadableArchives, ",") {
			archive = strings.TrimSpace(archive)
			if archive == "" {
				continue
			}
			if dashIdx := strings.IndexByte(archive, '-'); dashIdx > 0 {
				modName := archive[:dashIdx]
				if !slices.Contains(archEntry.EssentialModules, modName) {
					archEntry.EssentialModules = append(archEntry.EssentialModules, modName)
				}
			}
		}
	}

	// Check for doc/examples/sources/debug_info by package name suffix.
	setVersionFeatureFlags(vi, pkg.Name)
}

func processToolPackage(pkg packageXML, toolMap map[string]*ToolInfo, baseURL string) {
	// Name format: qt.tools.qtcreator or qt.tools.cmake.win64
	parts := strings.Split(pkg.Name, ".")
	if len(parts) < 3 {
		return
	}
	toolName := parts[2]

	tool, ok := toolMap[toolName]
	if !ok {
		tool = &ToolInfo{Name: toolName, Display: pkg.DisplayName}
		toolMap[toolName] = tool
	}

	tv := ToolVersionInfo{
		Version:     pkg.Version,
		DisplayName: pkg.DisplayName,
		ReleaseDate: parseDate(pkg.ReleaseDate),
		Archives:    buildArchiveRefs(pkg, baseURL),
	}
	tool.Versions = append(tool.Versions, tv)
}

func parseToolIndex(body []byte, toolName, baseURL string) (*ToolInfo, error) {
	idx, err := parseRepoIndex(body, baseURL)
	if err != nil {
		return nil, err
	}
	for _, t := range idx.Tools {
		if t.Name == toolName {
			return &t, nil
		}
	}
	// Return empty if not found in parsed data.
	return &ToolInfo{Name: toolName}, nil
}

// setVersionFeatureFlags inspects a package name and sets the corresponding
// feature flags (HasDocs, HasExamples, HasSources, HasDebugInfo) on vi.
func setVersionFeatureFlags(vi *QtVersionInfo, pkgName string) {
	nameLower := strings.ToLower(pkgName)
	if strings.Contains(nameLower, ".doc.") || strings.HasSuffix(nameLower, ".doc") {
		vi.HasDocs = true
	}
	if strings.Contains(nameLower, ".examples.") || strings.HasSuffix(nameLower, ".examples") {
		vi.HasExamples = true
	}
	if strings.Contains(nameLower, "sources") || strings.Contains(nameLower, "_src") {
		vi.HasSources = true
	}
	if strings.Contains(nameLower, "debug_info") || strings.Contains(nameLower, "debuginfo") {
		vi.HasDebugInfo = true
	}
}

// buildArchiveRefs constructs ArchiveRef values for every downloadable archive
// in pkg, using baseURL to form the full download URL.
//
// Qt stores archive files on disk with a version+timestamp prefix:
//
//	URL = baseURL + pkgName + "/" + pkg.Version + archiveFilename
//
// e.g. "6.10.1-0-202511161843qtbase-Windows-...7z"
// The DownloadableArchives field omits this prefix; pkg.Version supplies it.
func buildArchiveRefs(pkg packageXML, baseURL string) []ArchiveRef {
	if pkg.DownloadableArchives == "" {
		return nil
	}

	// UpdateFile FileName attributes use the version-prefixed name on disk.
	sha1ByFile := make(map[string]string)
	sizeByFile := make(map[string]int64)
	for _, uf := range pkg.UpdateFiles {
		sha1ByFile[uf.FileName] = uf.SHA1
		sz, parseErr := strconv.ParseInt(uf.CompressedSize, 10, 64)
		if parseErr == nil {
			sizeByFile[uf.FileName] = sz
		}
	}

	// Fallback: parallel ArchiveSHA1 field (comma-separated, same order as DownloadableArchives).
	sha1Fallback := strings.Split(pkg.SHA1, ",")

	var refs []ArchiveRef
	for i, raw := range strings.Split(pkg.DownloadableArchives, ",") {
		filename := strings.TrimSpace(raw)
		if filename == "" {
			continue
		}

		// The file on disk is named {pkg.Version}{filename} when Version is set.
		diskFilename := filename
		if pkg.Version != "" {
			diskFilename = pkg.Version + filename
		}

		sha1 := sha1ByFile[diskFilename]
		if sha1 == "" {
			sha1 = sha1ByFile[filename] // fallback for schemas without version prefix
		}
		if sha1 == "" && i < len(sha1Fallback) {
			sha1 = strings.TrimSpace(sha1Fallback[i])
		}

		refs = append(refs, ArchiveRef{
			URL:      baseURL + pkg.Name + "/" + diskFilename,
			Filename: diskFilename,
			SHA1:     sha1,
			Size:     sizeByFile[diskFilename],
		})
	}
	return refs
}

// repoSuffixToVersion converts "6100" -> "6.10.0", "51518" -> "5.15.18".
func repoSuffixToVersion(suffix string, major int) string {
	// suffix is compact version with no dots: e.g. "6100" for 6.10.0, "51518" for 5.15.18
	majorStr := strconv.Itoa(major)
	if !strings.HasPrefix(suffix, majorStr) {
		return ""
	}
	rest := suffix[len(majorStr):]
	if len(rest) < 1 {
		return ""
	}
	// Try to parse as minor + patch.
	// For Qt 6: "6100" -> major=6, rest="100" -> minor=10, patch=0
	// For Qt 5: "51518" -> major=5, rest="1518" -> minor=15, patch=18
	// Heuristic: split rest into (minor_digits, patch_digits).
	// We try 2-digit minor first, then 1-digit.
	for minorLen := 2; minorLen >= 1; minorLen-- {
		if len(rest) < minorLen {
			continue
		}
		minorStr := rest[:minorLen]
		patchStr := rest[minorLen:]
		minor, err1 := strconv.Atoi(minorStr)
		if err1 != nil {
			continue
		}
		patch := 0
		if patchStr != "" {
			var err2 error
			patch, err2 = strconv.Atoi(patchStr)
			if err2 != nil {
				continue
			}
		} else if len(rest) != 1 {
			// Empty patch is only valid when rest is a single digit (Qt 5.9.0 -> "qt5_59").
			// For longer rest strings like "83", prefer the next minorLen iteration.
			continue
		}
		// Sanity check.
		if minor >= 0 && minor <= 99 && patch >= 0 && patch <= 99 {
			return fmt.Sprintf("%d.%d.%d", major, minor, patch)
		}
	}
	return ""
}

func extractTarget(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	// Target is typically the last component.
	last := parts[len(parts)-1]
	// Skip if it's a well-known non-target component.
	skip := map[string]bool{"doc": true, "examples": true, "sources": true, "debug_info": true, "addons": true}
	if skip[last] {
		return ""
	}
	// Targets contain underscores and look like "win64_msvc2022_64".
	if strings.Contains(last, "_") || last == "macos" {
		return last
	}
	return ""
}

func isVirtual(s string) bool {
	return strings.EqualFold(strings.TrimSpace(s), "true")
}

func parseDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}
	}
	return t
}
