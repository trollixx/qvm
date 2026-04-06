package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/urfave/cli/v3"

	"github.com/trollixx/qvm/internal/config"
	"github.com/trollixx/qvm/internal/install"
	"github.com/trollixx/qvm/internal/repository"
	"github.com/trollixx/qvm/internal/storage"
)

// Version is the current qvm version string.
// Override at build time: go build -ldflags "-X github.com/trollixx/qvm/internal/cli.Version=1.2.3".
var Version = "dev" //nolint:gochecknoglobals // set by build ldflags

// app holds shared dependencies for all CLI commands.
type app struct {
	streams *IOStreams
}

// NewApp creates and configures the qvm CLI application.
func NewApp() *cli.Command {
	return newRootCommand(NewIOStreams())
}

func newRootCommand(streams *IOStreams) *cli.Command {
	a := &app{streams: streams}
	return &cli.Command{
		Name:                  "qvm",
		Usage:                 "Qt Version Manager",
		Version:               Version,
		EnableShellCompletion: true,
		Writer:                a.streams.Out,
		ErrWriter:             a.streams.ErrOut,
		ExitErrHandler: func(_ context.Context, _ *cli.Command, err error) {
			if err == nil || err.Error() == "" {
				return
			}
			fmt.Fprintln(a.streams.ErrOut, err.Error())
			var he *hintError
			if errors.As(err, &he) {
				fmt.Fprintln(a.streams.ErrOut)
				fmt.Fprintln(a.streams.ErrOut, he.hint)
			}
		},
		Action: a.runDefault,
		Commands: []*cli.Command{
			a.newInstallCommand(),
			a.newListCommand(),
			a.newUninstallCommand(),
			a.newPathCommand(),
			a.newInfoCommand(),
			a.newSearchCommand(),
			a.newDoctorCommand(),
			a.newConfigCommand(),
			a.newMirrorCommand(),
			a.newCacheCommand(),
		},
	}
}

// runDefault handles bare "qvm" with no arguments by printing a quick-start guide.
func (a *app) runDefault(_ context.Context, _ *cli.Command) error {
	fmt.Fprintf(a.streams.Out, "qvm - Qt Version Manager (v%s)\n", Version)
	fmt.Fprintln(a.streams.Out)
	fmt.Fprintln(a.streams.Out, "Quick start:")
	fmt.Fprintln(a.streams.Out, "  qvm list --all           List available Qt versions")
	fmt.Fprintln(a.streams.Out, "  qvm install 6.8.3        Install a Qt version")
	fmt.Fprintln(a.streams.Out, "  qvm path 6.8.3           Print install directory")
	fmt.Fprintln(a.streams.Out, "  qvm list                 Show what's installed")
	fmt.Fprintln(a.streams.Out, "  qvm doctor               Check environment health")
	fmt.Fprintln(a.streams.Out)
	fmt.Fprintln(a.streams.Out, "Run 'qvm --help' for all commands and options.")
	return nil
}

// resolveHost returns host if non-empty, otherwise auto-detects.
func resolveHost(host string) string {
	if host != "" {
		return host
	}
	return repository.PlatformHost()
}

// buildFetcher constructs a MetadataFetcher from config.
func buildFetcher(cfg *config.Config, host string) (*repository.MetadataFetcher, error) {
	cache, err := repository.NewCache()
	if err != nil {
		return nil, err
	}
	mirrors := repository.NewMirrorList(cfg.Repository.URL, cfg.Repository.Mirrors, resolveHost(host))
	client := repository.NewClient(cfg.Download.TimeoutSeconds)
	return repository.NewMetadataFetcher(client, cache, mirrors), nil
}

// buildDeps constructs the full dependency chain from config.
func buildDeps(
	cfg *config.Config,
	host string,
) (*install.Installer, error) {
	cache, err := repository.NewCache()
	if err != nil {
		return nil, err
	}

	mirrors := repository.NewMirrorList(cfg.Repository.URL, cfg.Repository.Mirrors, resolveHost(host))
	client := repository.NewClient(cfg.Download.TimeoutSeconds)
	fetcher := repository.NewMetadataFetcher(client, cache, mirrors)
	resolver := repository.NewResolver(fetcher)

	registry, err := storage.NewRegistryManager()
	if err != nil {
		return nil, err
	}

	installer := install.NewInstaller(resolver, registry)
	return installer, nil
}

// formatSize converts bytes to a human-readable string.
func formatSize(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f", float64(bytes)/float64(gb)) + " GB"
	case bytes >= mb:
		return fmt.Sprintf("%.1f", float64(bytes)/float64(mb)) + " MB"
	case bytes >= kb:
		return fmt.Sprintf("%.1f", float64(bytes)/float64(kb)) + " KB"
	default:
		return strconv.Itoa(int(bytes)) + " B"
	}
}
