package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/trollixx/qvm/internal/config"
	"github.com/trollixx/qvm/internal/storage"
)

func (a *app) newUninstallCommand() *cli.Command {
	return &cli.Command{
		Name:            "uninstall",
		Aliases:         []string{"remove"},
		Usage:           "Uninstall a Qt version",
		ArgsUsage:       "<version>",
		CommandNotFound: showHelpOnNotFound,
		Flags: []cli.Flag{
			newArchFlag(),
			newYesFlag(),
		},
		Action: a.runUninstall,
	}
}

func (a *app) runUninstall(ctx context.Context, cmd *cli.Command) error {
	_ = ctx

	arg := cmd.Args().Get(0)
	if arg == "" {
		return errors.New("missing argument\n\n" +
			"Usage:\n" +
			"  qvm uninstall <version>       Uninstall a Qt version\n\n" +
			"Example:\n" +
			"  qvm uninstall 6.8.3")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	registry, err := storage.NewRegistryManager()
	if err != nil {
		return fmt.Errorf("opening registry: %w", err)
	}

	autoYes := cmd.Bool("yes")

	return a.runUninstallQt(ctx, cmd, cfg, registry, arg, autoYes)
}

func (a *app) runUninstallQt(
	ctx context.Context,
	cmd *cli.Command,
	cfg *config.Config,
	registry *storage.RegistryManager,
	version string,
	autoYes bool,
) error {
	_ = ctx

	arch := cmd.String("arch")

	reg, err := registry.Load()
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	// Find matching installations.
	var matches []storage.InstalledQt
	for _, q := range reg.Qt {
		if q.Version == version && (arch == "" || q.Arch == arch) {
			matches = append(matches, q)
		}
	}

	if len(matches) == 0 {
		if arch != "" {
			return withHint(
				fmt.Errorf("Qt %s (arch: %s) is not installed", version, arch),
				"Run 'qvm list' to see installed versions.",
			)
		}
		return withHint(
			fmt.Errorf("Qt %s is not installed", version),
			"Run 'qvm list' to see installed versions.",
		)
	}

	if !autoYes {
		fmt.Fprintf(a.streams.Out, "The following installations will be removed:\n")
		for _, q := range matches {
			size := ""
			if q.SizeBytes > 0 {
				size = "  (" + formatSize(q.SizeBytes) + ")"
			}
			fmt.Fprintf(a.streams.Out, "  Qt %s  %s  %s%s\n", q.Version, q.Arch, q.InstallDir, size)
		}
		if !a.confirm("Proceed?") {
			fmt.Fprintln(a.streams.Out, "Aborted.")
			return nil
		}
	}

	err = storage.Cleanup(registry, version, arch, cfg.Install.Dir)
	if err != nil {
		return fmt.Errorf("uninstall failed: %w", err)
	}

	fmt.Fprintf(a.streams.Out, "Qt %s uninstalled successfully.\n", version)
	return nil
}

func (a *app) confirm(prompt string) bool {
	fmt.Fprintf(a.streams.Out, "%s [y/N] ", prompt)
	scanner := bufio.NewScanner(a.streams.In)
	if !scanner.Scan() {
		return false
	}
	resp := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return resp == "y" || resp == "yes"
}
