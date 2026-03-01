package repository

import "time"

// ArchiveRef describes a single downloadable archive within a package.
type ArchiveRef struct {
	URL      string `json:"url"`
	Filename string `json:"filename"`           // e.g. "qtbase-Windows-...7z"
	SHA1     string `json:"sha1"`               // Expected SHA1 hex digest
	Size     int64  `json:"size,omitempty"`     // Compressed size in bytes (may be 0 if unknown)
}

// Module describes an installable Qt module.
type Module struct {
	Name        string       `json:"name"`                  // e.g. "qtcharts"
	DisplayName string       `json:"display_name"`          // e.g. "Qt Charts"
	IsEssential bool         `json:"is_essential,omitempty"` // true = always installed (set when building an arch-specific list)
	Archives    []ArchiveRef `json:"archives,omitempty"`    // archives for this module (may depend on arch)
}

// Arch describes a build arch (OS + compiler combination).
type Arch struct {
	Name             string   `json:"name"`                        // e.g. "win64_msvc2022_64"
	DisplayName      string   `json:"display_name"`                // e.g. "MSVC 2022, 64-bit"
	IsDefault        bool     `json:"is_default,omitempty"`        // true = recommended for the current host
	EssentialModules []string `json:"essential_modules,omitempty"` // module names derived from this arch's DownloadableArchives
}

// QtVersionInfo describes an available Qt SDK version.
type QtVersionInfo struct {
	Version      string    `json:"version"`
	Major        int       `json:"major"`
	IsLTS        bool      `json:"is_lts,omitempty"`
	ReleaseDate  time.Time `json:"release_date,omitempty"`
	Archs        []Arch    `json:"archs,omitempty"`
	Modules      []Module  `json:"modules,omitempty"` // Add-on modules only (not essentials; those are per-arch)
	HasDocs      bool      `json:"has_docs,omitempty"`
	HasExamples  bool      `json:"has_examples,omitempty"`
	HasSources   bool      `json:"has_sources,omitempty"`
	HasDebugInfo bool      `json:"has_debug_info,omitempty"`
	// PackageArchives maps Qt package name → archive list (internal; not exposed in JSON output).
	// Keys follow the Updates.xml package naming convention, e.g.:
	//   "qt.qt6.6101.win64_msvc2022_64"             (essential bundle)
	//   "qt.qt6.6101.addons.qtcharts.win64_msvc2022_64" (addon module)
	//   "qt.qt6.6101.doc.qtcharts"                   (documentation)
	PackageArchives map[string][]ArchiveRef `json:"-"`
}

// SetDefaultArch marks the named arch as IsDefault (and clears the flag on all others).
// Call this after fetching metadata, once the platform default is known.
func (vi *QtVersionInfo) SetDefaultArch(name string) {
	for i := range vi.Archs {
		vi.Archs[i].IsDefault = vi.Archs[i].Name == name
	}
}

// ModulesForArch returns the full module list for a specific arch: the
// arch's essential modules (as Module{IsEssential: true}) followed by all
// add-on modules. Returns only add-ons if the arch is not found.
func (vi *QtVersionInfo) ModulesForArch(archName string) []Module {
	var result []Module
	for _, a := range vi.Archs {
		if a.Name == archName {
			for _, name := range a.EssentialModules {
				result = append(result, Module{
					Name:        name,
					DisplayName: name,
					IsEssential: true,
				})
			}
			break
		}
	}
	result = append(result, vi.Modules...)
	return result
}

// ToolVersionInfo describes one installable version of a tool.
type ToolVersionInfo struct {
	Version     string       `json:"version"`
	DisplayName string       `json:"display_name"`
	ReleaseDate time.Time    `json:"release_date,omitempty"`
	Archives    []ArchiveRef `json:"archives,omitempty"`
}

// ToolInfo describes an installable tool (e.g. qtcreator, cmake).
type ToolInfo struct {
	Name     string            `json:"name"`              // e.g. "qtcreator"
	Display  string            `json:"display"`           // e.g. "Qt Creator"
	Versions []ToolVersionInfo `json:"versions,omitempty"`
}

// RepoIndex holds the full parsed repository state.
type RepoIndex struct {
	QtVersions []QtVersionInfo `json:"qt_versions,omitempty"`
	Tools      []ToolInfo      `json:"tools,omitempty"`
}

// PackageUpdate mirrors one <PackageUpdate> element from Updates.xml.
type PackageUpdate struct {
	Name                 string
	DisplayName          string
	Description          string
	Version              string
	ReleaseDate          time.Time
	DownloadableArchives []string
	ArchiveSHA1          []string // parallel to DownloadableArchives
	Dependencies         []string
	Virtual              bool
	SizeCompressed       int64
	SizeUncompressed     int64
}

// ArchiveEntry holds SHA1+size info for a single archive within a PackageUpdate.
type ArchiveEntry struct {
	Filename         string
	CompressedSize   int64
	UncompressedSize int64
	SHA1             string
}
