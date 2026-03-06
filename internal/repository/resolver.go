package repository

import (
	"context"
	"fmt"
	"strings"

	qerr "github.com/trollixx/qvm/internal/errors"
)

// ResolveOptions configures what archives to resolve for a Qt install.
type ResolveOptions struct {
	Version        string
	Arch           string   // e.g. "win64_msvc2022_64"
	TargetPlatform string   // e.g. "desktop", "android", "wasm"; defaults to "desktop"
	Modules        []string // add-on module names; nil = essentials only
	Docs           []string // "*" = all selected modules, or specific names
	Examples       []string // same semantics as Docs
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

	// Add-on module archives.
	for _, mod := range opts.Modules {
		addonPkg := prefix + ".addons." + mod + "." + opts.Arch
		if refs, ok := vi.PackageArchives[addonPkg]; ok {
			for _, ref := range refs {
				archives = append(archives, ResolvedArchive{Name: addonPkg, Ref: ref})
			}
			continue
		}
		// Auto-prefix "qt" and retry.
		if !strings.HasPrefix(mod, "qt") {
			qtMod := "qt" + mod
			addonPkg = prefix + ".addons." + qtMod + "." + opts.Arch
			if refs, ok := vi.PackageArchives[addonPkg]; ok {
				for _, ref := range refs {
					archives = append(archives, ResolvedArchive{Name: addonPkg, Ref: ref})
				}
				continue
			}
		}
		var available []string
		for _, m := range vi.ModulesForArch(opts.Arch) {
			available = append(available, m.Name)
		}
		return nil, qerr.SuggestModule(mod, available)
	}

	// Documentation.
	if len(opts.Docs) > 0 {
		archives = append(archives, resolveDocArchives(vi, prefix, opts.Docs)...)
	}

	// Examples.
	if len(opts.Examples) > 0 {
		archives = append(archives, resolveExamplesArchives(vi, prefix, opts.Examples)...)
	}

	// Sources.
	if opts.Sources {
		archives = append(archives, resolveSourcesArchives(vi, prefix)...)
	}

	// Debug info.
	if opts.DebugInfo {
		archives = append(archives, resolveDebugInfoArchives(vi, prefix, opts.Arch)...)
	}

	if len(archives) == 0 {
		return nil, fmt.Errorf("no archives resolved for Qt %s %s", opts.Version, opts.Arch)
	}

	return archives, nil
}

func resolveDocArchives(vi *QtVersionInfo, prefix string, modules []string) []ResolvedArchive {
	var result []ResolvedArchive
	if len(modules) == 1 && modules[0] == "*" {
		for pkg, refs := range vi.PackageArchives {
			if strings.HasPrefix(pkg, prefix+".doc.") {
				for _, ref := range refs {
					result = append(result, ResolvedArchive{Name: pkg, Ref: ref})
				}
			}
		}
	} else {
		for _, mod := range modules {
			pkg := prefix + ".doc." + mod
			if refs, ok := vi.PackageArchives[pkg]; ok {
				for _, ref := range refs {
					result = append(result, ResolvedArchive{Name: pkg, Ref: ref})
				}
			}
		}
	}
	return result
}

func resolveExamplesArchives(vi *QtVersionInfo, prefix string, modules []string) []ResolvedArchive {
	var result []ResolvedArchive
	if len(modules) == 1 && modules[0] == "*" {
		for pkg, refs := range vi.PackageArchives {
			if strings.HasPrefix(pkg, prefix+".examples.") {
				for _, ref := range refs {
					result = append(result, ResolvedArchive{Name: pkg, Ref: ref})
				}
			}
		}
	} else {
		for _, mod := range modules {
			pkg := prefix + ".examples." + mod
			if refs, ok := vi.PackageArchives[pkg]; ok {
				for _, ref := range refs {
					result = append(result, ResolvedArchive{Name: pkg, Ref: ref})
				}
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

func resolveDebugInfoArchives(vi *QtVersionInfo, prefix, arch string) []ResolvedArchive {
	var result []ResolvedArchive
	for pkg, refs := range vi.PackageArchives {
		if strings.HasPrefix(pkg, prefix) && (strings.Contains(pkg, "debug_info") || strings.Contains(pkg, "debuginfo")) {
			if arch == "" || strings.HasSuffix(pkg, "."+arch) || strings.Contains(pkg, "."+arch+".") {
				for _, ref := range refs {
					result = append(result, ResolvedArchive{Name: pkg, Ref: ref})
				}
			}
		}
	}
	return result
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
