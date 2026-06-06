package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/trollixx/qvm/internal/storage"
)

func (a *app) newInfoCommand() *cli.Command {
	return &cli.Command{
		Name:            "info",
		Aliases:         []string{"show"},
		Usage:           "Show detailed info about an installed Qt version",
		ArgsUsage:       "[<version>]",
		CommandNotFound: showHelpOnNotFound,
		Flags: []cli.Flag{
			newArchFlag(),
			newFormatFlag(),
		},
		Action: a.runInfo,
	}
}

func (a *app) runInfo(ctx context.Context, cmd *cli.Command) error {
	_ = ctx

	arg, err := resolveVersionArg(cmd.Args().Get(0))
	if err != nil {
		return err
	}
	if arg == "" {
		return newHintError("missing argument",
			"Usage:\n"+
				"  qvm info [<version>]          Show Qt version details\n\n"+
				"Example:\n"+
				"  qvm info 6.8.3\n\n"+
				"Tip: set a default version with 'qvm use <version>' to omit it here.")
	}

	format := cmd.String("format")

	registry, err := storage.NewRegistryManager()
	if err != nil {
		return fmt.Errorf("opening registry: %w", err)
	}

	reg, err := registry.Load()
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	return a.runInfoQt(reg, arg, cmd.String("arch"), format)
}

func (a *app) runInfoQt(reg *storage.Registry, version, arch, format string) error {
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
				fmt.Sprintf("To install it: qvm install %s --arch %s", version, arch),
			)
		}
		return withHint(
			fmt.Errorf("Qt %s is not installed", version),
			fmt.Sprintf("To install it: qvm install %s", version),
		)
	}

	if format == formatJSON {
		enc := json.NewEncoder(a.streams.Out)
		enc.SetIndent("", "  ")
		if len(matches) == 1 {
			return enc.Encode(matches[0])
		}
		return enc.Encode(matches)
	}

	for _, q := range matches {
		a.printQtInfo(q)
		fmt.Fprintln(a.streams.Out)
	}
	return nil
}

func (a *app) printQtInfo(q storage.InstalledQt) {
	fmt.Fprintf(a.streams.Out, "Qt %s  %s\n", q.Version, q.Arch)
	fmt.Fprintf(a.streams.Out, "  Install dir:  %s\n", q.InstallDir)
	fmt.Fprintf(a.streams.Out, "  Installed at: %s\n", q.InstalledAt.Format("2006-01-02 15:04:05"))
	if q.SizeBytes > 0 {
		fmt.Fprintf(a.streams.Out, "  Size:         %s\n", formatSize(q.SizeBytes))
	}

	if len(q.Modules) > 0 {
		fmt.Fprintf(a.streams.Out, "  Modules:      %s\n", strings.Join(q.Modules, ", "))
	} else {
		fmt.Fprintf(a.streams.Out, "  Modules:      (essentials only)\n")
	}

	if q.Extras.Docs {
		fmt.Fprintf(a.streams.Out, "  Docs:         yes\n")
	}
	if q.Extras.Examples {
		fmt.Fprintf(a.streams.Out, "  Examples:     yes\n")
	}
	if q.Extras.Sources {
		fmt.Fprintf(a.streams.Out, "  Sources:      yes\n")
	}
	if q.Extras.DebugInfo {
		fmt.Fprintf(a.streams.Out, "  Debug symbols: yes\n")
	}

	// Show qmake path.
	qmakePath := findQmakeInDir(q.InstallDir)
	if qmakePath != "" {
		fmt.Fprintf(a.streams.Out, "  qmake:        %s\n", qmakePath)
	}
}
