package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/trollixx/qvm/internal/config"
	"github.com/trollixx/qvm/internal/platform"
	"github.com/trollixx/qvm/internal/repository"
	"github.com/trollixx/qvm/internal/storage"
	"github.com/trollixx/qvm/pkg/qtmeta"
)

func (a *app) newListCommand() *cli.Command {
	return &cli.Command{
		Name:            "list",
		Aliases:         []string{"ls"},
		Usage:           "List installed or available Qt versions and tools",
		ArgsUsage:       "[qt | qt@<major>[.<minor>[.<patch>]] | tools]",
		CommandNotFound: showHelpOnNotFound,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "all",
				Usage: "show all available versions, not just installed",
			},
			newFormatFlag(),
			newHostFlag(),
		},
		Action: a.runList,
	}
}

func (a *app) runList(ctx context.Context, cmd *cli.Command) error {
	arg := cmd.Args().Get(0)
	showAll := cmd.Bool("all")
	format := cmd.String("format")
	host := cmd.String("host")

	// qt@<version> detail or filtered view - always fetches remote metadata.
	if version, ok := strings.CutPrefix(arg, "qt@"); ok {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		fetcher, err := buildFetcher(cfg, host)
		if err != nil {
			return fmt.Errorf("initializing fetcher: %w", err)
		}

		vf, err := qtmeta.ParseVersionFilter(version)
		if err != nil {
			return fmt.Errorf("invalid version %q: %w", version, err)
		}

		// Full version (e.g. "6.8.3") -> detail view.
		// Partial version (e.g. "6" or "6.9") -> filtered list view.
		if vf.IsFullVersion() {
			return a.runListQtVersion(ctx, fetcher, version, format)
		}

		registry, err := storage.NewRegistryManager()
		if err != nil {
			return fmt.Errorf("opening registry: %w", err)
		}
		reg, err := registry.Load()
		if err != nil {
			return fmt.Errorf("loading registry: %w", err)
		}
		return a.runListQtFiltered(ctx, fetcher, reg, vf, format)
	}

	if arg != "" && arg != "qt" && arg != listTargetTools {
		return fmt.Errorf("unknown list target %q\n\n"+
			"Usage:\n"+
			"  qvm list                 Show installed versions\n"+
			"  qvm list --all           Show all available versions\n"+
			"  qvm list qt@6            Show all Qt 6.x versions\n"+
			"  qvm list qt@6.8          Show all Qt 6.8.x versions\n"+
			"  qvm list qt@6.8.3        Show version details (archs, modules)", arg)
	}

	if showAll {
		return a.runListAll(ctx, arg, format, host)
	}
	return a.runListInstalled(arg, format)
}

// --- Installed-only views (default) ---

func (a *app) runListInstalled(arg, format string) error {
	registry, err := storage.NewRegistryManager()
	if err != nil {
		return fmt.Errorf("opening registry: %w", err)
	}
	reg, err := registry.Load()
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	if format == formatJSON {
		switch arg {
		case "qt":
			return a.printJSON(reg.Qt)
		case listTargetTools:
			return a.printJSON(reg.Tools)
		default:
			return a.printJSON(reg)
		}
	}

	switch arg {
	case "qt":
		a.printInstalledQt(reg)
	case listTargetTools:
		a.printInstalledTools(reg)
	default:
		a.printInstalledQt(reg)
		fmt.Fprintln(a.streams.Out)
		a.printInstalledTools(reg)
	}
	return nil
}

func (a *app) printInstalledQt(reg *storage.Registry) {
	fmt.Fprintln(a.streams.Out, "Installed Qt versions")

	if len(reg.Qt) == 0 {
		fmt.Fprintln(a.streams.Out, "  (none)")
		fmt.Fprintln(a.streams.Out)
		fmt.Fprintln(a.streams.Out, "  Install one with: qvm install qt@<version>")
		fmt.Fprintln(a.streams.Out, "  Run 'qvm list --all' to see available versions.")
		return
	}

	for _, q := range reg.Qt {
		size := ""
		if q.SizeBytes > 0 {
			size = formatSize(q.SizeBytes)
		}

		fmt.Fprintf(a.streams.Out, "  %-8s %-30s %-40s %s\n",
			q.Version, q.Arch, q.InstallDir, size)

		if len(q.Modules) > 0 {
			fmt.Fprintf(a.streams.Out, "          modules: %s\n", strings.Join(q.Modules, ", "))
		}

		extras := buildExtrasLine(q.Extras)
		if extras != "" {
			fmt.Fprintf(a.streams.Out, "          extras:  %s\n", extras)
		}
	}
}

func (a *app) printInstalledTools(reg *storage.Registry) {
	fmt.Fprintln(a.streams.Out, "Installed tools")

	if len(reg.Tools) == 0 {
		fmt.Fprintln(a.streams.Out, "  (none)")
		fmt.Fprintln(a.streams.Out)
		fmt.Fprintln(a.streams.Out, "  Run 'qvm list tools --all' to see available tools.")
		return
	}

	for _, t := range reg.Tools {
		size := ""
		if t.SizeBytes > 0 {
			size = formatSize(t.SizeBytes)
		}
		fmt.Fprintf(a.streams.Out, "  %-16s %-12s %-40s %s\n",
			t.Name, t.Version, t.InstallDir, size)
	}
}

func buildExtrasLine(extras storage.InstalledExtras) string {
	var parts []string
	if extras.Docs {
		parts = append(parts, "docs")
	}
	if extras.Examples {
		parts = append(parts, "examples")
	}
	if extras.Sources {
		parts = append(parts, "sources")
	}
	if extras.DebugInfo {
		parts = append(parts, "debug-symbols")
	}
	return strings.Join(parts, ", ")
}

// --- All-available views (with installed markers) ---

func (a *app) runListAll(ctx context.Context, arg, format, host string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	fetcher, err := buildFetcher(cfg, host)
	if err != nil {
		return fmt.Errorf("initializing fetcher: %w", err)
	}

	// Load registry for installed markers.
	registry, err := storage.NewRegistryManager()
	if err != nil {
		return fmt.Errorf("opening registry: %w", err)
	}
	reg, err := registry.Load()
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	switch arg {
	case "qt":
		return a.runListAllQt(ctx, fetcher, reg, format)
	case listTargetTools:
		return a.runListAllTools(ctx, fetcher, reg, format)
	default:
		err = a.runListAllQt(ctx, fetcher, reg, format)
		if err != nil {
			return err
		}
		fmt.Fprintln(a.streams.Out)
		return a.runListAllTools(ctx, fetcher, reg, format)
	}
}

func (a *app) runListAllQt(
	ctx context.Context,
	fetcher *repository.MetadataFetcher,
	reg *storage.Registry,
	format string,
) error {
	versions, err := fetcher.FetchAllQtVersions(ctx)
	if err != nil {
		return fmt.Errorf("fetching Qt versions: %w", err)
	}

	if format == formatJSON {
		return a.printJSON(versions)
	}

	// Build installed version lookup: version -> []arch.
	installedVersions := map[string][]string{}
	for _, q := range reg.Qt {
		installedVersions[q.Version] = append(installedVersions[q.Version], q.Arch)
	}

	// Group by major version.
	byMajor := map[int][]repository.QtVersionInfo{}
	for _, v := range versions {
		byMajor[v.Major] = append(byMajor[v.Major], v)
	}

	majors := []int{}
	for k := range byMajor {
		majors = append(majors, k)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(majors)))

	fmt.Fprintln(a.streams.Out, "Qt versions")

	for _, major := range majors {
		vers := byMajor[major]
		sortVersionsDesc(vers)

		fmt.Fprintf(a.streams.Out, "\nQt %d\n", major)

		recommendedVersion := newestLTSPatch(vers)

		for _, v := range vers {
			var label string
			switch {
			case v.IsPreview:
				label = "Preview"
			case v.IsLTS:
				label = "LTS"
			case v.Version == vers[0].Version:
				label = "Latest"
			}

			a.printVersionRow(v, label, installedVersions, recommendedVersion)
		}
	}

	fmt.Fprintln(a.streams.Out, "\nRun 'qvm list qt@<version>' to see available targets and modules.")
	return nil
}

func (a *app) runListQtFiltered(
	ctx context.Context,
	fetcher *repository.MetadataFetcher,
	reg *storage.Registry,
	vf qtmeta.VersionFilter,
	format string,
) error {
	versions, err := fetcher.FetchAllQtVersions(ctx)
	if err != nil {
		return fmt.Errorf("fetching Qt versions: %w", err)
	}

	var filtered []repository.QtVersionInfo
	for _, v := range versions {
		if vf.MatchesString(v.Version) {
			filtered = append(filtered, v)
		}
	}

	if len(filtered) == 0 {
		return withHint(
			fmt.Errorf("no Qt versions matching %q found", vf.String()),
			"Run 'qvm list --all' to see all available versions.",
		)
	}

	if format == formatJSON {
		return a.printJSON(filtered)
	}

	sortVersionsDesc(filtered)

	installedVersions := map[string][]string{}
	for _, q := range reg.Qt {
		installedVersions[q.Version] = append(installedVersions[q.Version], q.Arch)
	}

	recommendedVersion := newestLTSPatch(filtered)

	fmt.Fprintf(a.streams.Out, "Qt versions matching %s\n\n", vf.String())

	for _, v := range filtered {
		label := ""
		if v.IsPreview {
			label = "Preview"
		} else if v.IsLTS {
			label = "LTS"
		}

		a.printVersionRow(v, label, installedVersions, recommendedVersion)
	}

	fmt.Fprintln(a.streams.Out, "\nRun 'qvm list qt@<version>' to see available targets and modules.")
	return nil
}

func (a *app) runListAllTools(
	ctx context.Context,
	fetcher *repository.MetadataFetcher,
	reg *storage.Registry,
	format string,
) error {
	tools, err := fetcher.FetchAllTools(ctx)
	if err != nil {
		return fmt.Errorf("fetching tools: %w", err)
	}

	if format == formatJSON {
		return a.printJSON(tools)
	}

	// Build installed lookup: tool name -> version -> true.
	installedTools := map[string]map[string]bool{}
	for _, t := range reg.Tools {
		if installedTools[t.Name] == nil {
			installedTools[t.Name] = map[string]bool{}
		}
		installedTools[t.Name][t.Version] = true
	}

	fmt.Fprintln(a.streams.Out, "Available tools")
	fmt.Fprintln(a.streams.Out)

	for _, t := range tools {
		display := t.Display
		if display == "" {
			display = t.Name
		}
		fmt.Fprintf(a.streams.Out, "  %s  (%s)\n", display, t.Name)
		for _, v := range t.Versions {
			date := ""
			if !v.ReleaseDate.IsZero() {
				date = v.ReleaseDate.Format("2006-01-02")
			}
			marker := ""
			if installedTools[t.Name][v.Version] {
				marker = "  \u2713 installed"
			}
			fmt.Fprintf(a.streams.Out, "      %-20s %s%s\n", v.Version, date, marker)
		}
	}

	return nil
}

// --- Version detail view ---

func (a *app) runListQtVersion(ctx context.Context, fetcher *repository.MetadataFetcher, version, format string) error {
	idx, err := fetcher.FetchQtVersion(ctx, version)
	if err != nil {
		return fmt.Errorf("fetching metadata for Qt %s: %w", version, err)
	}

	var vi *repository.QtVersionInfo
	for i := range idx.QtVersions {
		if idx.QtVersions[i].Version == version {
			vi = &idx.QtVersions[i]
			break
		}
	}
	if vi == nil {
		return withHint(
			fmt.Errorf("Qt version %s not found in repository", version),
			"Run 'qvm list --all' to see available versions.",
		)
	}

	vi.SetDefaultArch(platform.Current().DefaultArch(version))

	if format == formatJSON {
		return a.printJSON(vi)
	}

	fmt.Fprintf(a.streams.Out, "Qt %s\n", version)

	fmt.Fprintf(a.streams.Out, "\nArchitectures\n")
	for _, ar := range vi.Archs {
		def := ""
		if ar.IsDefault {
			def = " (default)"
		}
		display := ar.DisplayName
		if display == ar.Name {
			display = ""
		}
		fmt.Fprintf(a.streams.Out, "  %-30s %s%s\n", ar.Name, display, def)
	}

	var addons []repository.Module
	for _, m := range vi.Modules {
		if !m.IsEssential {
			addons = append(addons, m)
		}
	}
	if len(addons) > 0 {
		fmt.Fprintf(a.streams.Out, "\nAdd-on modules\n")
		for _, m := range addons {
			fmt.Fprintf(a.streams.Out, "  %-20s %s\n", m.Name, m.DisplayName)
		}
	}

	// Supplementary items.
	var supp []string
	if vi.HasDocs {
		supp = append(supp, "docs")
	}
	if vi.HasExamples {
		supp = append(supp, "examples")
	}
	if vi.HasSources {
		supp = append(supp, "sources")
	}
	if vi.HasDebugInfo {
		supp = append(supp, "debug-symbols")
	}

	if len(supp) > 0 {
		fmt.Fprintf(a.streams.Out, "\nSupplementary (available for all modules above)\n")
		fmt.Fprintf(a.streams.Out, "  %s\n", strings.Join(supp, ", "))
	}

	return nil
}

func sortVersionsDesc(vers []repository.QtVersionInfo) {
	sort.Slice(vers, func(i, j int) bool {
		vi, erri := qtmeta.ParseVersion(vers[i].Version)
		vj, errj := qtmeta.ParseVersion(vers[j].Version)
		if erri != nil || errj != nil {
			return vers[i].Version > vers[j].Version
		}
		return vj.Less(vi)
	})
}

func (a *app) printVersionRow(
	v repository.QtVersionInfo,
	label string,
	installed map[string][]string,
	recommended string,
) {
	date := ""
	if !v.ReleaseDate.IsZero() {
		date = v.ReleaseDate.Format("2006-01-02")
	}

	suffix := ""
	if archs, ok := installed[v.Version]; ok {
		if len(archs) == 1 {
			suffix = fmt.Sprintf("  \u2713 installed (%s)", archs[0])
		} else {
			suffix = fmt.Sprintf("  \u2713 installed (%d archs)", len(archs))
		}
	} else if v.Version == recommended {
		suffix = "   \u2190 recommended"
	}

	fmt.Fprintf(a.streams.Out, "  %-10s %-16s %s%s\n", v.Version, label, date, suffix)
}

// newestLTSPatch returns the version string of the newest patch release within
// the highest LTS minor series present in vers (which must already be sorted
// descending). Returns "" if no LTS versions are found.
func newestLTSPatch(vers []repository.QtVersionInfo) string {
	// Find the highest LTS minor (e.g. 6.8 if both 6.5 and 6.8 are LTS).
	bestMinor := -1
	for _, v := range vers {
		if !v.IsLTS {
			continue
		}
		pv, err := qtmeta.ParseVersion(v.Version)
		if err != nil {
			continue
		}
		if pv.Minor() > bestMinor {
			bestMinor = pv.Minor()
		}
	}
	if bestMinor < 0 {
		return ""
	}
	// Return the first (highest patch) version with that minor.
	for _, v := range vers {
		if !v.IsLTS {
			continue
		}
		pv, err := qtmeta.ParseVersion(v.Version)
		if err != nil {
			continue
		}
		if pv.Minor() == bestMinor {
			return v.Version
		}
	}
	return ""
}

func (a *app) printJSON(v any) error {
	enc := json.NewEncoder(a.streams.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
