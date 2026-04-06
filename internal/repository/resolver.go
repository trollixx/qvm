package repository

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	qerr "github.com/trollixx/qvm/internal/errors"
)

// ResolveOptions configures what archives to resolve for a Qt install.
type ResolveOptions struct {
	Version        string
	Arch           string   // e.g. "win64_msvc2022_64"
	TargetPlatform string   // e.g. "desktop", "android", "wasm"; defaults to "desktop"
	Modules        []string // add-on module names to install (delta - only new ones)
	AllModules     []string // all module names (existing + new) for scoping docs/examples
	Docs           bool
	Examples       bool
	Sources        bool
	DebugInfo      bool
	SkipEssentials bool // true when the essential bundle is already installed
}

// ResolvedArchive is a single archive selected for download.
type ResolvedArchive struct {
	Name string // package name, e.g. "qt.qt6.6100.win64_msvc2022_64"
	Ref  ArchiveRef
}

// Resolver translates user intent into a concrete list of archives to download.
type Resolver struct {
	fetcher *MetadataFetcher
}

// NewResolver creates a Resolver backed by the given MetadataFetcher.
func NewResolver(fetcher *MetadataFetcher) *Resolver {
	return &Resolver{fetcher: fetcher}
}

// Resolve returns the archives needed for the given options.
func (r *Resolver) Resolve(ctx context.Context, opts ResolveOptions) ([]ResolvedArchive, error) {
	idx, err := r.fetcher.FetchQtVersion(ctx, opts.Version)
	if err != nil {
		return nil, fmt.Errorf("fetching metadata for Qt %s: %w", opts.Version, err)
	}

	// Find the requested version.
	var vi *QtVersionInfo
	for i := range idx.QtVersions {
		if idx.QtVersions[i].Version == opts.Version {
			vi = &idx.QtVersions[i]
			break
		}
	}
	if vi == nil {
		available := make([]string, 0, len(idx.QtVersions))
		for _, v := range idx.QtVersions {
			available = append(available, v.Version)
		}
		return nil, qerr.SuggestVersion(opts.Version, available)
	}

	// Validate arch.
	if opts.Arch != "" {
		found := false
		for _, a := range vi.Archs {
			if a.Name == opts.Arch {
				found = true
				break
			}
		}
		if !found {
			available := make([]string, 0, len(vi.Archs))
			for _, a := range vi.Archs {
				available = append(available, a.Name)
			}
			return nil, qerr.SuggestArch(opts.Arch, available)
		}
	}

	return resolveArchives(vi, opts)
}

// lookupAddonArchives finds archives for an addon module using the Qt 6
// package key format: prefix.addons.mod.arch.
func lookupAddonArchives(vi *QtVersionInfo, prefix, mod, arch string) (bool, []ResolvedArchive) {
	pkg := prefix + ".addons." + mod + "." + arch
	refs, ok := vi.PackageArchives[pkg]
	if !ok {
		return false, nil
	}
	result := make([]ResolvedArchive, 0, len(refs))
	for _, ref := range refs {
		result = append(result, ResolvedArchive{Name: pkg, Ref: ref})
	}
	return true, result
}

// resolveArchives resolves the concrete archives from a QtVersionInfo and options.
func resolveArchives(vi *QtVersionInfo, opts ResolveOptions) ([]ResolvedArchive, error) {
	major := vi.Major
	verStr := versionToRepoStr(opts.Version, major)
	prefix := fmt.Sprintf("qt.qt%d.%s", major, verStr)

	var archives []ResolvedArchive

	// Essential archives for the arch (skipped when already installed).
	if !opts.SkipEssentials {
		essentialPkg := prefix + "." + opts.Arch
		if refs, ok := vi.PackageArchives[essentialPkg]; ok {
			for _, ref := range refs {
				archives = append(archives, ResolvedArchive{Name: essentialPkg, Ref: ref})
			}
		}
	}

	// Build set of essential module names so we can skip them if requested.
	essentialSet := make(map[string]bool)
	for _, a := range vi.Archs {
		if a.Name == opts.Arch {
			for _, name := range a.EssentialModules {
				essentialSet[name] = true
			}
			break
		}
	}

	// Add-on module archives.
	for _, mod := range opts.Modules {
		if found, res := lookupAddonArchives(vi, prefix, mod, opts.Arch); found {
			archives = append(archives, res...)
			continue
		}
		// Auto-prefix "qt" and retry.
		qtMod := mod
		if !strings.HasPrefix(mod, "qt") {
			qtMod = "qt" + mod
			if found, res := lookupAddonArchives(vi, prefix, qtMod, opts.Arch); found {
				archives = append(archives, res...)
				continue
			}
		}
		// Module is already part of the essential bundle - skip silently.
		if essentialSet[mod] || essentialSet[qtMod] {
			continue
		}
		var available []string
		for _, m := range vi.ModulesForArch(opts.Arch) {
			available = append(available, m.Name)
		}
		return nil, qerr.SuggestModule(mod, available)
	}

	// Build the full module list for scoping docs/examples.
	// Use AllModules if provided (delta install), otherwise derive from
	// the arch's essentials + requested addons.
	allModules := opts.AllModules
	if len(allModules) == 0 {
		for _, a := range vi.Archs {
			if a.Name == opts.Arch {
				allModules = append(allModules, a.EssentialModules...)
				break
			}
		}
		allModules = append(allModules, opts.Modules...)
	}

	// Documentation - scoped to all installed modules.
	if opts.Docs {
		archives = append(archives, resolveModuleScopedArchives(vi, prefix, "doc", allModules)...)
	}

	// Examples - scoped to all installed modules.
	if opts.Examples {
		archives = append(archives, resolveModuleScopedArchives(vi, prefix, "examples", allModules)...)
	}

	// Sources.
	if opts.Sources {
		archives = append(archives, resolveSourcesArchives(vi, prefix)...)
	}

	// Debug info - scoped to all installed modules.
	if opts.DebugInfo {
		archives = append(archives, resolveDebugInfoArchives(vi, prefix, opts.Arch, allModules)...)
	}

	if len(archives) == 0 {
		return nil, fmt.Errorf("no archives resolved for Qt %s %s", opts.Version, opts.Arch)
	}

	return archives, nil
}

// resolveModuleScopedArchives resolves archives for a given package segment
// (e.g. "doc" or "examples") scoped to the provided list of modules.
func resolveModuleScopedArchives(vi *QtVersionInfo, prefix, segment string, modules []string) []ResolvedArchive {
	var result []ResolvedArchive
	for _, mod := range modules {
		pkg := prefix + "." + segment + "." + mod
		if refs, ok := vi.PackageArchives[pkg]; ok {
			for _, ref := range refs {
				result = append(result, ResolvedArchive{Name: pkg, Ref: ref})
			}
		}
	}
	return result
}

func resolveSourcesArchives(vi *QtVersionInfo, prefix string) []ResolvedArchive {
	var result []ResolvedArchive
	for pkg, refs := range vi.PackageArchives {
		if strings.HasPrefix(pkg, prefix) {
			if strings.Contains(pkg, ".src") || strings.Contains(pkg, "sources") {
				for _, ref := range refs {
					result = append(result, ResolvedArchive{Name: pkg, Ref: ref})
				}
			}
		}
	}
	return result
}

func resolveDebugInfoArchives(vi *QtVersionInfo, prefix, arch string, modules []string) []ResolvedArchive {
	// Build a set of installed module names for filtering individual archives.
	moduleSet := make(map[string]bool, len(modules))
	for _, m := range modules {
		moduleSet[m] = true
	}

	var result []ResolvedArchive
	for pkg, refs := range vi.PackageArchives {
		if !strings.HasPrefix(pkg, prefix) ||
			(!strings.Contains(pkg, "debug_info") && !strings.Contains(pkg, "debuginfo")) {
			continue
		}
		if arch != "" && !strings.HasSuffix(pkg, "."+arch) && !strings.Contains(pkg, "."+arch+".") {
			continue
		}
		for _, ref := range refs {
			if len(moduleSet) > 0 {
				modName := archiveModuleName(ref.Filename)
				if modName != "" && !moduleSet[modName] {
					continue
				}
			}
			result = append(result, ResolvedArchive{Name: pkg, Ref: ref})
		}
	}
	return result
}

// archiveModuleName extracts the Qt module name from an archive filename.
// Filenames follow the pattern: "{version_prefix}{module}-{OS}-...-debug-symbols.7z"
// e.g. "6.10.2-0-202601261212qtbase-Windows-...-debug-symbols.7z" -> "qtbase"
// or simply "qtbase-Windows-msvc.7z" -> "qtbase" (no version prefix).
func archiveModuleName(filename string) string {
	if filename == "" {
		return ""
	}
	// If the filename starts with a letter, there's no version prefix.
	if unicode.IsLetter(rune(filename[0])) {
		if dashIdx := strings.IndexByte(filename, '-'); dashIdx > 0 {
			return filename[:dashIdx]
		}
		return filename
	}
	// Skip the version-timestamp prefix by finding a digit-to-letter transition.
	for i := 1; i < len(filename); i++ {
		ch := filename[i]
		prev := filename[i-1]
		if unicode.IsLetter(rune(ch)) && unicode.IsDigit(rune(prev)) {
			rest := filename[i:]
			if dashIdx := strings.IndexByte(rest, '-'); dashIdx > 0 {
				return rest[:dashIdx]
			}
			return rest
		}
	}
	return ""
}

// ResolveTool resolves archives for a named tool at a specific version.
func (r *Resolver) ResolveTool(ctx context.Context, toolName, version string) ([]ResolvedArchive, error) {
	toolInfo, err := r.fetcher.FetchTool(ctx, toolName)
	if err != nil {
		return nil, fmt.Errorf("fetching tool %s: %w", toolName, err)
	}

	for _, tv := range toolInfo.Versions {
		if tv.Version == version {
			archives := make([]ResolvedArchive, 0, len(tv.Archives))
			for _, ref := range tv.Archives {
				archives = append(archives, ResolvedArchive{
					Name: toolName + "@" + version,
					Ref:  ref,
				})
			}
			if len(archives) == 0 {
				return nil, fmt.Errorf("tool %s@%s has no archives", toolName, version)
			}
			return archives, nil
		}
	}

	available := make([]string, 0, len(toolInfo.Versions))
	for _, tv := range toolInfo.Versions {
		available = append(available, tv.Version)
	}
	return nil, qerr.Newf(qerr.CodeUnknownTool, "tool %s version %q not found\n\nAvailable: %s",
		toolName, version, strings.Join(available, ", "))
}
