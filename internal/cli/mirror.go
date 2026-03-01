package cli

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/trollixx/qvm/internal/config"
	"github.com/trollixx/qvm/internal/repository"
	"github.com/urfave/cli/v3"
)

const probeTimeoutSeconds = 8

func newMirrorCommand() *cli.Command {
	return &cli.Command{
		Name:   "mirror",
		Usage:  "Probe and manage Qt repository mirrors",
		Action: runMirrorList,
		Commands: []*cli.Command{
			{
				Name:  "list",
				Usage: "Probe cached mirrors and display latency",
				Action: runMirrorList,
			},
			{
				Name:  "refresh",
				Usage: "Fetch the latest mirror list from Qt and update the local cache",
				Action: runMirrorRefresh,
			},
			{
				Name:      "select",
				Usage:     "Set the primary mirror URL, or auto-select the fastest",
				ArgsUsage: "[<url>]",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "auto",
						Usage: "Probe all cached mirrors and select the fastest",
					},
				},
				Action: runMirrorSelect,
			},
		},
	}
}

func runMirrorRefresh(ctx context.Context, _ *cli.Command) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Fetching mirror list from %s...\n", repository.MirrorListURL)
	mirrors, err := repository.FetchMirrorList(ctx, cfg.Download.TimeoutSeconds)
	if err != nil {
		return fmt.Errorf("fetching mirror list: %w", err)
	}

	mlc, err := repository.NewMirrorListCache()
	if err != nil {
		return fmt.Errorf("opening mirror list cache: %w", err)
	}
	if err := mlc.Save(mirrors); err != nil {
		return fmt.Errorf("saving mirror list: %w", err)
	}

	fmt.Fprintf(os.Stdout, "%s  Cached %d mirrors to %s\n", checkOK, len(mirrors), mlc.Path())
	return nil
}

func runMirrorList(ctx context.Context, _ *cli.Command) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	mlc, err := repository.NewMirrorListCache()
	if err != nil {
		return fmt.Errorf("opening mirror list cache: %w", err)
	}

	cached, err := mlc.Load()
	if err != nil {
		return fmt.Errorf("loading mirror list cache: %w", err)
	}
	if len(cached) == 0 {
		fmt.Fprintf(os.Stdout, "No mirror list cached.\n")
		fmt.Fprintf(os.Stdout, "Run 'qvm mirror refresh' to fetch the list from %s\n\n", repository.MirrorListURL)
		fmt.Fprintf(os.Stdout, "Current primary: %s\n", cfg.Repository.URL)
		return nil
	}

	filtered := filterBlacklist(cached, cfg.Repository.Blacklist)
	urls := dedupURLs(cfg.Repository.URL, filtered)

	fmt.Fprintf(os.Stdout, "Probing %d mirrors...\n\n", len(urls))
	results := repository.ProbeURLs(ctx, urls, probeTimeoutSeconds)

	primary := cfg.Repository.URL
	fastestURL := ""
	for _, r := range results {
		if r.Reachable {
			fastestURL = r.URL
			break
		}
	}

	for _, r := range results {
		marker := "  "
		if r.URL == primary {
			marker = "* "
		}
		suffix := ""
		if r.URL == fastestURL && r.URL != primary {
			suffix = "  \u2190 fastest"
		}
		latency := "timeout"
		if r.Reachable {
			latency = fmt.Sprintf("%d ms", r.Latency.Milliseconds())
		}
		fmt.Fprintf(os.Stdout, "  %s%-52s %s%s\n", marker, mirrorDisplayName(r.URL), latency, suffix)
	}

	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "* current primary")
	if fastestURL != "" && fastestURL != primary {
		fmt.Fprintln(os.Stdout, "Run 'qvm mirror select --auto' to switch to the fastest mirror.")
	}
	return nil
}

func runMirrorSelect(ctx context.Context, cmd *cli.Command) error {
	auto := cmd.Bool("auto")
	urlArg := cmd.Args().Get(0)

	if !auto && urlArg == "" {
		return fmt.Errorf("usage: qvm mirror select --auto | qvm mirror select <url>")
	}
	if auto && urlArg != "" {
		return fmt.Errorf("cannot combine --auto with a URL argument")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if auto {
		return mirrorSelectAuto(ctx, cfg)
	}
	return mirrorSelectURL(ctx, cfg, urlArg)
}

func mirrorSelectAuto(ctx context.Context, cfg *config.Config) error {
	mlc, err := repository.NewMirrorListCache()
	if err != nil {
		return fmt.Errorf("opening mirror list cache: %w", err)
	}
	cached, err := mlc.Load()
	if err != nil {
		return fmt.Errorf("loading mirror list: %w", err)
	}
	if len(cached) == 0 {
		return fmt.Errorf("no mirror list cached; run 'qvm mirror refresh' first")
	}

	filtered := filterBlacklist(cached, cfg.Repository.Blacklist)
	urls := dedupURLs(cfg.Repository.URL, filtered)

	fmt.Fprintf(os.Stdout, "Probing %d mirrors...\n\n", len(urls))
	results := repository.ProbeURLs(ctx, urls, probeTimeoutSeconds)

	for _, r := range results {
		if r.Reachable {
			fmt.Fprintf(os.Stdout, "  %-52s %d ms\n", mirrorDisplayName(r.URL), r.Latency.Milliseconds())
		} else {
			fmt.Fprintf(os.Stdout, "  %-52s timeout\n", mirrorDisplayName(r.URL))
		}
	}
	fmt.Fprintln(os.Stdout)

	for _, r := range results {
		if !r.Reachable {
			continue
		}
		if r.URL == cfg.Repository.URL {
			fmt.Fprintf(os.Stdout, "%s  Already using the fastest mirror: %s\n", checkOK, mirrorDisplayName(r.URL))
			return nil
		}
		cfg.Repository.URL = r.URL
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		fmt.Fprintf(os.Stdout, "%s  Primary mirror set to: %s\n", checkOK, r.URL)
		return nil
	}
	return fmt.Errorf("no reachable mirrors found")
}

func mirrorSelectURL(ctx context.Context, cfg *config.Config, rawURL string) error {
	if !strings.HasSuffix(rawURL, "/") {
		rawURL += "/"
	}
	if _, err := url.ParseRequestURI(rawURL); err != nil {
		return fmt.Errorf("invalid URL %q: %w", rawURL, err)
	}
	if isBlacklisted(rawURL, cfg.Repository.Blacklist) {
		return fmt.Errorf("mirror %s is in the blacklist; remove it with: qvm config set repository.blacklist ...", mirrorDisplayName(rawURL))
	}

	fmt.Fprintf(os.Stdout, "Probing %s...\n", mirrorDisplayName(rawURL))
	results := repository.ProbeURLs(ctx, []string{rawURL}, probeTimeoutSeconds)
	if len(results) == 0 || !results[0].Reachable {
		return fmt.Errorf("mirror %s is not reachable (timeout or HTTP error)", mirrorDisplayName(rawURL))
	}
	fmt.Fprintf(os.Stdout, "Latency: %d ms\n\n", results[0].Latency.Milliseconds())

	cfg.Repository.URL = rawURL
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Fprintf(os.Stdout, "%s  Primary mirror set to: %s\n", checkOK, rawURL)
	return nil
}

// filterBlacklist returns mirrors with any blacklisted URLs removed.
func filterBlacklist(mirrors []string, blacklist []string) []string {
	if len(blacklist) == 0 {
		return mirrors
	}
	bl := make(map[string]bool, len(blacklist))
	for _, b := range blacklist {
		bl[normalizeURL(b)] = true
	}
	out := make([]string, 0, len(mirrors))
	for _, m := range mirrors {
		if !bl[normalizeURL(m)] {
			out = append(out, m)
		}
	}
	return out
}

// isBlacklisted reports whether url is in the blacklist (normalized comparison).
func isBlacklisted(rawURL string, blacklist []string) bool {
	norm := normalizeURL(rawURL)
	for _, b := range blacklist {
		if normalizeURL(b) == norm {
			return true
		}
	}
	return false
}

// normalizeURL ensures url ends with "/".
func normalizeURL(u string) string {
	if !strings.HasSuffix(u, "/") {
		return u + "/"
	}
	return u
}

// dedupURLs returns primary followed by fallbacks with duplicates removed.
func dedupURLs(primary string, fallbacks []string) []string {
	seen := map[string]bool{primary: true}
	out := []string{primary}
	for _, u := range fallbacks {
		if !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	return out
}

// mirrorDisplayName strips the URL scheme and trailing slash for compact display.
func mirrorDisplayName(base string) string {
	s := strings.TrimPrefix(base, "https://")
	s = strings.TrimPrefix(s, "http://")
	return strings.TrimSuffix(s, "/")
}
