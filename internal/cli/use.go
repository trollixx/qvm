package cli

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/trollixx/qvm/internal/config"
	"github.com/trollixx/qvm/internal/storage"
	"github.com/trollixx/qvm/pkg/qtmeta"
)

func (a *app) newUseCommand() *cli.Command {
	return &cli.Command{
		Name:            "use",
		Usage:           "Set the default Qt version for qvm commands",
		ArgsUsage:       "[<version>]",
		CommandNotFound: showHelpOnNotFound,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "unset",
				Usage: "clear the default version",
			},
		},
		Action: a.runUse,
	}
}

func (a *app) runUse(_ context.Context, cmd *cli.Command) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if cmd.Bool("unset") {
		if cmd.Args().Get(0) != "" {
			return newHintError("cannot combine --unset with a version argument",
				"Use 'qvm use --unset' alone to clear the default.")
		}
		cfg.Qt.Default = ""
		if saveErr := config.Save(cfg); saveErr != nil {
			return fmt.Errorf("saving config: %w", saveErr)
		}
		fmt.Fprintln(a.streams.Out, "Default Qt version cleared.")
		return nil
	}

	version := cmd.Args().Get(0)
	if version == "" {
		if cfg.Qt.Default == "" {
			return newHintError("no default Qt version set",
				"Set one with: qvm use <version>")
		}
		fmt.Fprintln(a.streams.Out, cfg.Qt.Default)
		return nil
	}

	registry, err := storage.NewRegistryManager()
	if err != nil {
		return fmt.Errorf("opening registry: %w", err)
	}
	reg, err := registry.Load()
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	if setErr := setDefaultQt(reg, cfg, version); setErr != nil {
		return setErr
	}
	if saveErr := config.Save(cfg); saveErr != nil {
		return fmt.Errorf("saving config: %w", saveErr)
	}
	fmt.Fprintf(a.streams.Out, "Default Qt version set to %s.\n", version)
	return nil
}

// setDefaultQt validates that version is installed and records it as the
// default in cfg. Saving the config is the caller's responsibility.
func setDefaultQt(reg *storage.Registry, cfg *config.Config, version string) error {
	for _, q := range reg.Qt {
		if q.Version == version {
			cfg.Qt.Default = version
			return nil
		}
	}
	return withHint(
		fmt.Errorf("Qt %s is not installed", version),
		fmt.Sprintf("To install it: qvm install %s\nRun 'qvm list' to see installed versions.", version),
	)
}

// resolveVersionArg returns arg, or the configured default Qt version when arg
// is empty. An empty result means neither was provided; the caller supplies
// its own usage hint.
func resolveVersionArg(arg string) (string, error) {
	if arg != "" {
		return arg, nil
	}
	cfg, err := config.Load()
	if err != nil {
		return "", fmt.Errorf("loading config: %w", err)
	}
	return cfg.Qt.Default, nil
}

// looksLikeVersion reports whether s is a full Qt version (major.minor.patch).
func looksLikeVersion(s string) bool {
	_, err := qtmeta.ParseVersion(s)
	return err == nil
}
