package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/trollixx/qvm/internal/platform"
	"github.com/trollixx/qvm/internal/storage"
)

func (a *app) newPrefixCommand() *cli.Command {
	return &cli.Command{
		Name:            "prefix",
		Usage:           "Print the install directory of a Qt version",
		ArgsUsage:       "<version>",
		CommandNotFound: showHelpOnNotFound,
		Flags: []cli.Flag{
			newArchFlag(),
		},
		Action: a.runPrefix,
	}
}

func (a *app) runPrefix(_ context.Context, cmd *cli.Command) error {
	arg := cmd.Args().Get(0)
	if arg == "" {
		return newHintError("missing argument",
			"Usage:\n"+
				"  qvm prefix <version>         Print Qt install directory\n\n"+
				"Examples:\n"+
				"  qvm prefix 6.8.3\n"+
				"  cmake -DCMAKE_PREFIX_PATH=$(qvm prefix 6.8.3) ..")
	}

	registry, err := storage.NewRegistryManager()
	if err != nil {
		return fmt.Errorf("opening registry: %w", err)
	}

	reg, err := registry.Load()
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	return a.prefixQt(reg, arg, cmd.String("arch"))
}

func (a *app) prefixQt(reg *storage.Registry, version, arch string) error {
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
			return withHint(
				fmt.Errorf("Qt %s (arch: %s) is not installed", version, arch),
				fmt.Sprintf("To install it: qvm install %s --arch %s", version, arch),
			)
		}
		return withHint(
			fmt.Errorf("Qt %s is not installed", version),
			fmt.Sprintf("To install it: qvm install %s", version),
		)
	case 1:
		fmt.Fprintln(a.streams.Out, matches[0].InstallDir)
		return nil
	default:
		// Multiple archs installed - prefer the local machine's default.
		defaultArch := platform.Current().DefaultArch(version)
		for _, m := range matches {
			if m.Arch == defaultArch {
				fmt.Fprintln(a.streams.Out, m.InstallDir)
				return nil
			}
		}
		// No default match - ask the user to specify.
		archs := make([]string, len(matches))
		for i, m := range matches {
			archs[i] = m.Arch
		}
		return withHint(
			fmt.Errorf("Qt %s is installed for multiple archs: %s", version, strings.Join(archs, ", ")),
			fmt.Sprintf("Use --arch to specify which one, e.g.:\n  qvm prefix %s --arch %s", version, archs[0]),
		)
	}
}
