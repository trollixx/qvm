package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/trollixx/qvm/internal/config"
	"github.com/trollixx/qvm/internal/repository"
)

const probeTimeoutSeconds = 8

func (a *app) newMirrorCommand() *cli.Command {
	return &cli.Command{
		Name:   "mirror",
		Usage:  "Probe and manage Qt repository mirrors",
		Action: a.runMirrorList,
		Commands: []*cli.Command{
			{
				Name:   "list",
				Usage:  "Probe cached mirrors and display latency",
				Action: a.runMirrorList,
			},
			{
				Name:   "refresh",
				Usage:  "Fetch the latest mirror list from Qt and update the local cache",
				Action: a.runMirrorRefresh,
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
				Action: a.runMirrorSelect,
			},
		},
	}
}

func (a *app) runMirrorRefresh(ctx context.Context, _ *cli.Command) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	fmt.Fprintf(a.streams.Out, "Fetching mirror list from %s...\n", repository.MirrorListURL)
	mirrors, err := repository.FetchMirrorList(ctx, cfg.Download.TimeoutSeconds)
	if err != nil {
		return fmt.Errorf("fetching mirror list: %w", err)
	}

	mlc, err := repository.NewMirrorListCache()
	if err != nil {
		return fmt.Errorf("opening mirror list cache: %w", err)
	}
	err = mlc.Save(mirrors)
	if err != nil {
		return fmt.Errorf("saving mirror list: %w", err)
	}

	fmt.Fprintf(a.streams.Out, "%s  Cached %d mirrors to %s\n", checkOK, len(mirrors), mlc.Path())
	return nil
}

func (a *app) runMirrorList(ctx context.Context, _ *cli.Command) error {
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
		fmt.Fprintf(a.streams.Out, "No mirror list cached.\n")
		fmt.Fprintf(a.streams.Out, "Run 'qvm mirror refresh' to fetch the list from %s\n\n", repository.MirrorListURL)
		fmt.Fprintf(a.streams.Out, "Current primary: %s\n", cfg.Repository.URL)
		return nil
	}

	filtered := filterBlacklist(cached, cfg.Repository.Blacklist)
	urls := dedupURLs(cfg.Repository.URL, filtered)

	fmt.Fprintf(a.streams.Out, "Probing %d mirrors...\n\n", len(urls))
	results := repository.ProbeURLs(ctx, urls, probeTimeoutSeconds, repository.PlatformHost())

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
		fmt.Fprintf(a.streams.Out, "  %s%-52s %s%s\n", marker, mirrorDisplayName(r.URL), latency, suffix)
	}

	fmt.Fprintln(a.streams.Out)
	fmt.Fprintln(a.streams.Out, "* current primary")
	if fastestURL != "" && fastestURL != primary {
		fmt.Fprintln(a.streams.Out, "Run 'qvm mirror select --auto' to switch to the fastest mirror.")
	}
	return nil
}

func (a *app) runMirrorSelect(ctx context.Context, cmd *cli.Command) error {
	auto := cmd.Bool("auto")
	urlArg := cmd.Args().Get(0)

	if !auto && urlArg == "" {
		return newHintError("specify --auto or a URL", "Usage: qvm mirror select --auto | qvm mirror select <url>")
	}
	if auto && urlArg != "" {
		return errors.New("cannot combine --auto with a URL argument")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if auto {
		return a.mirrorSelectAuto(ctx, cfg)
	}
	return a.mirrorSelectURL(ctx, cfg, urlArg)
}

func (a *app) mirrorSelectAuto(ctx context.Context, cfg *config.Config) error {
	mlc, err := repository.NewMirrorListCache()
	if err != nil {
		return fmt.Errorf("opening mirror list cache: %w", err)
	}
	cached, err := mlc.Load()
	if err != nil {
		return fmt.Errorf("loading mirror list: %w", err)
	}
	if len(cached) == 0 {
		return errors.New("no mirror list cached; run 'qvm mirror refresh' first")
	}

	filtered := filterBlacklist(cached, cfg.Repository.Blacklist)
	urls := dedupURLs(cfg.Repository.URL, filtered)

	fmt.Fprintf(a.streams.Out, "Probing %d mirrors...\n\n", len(urls))
	results := repository.ProbeURLs(ctx, urls, probeTimeoutSeconds, repository.PlatformHost())

	for _, r := range results {
		if r.Reachable {
			fmt.Fprintf(a.streams.Out, "  %-52s %d ms\n", mirrorDisplayName(r.URL), r.Latency.Milliseconds())
		} else {
			fmt.Fprintf(a.streams.Out, "  %-52s timeout\n", mirrorDisplayName(r.URL))
		}
	}
	fmt.Fprintln(a.streams.Out)

	for _, r := range results {
		if !r.Reachable {
			continue
		}
		if r.URL == cfg.Repository.URL {
			fmt.Fprintf(a.streams.Out, "%s  Already using the fastest mirror: %s\n", checkOK, mirrorDisplayName(r.URL))
			return nil
		}
		cfg.Repository.URL = r.URL
		err = config.Save(cfg)
		if err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		fmt.Fprintf(a.streams.Out, "%s  Primary mirror set to: %s\n", checkOK, r.URL)
		return nil
	}
	return errors.New("no reachable mirrors found")
}

func (a *app) mirrorSelectURL(ctx context.Context, cfg *config.Config, rawURL string) error {
	if !strings.HasSuffix(rawURL, "/") {
		rawURL += "/"
	}
	_, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", rawURL, err)
	}
	if isBlacklisted(rawURL, cfg.Repository.Blacklist) {
		return fmt.Errorf(
			"mirror %s is in the blacklist; remove it with 'qvm config set repository.blacklist <value>'",
			mirrorDisplayName(rawURL),
		)
	}

	fmt.Fprintf(a.streams.Out, "Probing %s...\n", mirrorDisplayName(rawURL))
	results := repository.ProbeURLs(ctx, []string{rawURL}, probeTimeoutSeconds, repository.PlatformHost())
	if len(results) == 0 || !results[0].Reachable {
		return fmt.Errorf("mirror %s is not reachable (timeout or HTTP error)", mirrorDisplayName(rawURL))
	}
	fmt.Fprintf(a.streams.Out, "Latency: %d ms\n\n", results[0].Latency.Milliseconds())

	cfg.Repository.URL = rawURL
	err = config.Save(cfg)
	if err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Fprintf(a.streams.Out, "%s  Primary mirror set to: %s\n", checkOK, rawURL)
	return nil
}

// filterBlacklist returns mirrors with any blacklisted URLs removed.
func filterBlacklist(mirrors, blacklist []string) []string {
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
