package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/sahilm/fuzzy"
	"github.com/trollixx/qvm/internal/config"
	"github.com/urfave/cli/v3"
)

// searchResult is the exported form of searchItem for JSON serialization.
type searchResult struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Display string `json:"display,omitempty"`
}

func newSearchCommand() *cli.Command {
	return &cli.Command{
		Name:      "search",
		Aliases:   []string{"s"},
		Usage:     "Search for Qt modules or tools",
		ArgsUsage: "<query>",
		Flags: []cli.Flag{
			formatFlag,
			hostFlag,
		},
		Action: runSearch,
	}
}

type searchItem struct {
	kind    string // "module" or "tool"
	name    string
	display string
}

func runSearch(ctx context.Context, cmd *cli.Command) error {
	query := cmd.Args().Get(0)
	if query == "" {
		return fmt.Errorf("missing argument\n\nUsage:\n  qvm search <query>       Search for Qt modules or tools\n\nExample:\n  qvm search charts")
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
		fmt.Fprintf(os.Stderr, "warning: could not fetch Qt version index: %v\n", err)
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

	// Fetch tools.
	tools, err := fetcher.FetchAllTools(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not fetch tools index: %v\n", err)
	} else {
		for _, t := range tools {
			item := searchItem{
				kind:    "tool",
				name:    t.Name,
				display: t.Display,
			}
			items = append(items, item)
			// Search by tool name.
			names = append(names, t.Name)
		}
	}

	if len(names) == 0 {
		fmt.Fprintln(os.Stdout, "No items available to search.")
		return nil
	}

	results := fuzzy.Find(query, names)
	if len(results) == 0 {
		fmt.Fprintf(os.Stdout, "No results found for %q.\n", query)
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

	if format == "json" {
		jsonResults := make([]searchResult, len(matched))
		for i, m := range matched {
			jsonResults[i] = searchResult{Kind: m.kind, Name: m.name, Display: m.display}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(jsonResults)
	}

	fmt.Fprintf(os.Stdout, "Search results for %q:\n\n", query)
	for _, item := range matched {
		display := item.display
		if display == "" {
			display = item.name
		}
		fmt.Fprintf(os.Stdout, "  [%-6s] %-24s %s\n", item.kind, item.name, display)
	}

	return nil
}
