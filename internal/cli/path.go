package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/trollixx/qvm/internal/platform"
	"github.com/trollixx/qvm/internal/storage"
)

func newPathCommand() *cli.Command {
	return &cli.Command{
		Name:            "path",
		Usage:           "Print the install directory of a Qt version or tool",
		ArgsUsage:       "qt@<version> | <tool>[@<version>]",
		CommandNotFound: showHelpOnNotFound,
		Flags: []cli.Flag{
			newArchFlag(),
		},
		Action: runPath,
	}
}

func runPath(_ context.Context, cmd *cli.Command) error {
	arg := cmd.Args().Get(0)
	if arg == "" {
		return fmt.Errorf(
			"missing argument\n\nUsage:\n  qvm path qt@<version>         Print Qt install directory\n  qvm path <tool>[@<version>]   Print tool install directory\n\nExamples:\n  qvm path qt@6.8.3\n  cmake -DCMAKE_PREFIX_PATH=$(qvm path qt@6.8.3) ..",
		)
	}

	registry, err := storage.NewRegistryManager()
	if err != nil {
		return fmt.Errorf("opening registry: %w", err)
	}

	reg, err := registry.Load()
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	if version, ok := strings.CutPrefix(arg, "qt@"); ok {
		arch := cmd.String("arch")
		return pathQt(reg, version, arch)
	}
	return pathTool(reg, arg)
}

func pathQt(reg *storage.Registry, version, arch string) error {
	// Collect all matching installs.
	var matches []storage.InstalledQt
	for i := range reg.Qt {
		q := &reg.Qt[i]
		if q.Version == version && (arch == "" || q.Arch == arch) {
			matches = append(matches, *q)
		}
	}

	switch len(matches) {
	case 0:
		if arch != "" {
			return fmt.Errorf(
				"Qt %s (arch: %s) is not installed\n\nTo install it: qvm install qt@%s --arch %s",
				version,
				arch,
				version,
				arch,
			)
		}
		return fmt.Errorf("Qt %s is not installed\n\nTo install it: qvm install qt@%s", version, version)
	case 1:
		fmt.Fprintln(os.Stdout, matches[0].InstallDir)
		return nil
	default:
		// Multiple archs installed - prefer the local machine's default.
		defaultArch := platform.Current().DefaultArch(version)
		for _, m := range matches {
			if m.Arch == defaultArch {
				fmt.Fprintln(os.Stdout, m.InstallDir)
				return nil
			}
		}
		// No default match - ask the user to specify.
		archs := make([]string, len(matches))
		for i, m := range matches {
			archs[i] = m.Arch
		}
		return fmt.Errorf(
			"Qt %s is installed for multiple archs: %s\n\nUse --arch to specify which one, e.g.:\n  qvm path qt@%s --arch %s",
			version,
			strings.Join(archs, ", "),
			version,
			archs[0],
		)
	}
}

func pathTool(reg *storage.Registry, arg string) error {
	toolName := arg
	toolVersion := ""
	if idx := strings.Index(arg, "@"); idx >= 0 {
		toolName = arg[:idx]
		toolVersion = arg[idx+1:]
	}

	for i := range reg.Tools {
		t := &reg.Tools[i]
		if t.Name == toolName && (toolVersion == "" || t.Version == toolVersion) {
			fmt.Fprintln(os.Stdout, t.InstallDir)
			return nil
		}
	}
	if toolVersion != "" {
		return fmt.Errorf(
			"tool %s@%s is not installed\n\nTo install it: qvm install %s@%s",
			toolName,
			toolVersion,
			toolName,
			toolVersion,
		)
	}
	return fmt.Errorf("tool %s is not installed\n\nRun 'qvm list tools --all' to see available tools.", toolName)
}
