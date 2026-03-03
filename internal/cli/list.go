package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/trollixx/qvm/internal/config"
	"github.com/trollixx/qvm/internal/platform"
	"github.com/trollixx/qvm/internal/repository"
	"github.com/trollixx/qvm/internal/storage"
	"github.com/trollixx/qvm/pkg/qtmeta"
	"github.com/urfave/cli/v3"
)

func newListCommand() *cli.Command {
	return &cli.Command{
		Name:      "list",
		Aliases:   []string{"ls"},
		Usage:     "List installed or available Qt versions and tools",
		ArgsUsage: "[qt | qt@<version> | tools]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "all",
				Usage: "show all available versions, not just installed",
			},
			formatFlag,
		},
		Action: runList,
	}
}

func runList(ctx context.Context, cmd *cli.Command) error {
	arg := cmd.Args().Get(0)
	showAll := cmd.Bool("all")
	format := cmd.String("format")

	if err := validateFormat(format); err != nil {
		return err
	}

	// qt@<version> detail view — always fetches remote metadata.
	if strings.HasPrefix(arg, "qt@") {
		version := strings.TrimPrefix(arg, "qt@")
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		fetcher, err := buildFetcher(cfg)
		if err != nil {
			return fmt.Errorf("initializing fetcher: %w", err)
		}
		return runListQtVersion(ctx, fetcher, version, format)
	}

	if arg != "" && arg != "qt" && arg != "tools" {
		return fmt.Errorf("unknown list target %q\n\nUsage:\n  qvm list                 Show installed versions\n  qvm list --all           Show all available versions\n  qvm list qt@<version>    Show version details (archs, modules)", arg)
	}

	if showAll {
		return runListAll(ctx, arg, format)
	}
	return runListInstalled(arg, format)
}

// --- Installed-only views (default) ---

func runListInstalled(arg, format string) error {
	registry, err := storage.NewRegistryManager()
	if err != nil {
		return fmt.Errorf("opening registry: %w", err)
	}
	reg, err := registry.Load()
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	if format == "json" {
		switch arg {
		case "qt":
			return printJSON(reg.Qt)
		case "tools":
			return printJSON(reg.Tools)
		default:
			return printJSON(reg)
		}
	}

	switch arg {
	case "qt":
		printInstalledQt(reg)
	case "tools":
		printInstalledTools(reg)
	default:
		printInstalledQt(reg)
		fmt.Fprintln(os.Stdout)
		printInstalledTools(reg)
	}
	return nil
}

func printInstalledQt(reg *storage.Registry) {
	fmt.Fprintln(os.Stdout, "Installed Qt versions")

	if len(reg.Qt) == 0 {
		fmt.Fprintln(os.Stdout, "  (none)")
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, "  Install one with: qvm install qt@<version>")
		fmt.Fprintln(os.Stdout, "  Run 'qvm list --all' to see available versions.")
		return
	}

	for _, q := range reg.Qt {
		size := ""
		if q.SizeBytes > 0 {
			size = formatSize(q.SizeBytes)
		}

		fmt.Fprintf(os.Stdout, "  %-8s %-30s %-40s %s\n",
			q.Version, q.Arch, q.InstallDir, size)

		if len(q.Modules) > 0 {
			fmt.Fprintf(os.Stdout, "          modules: %s\n", strings.Join(q.Modules, ", "))
		}

		extras := buildExtrasLine(q.Extras)
		if extras != "" {
			fmt.Fprintf(os.Stdout, "          extras:  %s\n", extras)
		}
	}
}

func printInstalledTools(reg *storage.Registry) {
	fmt.Fprintln(os.Stdout, "Installed tools")

	if len(reg.Tools) == 0 {
		fmt.Fprintln(os.Stdout, "  (none)")
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, "  Run 'qvm list tools --all' to see available tools.")
		return
	}

	for _, t := range reg.Tools {
		size := ""
		if t.SizeBytes > 0 {
			size = formatSize(t.SizeBytes)
		}
		fmt.Fprintf(os.Stdout, "  %-16s %-12s %-40s %s\n",
			t.Name, t.Version, t.InstallDir, size)
	}
}

func buildExtrasLine(extras storage.InstalledExtras) string {
	var parts []string
	if len(extras.Docs) > 0 {
		if len(extras.Docs) == 1 && extras.Docs[0] == "*" {
			parts = append(parts, "docs (all)")
		} else {
			parts = append(parts, fmt.Sprintf("docs (%s)", strings.Join(extras.Docs, ", ")))
		}
	}
	if len(extras.Examples) > 0 {
		if len(extras.Examples) == 1 && extras.Examples[0] == "*" {
			parts = append(parts, "examples (all)")
		} else {
			parts = append(parts, fmt.Sprintf("examples (%s)", strings.Join(extras.Examples, ", ")))
		}
	}
	if extras.Sources {
		parts = append(parts, "sources")
	}
	if extras.DebugInfo {
		parts = append(parts, "debug-info")
	}
	return strings.Join(parts, ", ")
}

// --- All-available views (with installed markers) ---

func runListAll(ctx context.Context, arg, format string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	fetcher, err := buildFetcher(cfg)
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
		return runListAllQt(ctx, fetcher, reg, format)
	case "tools":
		return runListAllTools(ctx, fetcher, reg, format)
	default:
		if err := runListAllQt(ctx, fetcher, reg, format); err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout)
		return runListAllTools(ctx, fetcher, reg, format)
	}
}

func runListAllQt(ctx context.Context, fetcher *repository.MetadataFetcher, reg *storage.Registry, format string) error {
	versions, err := fetcher.FetchAllQtVersions(ctx)
	if err != nil {
		return fmt.Errorf("fetching Qt versions: %w", err)
	}

	if format == "json" {
		return printJSON(versions)
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

	fmt.Fprintln(os.Stdout, "Qt versions")

	for _, major := range majors {
		vers := byMajor[major]
		// Sort by semantic version descending.
		sort.Slice(vers, func(i, j int) bool {
			vi, erri := qtmeta.ParseVersion(vers[i].Version)
			vj, errj := qtmeta.ParseVersion(vers[j].Version)
			if erri != nil || errj != nil {
				return vers[i].Version > vers[j].Version
			}
			return vj.Less(vi) // descending
		})

		fmt.Fprintf(os.Stdout, "\nQt %d\n", major)

		recommendedVersion := newestLTSPatch(vers)

		for _, v := range vers {
			label := ""
			if v.IsPreview {
				label = "Preview"
			} else if v.IsLTS {
				label = "LTS"
			} else if v.Version == vers[0].Version {
				label = "Latest"
			}

			date := ""
			if !v.ReleaseDate.IsZero() {
				date = v.ReleaseDate.Format("2006-01-02")
			}

			suffix := ""
			if archs, ok := installedVersions[v.Version]; ok {
				if len(archs) == 1 {
					suffix = fmt.Sprintf("  \u2713 installed (%s)", archs[0])
				} else {
					suffix = fmt.Sprintf("  \u2713 installed (%d archs)", len(archs))
				}
			} else if v.Version == recommendedVersion {
				suffix = "   \u2190 recommended"
			}

			fmt.Fprintf(os.Stdout, "  %-10s %-16s %s%s\n", v.Version, label, date, suffix)
		}
	}

	fmt.Fprintln(os.Stdout, "\nRun 'qvm list qt@<version>' to see available targets and modules.")
	return nil
}

func runListAllTools(ctx context.Context, fetcher *repository.MetadataFetcher, reg *storage.Registry, format string) error {
	tools, err := fetcher.FetchAllTools(ctx)
	if err != nil {
		return fmt.Errorf("fetching tools: %w", err)
	}

	if format == "json" {
		return printJSON(tools)
	}

	// Build installed lookup: tool name -> version -> true.
	installedTools := map[string]map[string]bool{}
	for _, t := range reg.Tools {
		if installedTools[t.Name] == nil {
			installedTools[t.Name] = map[string]bool{}
		}
		installedTools[t.Name][t.Version] = true
	}

	fmt.Fprintln(os.Stdout, "Available tools")
	fmt.Fprintln(os.Stdout)

	for _, t := range tools {
		display := t.Display
		if display == "" {
			display = t.Name
		}
		fmt.Fprintf(os.Stdout, "  %s  (%s)\n", display, t.Name)
		for _, v := range t.Versions {
			date := ""
			if !v.ReleaseDate.IsZero() {
				date = v.ReleaseDate.Format("2006-01-02")
			}
			marker := ""
			if installedTools[t.Name][v.Version] {
				marker = "  \u2713 installed"
			}
			fmt.Fprintf(os.Stdout, "      %-20s %s%s\n", v.Version, date, marker)
		}
	}

	return nil
}

// --- Version detail view ---

func runListQtVersion(ctx context.Context, fetcher *repository.MetadataFetcher, version string, format string) error {
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
		return fmt.Errorf("Qt version %s not found in repository\n\nRun 'qvm list --all' to see available versions.", version)
	}

	vi.SetDefaultArch(platform.Current().DefaultArch(version))

	if format == "json" {
		return printJSON(vi)
	}

	fmt.Fprintf(os.Stdout, "Qt %s\n", version)

	fmt.Fprintf(os.Stdout, "\nArchitectures\n")
	for _, a := range vi.Archs {
		def := ""
		if a.IsDefault {
			def = " (default)"
		}
		display := a.DisplayName
		if display == a.Name {
			display = ""
		}
		fmt.Fprintf(os.Stdout, "  %-30s %s%s\n", a.Name, display, def)
	}

	var addons []repository.Module
	for _, m := range vi.Modules {
		if !m.IsEssential {
			addons = append(addons, m)
		}
	}
	if len(addons) > 0 {
		fmt.Fprintf(os.Stdout, "\nAdd-on modules\n")
		for _, m := range addons {
			fmt.Fprintf(os.Stdout, "  %-20s %s\n", m.Name, m.DisplayName)
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
		supp = append(supp, "debug-info")
	}

	if len(supp) > 0 {
		fmt.Fprintf(os.Stdout, "\nSupplementary (available for all modules above)\n")
		fmt.Fprintf(os.Stdout, "  %s\n", strings.Join(supp, ", "))
	}

	return nil
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
		if pv.Minor > bestMinor {
			bestMinor = pv.Minor
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
		if pv.Minor == bestMinor {
			return v.Version
		}
	}
	return ""
}

func printJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
