package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/trollixx/qvm/internal/config"
	"github.com/trollixx/qvm/internal/storage"
)

func newUninstallCommand() *cli.Command {
	return &cli.Command{
		Name:            "uninstall",
		Aliases:         []string{"remove"},
		Usage:           "Uninstall a Qt version or tool",
		ArgsUsage:       "qt@<version> | <tool>@<version>",
		CommandNotFound: showHelpOnNotFound,
		Flags: []cli.Flag{
			archFlag,
			yesFlag,
		},
		Action: runUninstall,
	}
}

func runUninstall(ctx context.Context, cmd *cli.Command) error {
	_ = ctx

	arg := cmd.Args().Get(0)
	if arg == "" {
		return fmt.Errorf(
			"missing argument\n\nUsage:\n  qvm uninstall qt@<version>       Uninstall a Qt version\n  qvm uninstall <tool>@<version>   Uninstall a tool\n\nExamples:\n  qvm uninstall qt@6.8.3\n  qvm uninstall qtcreator@15.0.0",
		)
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

	if strings.HasPrefix(arg, "qt@") {
		version := strings.TrimPrefix(arg, "qt@")
		return runUninstallQt(ctx, cmd, cfg, registry, version, autoYes)
	}
	return runUninstallTool(ctx, cmd, cfg, registry, arg, autoYes)
}

func runUninstallQt(
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
			return fmt.Errorf(
				"Qt %s (arch: %s) is not installed\n\nRun 'qvm list' to see installed versions.",
				version,
				arch,
			)
		}
		return fmt.Errorf("Qt %s is not installed\n\nRun 'qvm list' to see installed versions.", version)
	}

	if !autoYes {
		fmt.Fprintf(os.Stdout, "The following installations will be removed:\n")
		for _, q := range matches {
			size := ""
			if q.SizeBytes > 0 {
				size = "  (" + formatSize(q.SizeBytes) + ")"
			}
			fmt.Fprintf(os.Stdout, "  Qt %s  %s  %s%s\n", q.Version, q.Arch, q.InstallDir, size)
		}
		if !confirm("Proceed?") {
			fmt.Fprintln(os.Stdout, "Aborted.")
			return nil
		}
	}

	if err := storage.Cleanup(registry, version, arch, cfg.Install.Dir); err != nil {
		return fmt.Errorf("uninstall failed: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Qt %s uninstalled successfully.\n", version)
	return nil
}

func runUninstallTool(
	ctx context.Context,
	cmd *cli.Command,
	cfg *config.Config,
	registry *storage.RegistryManager,
	arg string,
	autoYes bool,
) error {
	_ = ctx
	_ = cmd

	at := strings.Index(arg, "@")
	if at < 0 {
		return fmt.Errorf(
			"missing version\n\nUsage:\n  qvm uninstall <tool>@<version>\n\nExample:\n  qvm uninstall %s@<version>",
			arg,
		)
	}

	toolName := arg[:at]
	toolVersion := arg[at+1:]

	reg, err := registry.Load()
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	var matches []storage.InstalledTool
	for _, t := range reg.Tools {
		if t.Name == toolName && (toolVersion == "" || t.Version == toolVersion) {
			matches = append(matches, t)
		}
	}

	if len(matches) == 0 {
		return fmt.Errorf(
			"tool %s@%s is not installed\n\nRun 'qvm list' to see installed versions.",
			toolName,
			toolVersion,
		)
	}

	if !autoYes {
		fmt.Fprintf(os.Stdout, "The following tools will be removed:\n")
		for _, t := range matches {
			size := ""
			if t.SizeBytes > 0 {
				size = "  (" + formatSize(t.SizeBytes) + ")"
			}
			fmt.Fprintf(os.Stdout, "  %s@%s  %s%s\n", t.Name, t.Version, t.InstallDir, size)
		}
		if !confirm("Proceed?") {
			fmt.Fprintln(os.Stdout, "Aborted.")
			return nil
		}
	}

	if err := storage.CleanupTool(registry, toolName, toolVersion, cfg.ToolsDir()); err != nil {
		return fmt.Errorf("uninstall failed: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Tool %s@%s uninstalled successfully.\n", toolName, toolVersion)
	return nil
}

func confirm(prompt string) bool {
	fmt.Fprintf(os.Stdout, "%s [y/N] ", prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return false
	}
	resp := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return resp == "y" || resp == "yes"
}
