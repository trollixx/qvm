package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sahilm/fuzzy"
	"github.com/urfave/cli/v3"

	"github.com/trollixx/qvm/internal/config"
)

// searchResult is the exported form of searchItem for JSON serialization.
type searchResult struct {
	Kind    string `json:"kind"`
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
	kind    string // "module" or "tool"
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

	fetcher, err := buildFetcher(cfg, cmd.String("host"))
	if err != nil {
		return fmt.Errorf("initializing fetcher: %w", err)
	}

	var items []searchItem
	var names []string

	// Fetch Qt versions for modules.
	versions, err := fetcher.FetchAllQtVersions(ctx)
	if err != nil {
		fmt.Fprintf(a.streams.ErrOut, "warning: could not fetch Qt version index: %v\n", err)
	} else {
		// Collect unique module names across all versions.
		seenModules := map[string]bool{}
		for _, v := range versions {
			for _, m := range v.Modules {
				if seenModules[m.Name] {
					continue
				}
				seenModules[m.Name] = true
				item := searchItem{
					kind:    "module",
					name:    m.Name,
					display: m.DisplayName,
				}
				items = append(items, item)
				names = append(names, m.Name)
			}
		}
	}

	if len(names) == 0 {
		fmt.Fprintln(a.streams.Out, "No items available to search.")
		return nil
	}

	results := fuzzy.Find(query, names)
	if len(results) == 0 {
		fmt.Fprintf(a.streams.Out, "No results found for %q.\n", query)
		return nil
	}

	// Deduplicate results.
	seen := map[string]bool{}
	var matched []searchItem
	for _, r := range results {
		if r.Index >= len(items) {
			continue
		}
		item := items[r.Index]
		key := item.kind + ":" + item.name
		if seen[key] {
			continue
		}
		seen[key] = true
		matched = append(matched, item)
	}

	if format == formatJSON {
		jsonResults := make([]searchResult, len(matched))
		for i, m := range matched {
			jsonResults[i] = searchResult{Kind: m.kind, Name: m.name, Display: m.display}
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
		fmt.Fprintf(a.streams.Out, "  [%-6s] %-24s %s\n", item.kind, item.name, display)
	}

	return nil
}
