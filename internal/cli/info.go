package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/trollixx/qvm/internal/storage"
	"github.com/urfave/cli/v3"
)

func newInfoCommand() *cli.Command {
	return &cli.Command{
		Name:            "info",
		Aliases:         []string{"show"},
		Usage:           "Show detailed info about an installed Qt version or tool",
		ArgsUsage:       "qt@<version> | <tool>[@<version>]",
		CommandNotFound: showHelpOnNotFound,
		Flags: []cli.Flag{
			archFlag,
			formatFlag,
		},
		Action: runInfo,
	}
}

func runInfo(ctx context.Context, cmd *cli.Command) error {
	_ = ctx

	arg := cmd.Args().Get(0)
	if arg == "" {
		return fmt.Errorf("missing argument\n\nUsage:\n  qvm info qt@<version>            Show Qt version details\n  qvm info <tool>[@<version>]      Show tool details\n\nExample:\n  qvm info qt@6.8.3")
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

	if strings.HasPrefix(arg, "qt@") {
		return runInfoQt(reg, strings.TrimPrefix(arg, "qt@"), cmd.String("arch"), format)
	}
	return runInfoTool(reg, arg, format)
}

func runInfoQt(reg *storage.Registry, version, arch, format string) error {
	var matches []storage.InstalledQt
	for _, q := range reg.Qt {
		if q.Version == version && (arch == "" || q.Arch == arch) {
			matches = append(matches, q)
		}
	}

	if len(matches) == 0 {
		if arch != "" {
			return fmt.Errorf("Qt %s (arch: %s) is not installed\n\nTo install it: qvm install qt@%s --arch %s", version, arch, version, arch)
		}
		return fmt.Errorf("Qt %s is not installed\n\nTo install it: qvm install qt@%s", version, version)
	}

	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if len(matches) == 1 {
			return enc.Encode(matches[0])
		}
		return enc.Encode(matches)
	}

	for _, q := range matches {
		printQtInfo(q)
		fmt.Fprintln(os.Stdout)
	}
	return nil
}

func runInfoTool(reg *storage.Registry, arg, format string) error {
	toolName := arg
	toolVersion := ""
	if idx := strings.Index(arg, "@"); idx >= 0 {
		toolName = arg[:idx]
		toolVersion = arg[idx+1:]
	}

	var matches []storage.InstalledTool
	for _, t := range reg.Tools {
		if t.Name == toolName && (toolVersion == "" || t.Version == toolVersion) {
			matches = append(matches, t)
		}
	}

	if len(matches) == 0 {
		if toolVersion != "" {
			return fmt.Errorf("tool %s@%s is not installed\n\nTo install it: qvm install %s@%s", toolName, toolVersion, toolName, toolVersion)
		}
		return fmt.Errorf("tool %s is not installed\n\nRun 'qvm list tools --all' to see available tools.", toolName)
	}

	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if len(matches) == 1 {
			return enc.Encode(matches[0])
		}
		return enc.Encode(matches)
	}

	for _, t := range matches {
		printToolInfo(t)
		fmt.Fprintln(os.Stdout)
	}
	return nil
}

func printToolInfo(t storage.InstalledTool) {
	fmt.Fprintf(os.Stdout, "%s  %s\n", t.Name, t.Version)
	fmt.Fprintf(os.Stdout, "  Install dir:  %s\n", t.InstallDir)
	fmt.Fprintf(os.Stdout, "  Installed at: %s\n", t.InstalledAt.Format("2006-01-02 15:04:05"))
	if t.SizeBytes > 0 {
		fmt.Fprintf(os.Stdout, "  Size:         %s\n", formatSize(t.SizeBytes))
	}
}

func printQtInfo(q storage.InstalledQt) {
	fmt.Fprintf(os.Stdout, "Qt %s  %s\n", q.Version, q.Arch)
	fmt.Fprintf(os.Stdout, "  Install dir:  %s\n", q.InstallDir)
	fmt.Fprintf(os.Stdout, "  Installed at: %s\n", q.InstalledAt.Format("2006-01-02 15:04:05"))
	if q.SizeBytes > 0 {
		fmt.Fprintf(os.Stdout, "  Size:         %s\n", formatSize(q.SizeBytes))
	}

	if len(q.Modules) > 0 {
		fmt.Fprintf(os.Stdout, "  Modules:      %s\n", strings.Join(q.Modules, ", "))
	} else {
		fmt.Fprintf(os.Stdout, "  Modules:      (essentials only)\n")
	}

	if q.Extras.Docs {
		fmt.Fprintf(os.Stdout, "  Docs:         yes\n")
	}
	if q.Extras.Examples {
		fmt.Fprintf(os.Stdout, "  Examples:     yes\n")
	}
	if q.Extras.Sources {
		fmt.Fprintf(os.Stdout, "  Sources:      yes\n")
	}
	if q.Extras.DebugInfo {
		fmt.Fprintf(os.Stdout, "  Debug symbols: yes\n")
	}

	// Show qmake path.
	qmakePath := findQmakeInDir(q.InstallDir)
	if qmakePath != "" {
		fmt.Fprintf(os.Stdout, "  qmake:        %s\n", qmakePath)
	}
}
