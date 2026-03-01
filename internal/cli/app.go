package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/trollixx/qvm/internal/config"
	"github.com/trollixx/qvm/internal/install"
	"github.com/trollixx/qvm/internal/repository"
	"github.com/trollixx/qvm/internal/storage"
	"github.com/urfave/cli/v3"
)

// Version is the current qvm version string.
// Override at build time: go build -ldflags "-X github.com/trollixx/qvm/internal/cli.Version=1.2.3"
var Version = "dev"

// NewApp creates and configures the qvm CLI application.
func NewApp() *cli.Command {
	app := &cli.Command{
		Name:                  "qvm",
		Usage:                 "Qt Version Manager",
		Version:               Version,
		EnableShellCompletion: true,
		Action:                runDefault,
		Commands: []*cli.Command{
			newInstallCommand(),
			newListCommand(),
			newUninstallCommand(),
			newPathCommand(),
			newInfoCommand(),
			newSearchCommand(),
			newDoctorCommand(),
			newConfigCommand(),
			newMirrorCommand(),
			newCacheCommand(),
		},
	}
	return app
}

// runDefault handles bare "qvm" with no arguments by printing a quick-start guide.
func runDefault(_ context.Context, _ *cli.Command) error {
	fmt.Fprintf(os.Stdout, "qvm - Qt Version Manager (v%s)\n", Version)
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "Quick start:")
	fmt.Fprintln(os.Stdout, "  qvm list --all           List available Qt versions")
	fmt.Fprintln(os.Stdout, "  qvm install qt@6.8.3     Install a Qt version")
	fmt.Fprintln(os.Stdout, "  qvm path qt@6.8.3        Print install directory")
	fmt.Fprintln(os.Stdout, "  qvm list                 Show what's installed")
	fmt.Fprintln(os.Stdout, "  qvm doctor               Check environment health")
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "Run 'qvm --help' for all commands and options.")
	return nil
}

// buildFetcher constructs a MetadataFetcher from config.
func buildFetcher(cfg *config.Config) (*repository.MetadataFetcher, error) {
	cache, err := repository.NewCache()
	if err != nil {
		return nil, err
	}
	mirrors := repository.NewMirrorList(cfg.Repository.URL, cfg.Repository.Mirrors)
	client := repository.NewClient(cfg.Download.TimeoutSeconds)
	return repository.NewMetadataFetcher(client, cache, mirrors), nil
}

// buildDeps constructs the full dependency chain from config.
func buildDeps(cfg *config.Config) (*repository.Resolver, *install.Installer, *storage.RegistryManager, error) {
	cache, err := repository.NewCache()
	if err != nil {
		return nil, nil, nil, err
	}

	mirrors := repository.NewMirrorList(cfg.Repository.URL, cfg.Repository.Mirrors)
	client := repository.NewClient(cfg.Download.TimeoutSeconds)
	fetcher := repository.NewMetadataFetcher(client, cache, mirrors)
	resolver := repository.NewResolver(fetcher)

	registry, err := storage.NewRegistryManager()
	if err != nil {
		return nil, nil, nil, err
	}

	installer := install.NewInstaller(resolver, registry)
	return resolver, installer, registry, nil
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
