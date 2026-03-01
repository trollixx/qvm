package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/trollixx/qvm/internal/storage"
	"github.com/urfave/cli/v3"
)

func newPathCommand() *cli.Command {
	return &cli.Command{
		Name:      "path",
		Usage:     "Print the install directory of a Qt version or tool",
		ArgsUsage: "qt@<version> | <tool>[@<version>]",
		Flags: []cli.Flag{
			archFlag,
		},
		Action: runPath,
	}
}

func runPath(_ context.Context, cmd *cli.Command) error {
	arg := cmd.Args().Get(0)
	if arg == "" {
		return fmt.Errorf("missing argument\n\nUsage:\n  qvm path qt@<version>         Print Qt install directory\n  qvm path <tool>[@<version>]   Print tool install directory\n\nExamples:\n  qvm path qt@6.8.3\n  cmake -DCMAKE_PREFIX_PATH=$(qvm path qt@6.8.3) ..")
	}

	registry, err := storage.NewRegistryManager()
	if err != nil {
		return fmt.Errorf("opening registry: %w", err)
	}

	reg, err := registry.Load()
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	if strings.HasPrefix(arg, "qt@") {
		version := strings.TrimPrefix(arg, "qt@")
		arch := cmd.String("arch")
		return pathQt(reg, version, arch)
	}
	return pathTool(reg, arg)
}

func pathQt(reg *storage.Registry, version, arch string) error {
	for i := range reg.Qt {
		q := &reg.Qt[i]
		if q.Version == version && (arch == "" || q.Arch == arch) {
			fmt.Fprintln(os.Stdout, q.InstallDir)
			return nil
		}
	}
	if arch != "" {
		return fmt.Errorf("Qt %s (arch: %s) is not installed\n\nTo install it: qvm install qt@%s --arch %s", version, arch, version, arch)
	}
	return fmt.Errorf("Qt %s is not installed\n\nTo install it: qvm install qt@%s", version, version)
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
		return fmt.Errorf("tool %s@%s is not installed\n\nTo install it: qvm install %s@%s", toolName, toolVersion, toolName, toolVersion)
	}
	return fmt.Errorf("tool %s is not installed\n\nRun 'qvm list tools --all' to see available tools.", toolName)
}
