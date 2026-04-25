package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sahilm/fuzzy"
	"github.com/urfave/cli/v3"

	"github.com/trollixx/qvm/internal/config"
	"github.com/trollixx/qvm/internal/repository"
	"github.com/trollixx/qvm/pkg/qtmeta"
)

// searchResult is the exported form of searchItem for JSON serialization.
type searchResult struct {
	Name    string `json:"name"`
	Display string `json:"display,omitempty"`
}

func (a *app) newSearchCommand() *cli.Command {
	return &cli.Command{
		Name:      "search",
		Aliases:   []string{"s"},
		Usage:     "Search for Qt modules",
		ArgsUsage: "<query>",
		Flags: []cli.Flag{
			newFormatFlag(),
			newHostFlag(),
		},
		Action: a.runSearch,
	}
}

type searchItem struct {
	name    string
	display string
}

func (a *app) runSearch(ctx context.Context, cmd *cli.Command) error {
	query := cmd.Args().Get(0)
	if query == "" {
		return errors.New("missing argument\n\n" +
			"Usage:\n" +
			"  qvm search <query>       Search for Qt modules\n\n" +
			"Example:\n" +
			"  qvm search charts")
	}

	format := cmd.String("format")
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	fetcher, err := buildFetcher(cfg, cmd.String("host"), "")
	if err != nil {
		return fmt.Errorf("initializing fetcher: %w", err)
	}

	items, err := collectSearchItems(ctx, fetcher, a.streams.ErrOut)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Fprintln(a.streams.Out, "No items available to search.")
		return nil
	}

	names := make([]string, len(items))
	for i, it := range items {
		names[i] = it.name
	}

	results := fuzzy.Find(query, names)
	if len(results) == 0 {
		fmt.Fprintf(a.streams.Out, "No results found for %q.\n", query)
		return nil
	}

	seen := map[string]bool{}
	var matched []searchItem
	for _, r := range results {
		if r.Index >= len(items) {
			continue
		}
		item := items[r.Index]
		if seen[item.name] {
			continue
		}
		seen[item.name] = true
		matched = append(matched, item)
	}

	if format == formatJSON {
		jsonResults := make([]searchResult, len(matched))
		for i, m := range matched {
			jsonResults[i] = searchResult{Name: m.name, Display: m.display}
		}
		enc := json.NewEncoder(a.streams.Out)
		enc.SetIndent("", "  ")
		return enc.Encode(jsonResults)
	}

	fmt.Fprintf(a.streams.Out, "Search results for %q:\n\n", query)
	for _, item := range matched {
		display := item.display
		if display == "" {
			display = item.name
		}
		fmt.Fprintf(a.streams.Out, "  %-24s %s\n", item.name, display)
	}

	return nil
}

// collectSearchItems fetches add-on module names from the latest released Qt version.
// FetchAllQtVersions only returns version stubs, so we drill into the newest released
// version's metadata to enumerate modules. Preview versions are skipped.
func collectSearchItems(
	ctx context.Context,
	fetcher *repository.MetadataFetcher,
	errOut interface {
		Write([]byte) (int, error)
	},
) ([]searchItem, error) {
	versions, err := fetcher.FetchAllQtVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching Qt version index: %w", err)
	}

	// Find the highest released (non-preview) version.
	var newest *repository.QtVersionInfo
	var newestParsed qtmeta.Version
	for i := range versions {
		v := &versions[i]
		if v.IsPreview {
			continue
		}
		parsed, perr := qtmeta.ParseVersion(v.Version)
		if perr != nil {
			continue
		}
		if newest == nil || parsed.GTE(newestParsed) {
			newest = v
			newestParsed = parsed
		}
	}
	if newest == nil {
		fmt.Fprintln(errOut, "warning: no released Qt versions found")
		return nil, nil
	}

	idx, err := fetcher.FetchQtVersion(ctx, newest.Version)
	if err != nil {
		return nil, fmt.Errorf("fetching metadata for Qt %s: %w", newest.Version, err)
	}

	// Find the matching version inside the index.
	var vi *repository.QtVersionInfo
	for i := range idx.QtVersions {
		if idx.QtVersions[i].Version == newest.Version {
			vi = &idx.QtVersions[i]
			break
		}
	}
	if vi == nil {
		return nil, nil
	}

	items := make([]searchItem, 0, len(vi.Modules))
	for _, m := range vi.Modules {
		items = append(items, searchItem{name: m.Name, display: m.DisplayName})
	}
	return items, nil
}
