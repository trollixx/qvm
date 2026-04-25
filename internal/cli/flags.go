package cli

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/trollixx/qvm/internal/repository"
)

const formatJSON = "json"

// showHelpOnNotFound is a CommandNotFound handler that shows the command's help
// instead of printing "No help topic for ...". This is needed because positional
// args (e.g. "6.8.3") are mistaken for subcommands when --help is used.
func showHelpOnNotFound(_ context.Context, cmd *cli.Command, _ string) {
	_ = cli.ShowSubcommandHelp(cmd)
}

// validateFormat returns an error if format is not a supported output format.
func validateFormat(format string) error {
	switch format {
	case "text", formatJSON:
		return nil
	default:
		return fmt.Errorf("unknown format %q: supported formats are text, json", format)
	}
}

// Shared flag constructor functions used across commands.
// Each command gets its own flag instance to avoid shared state.

func newFormatFlag() *cli.StringFlag {
	return &cli.StringFlag{
		Name:      "format",
		Aliases:   []string{"f"},
		Value:     "text",
		Usage:     "output format: text or json",
		Validator: validateFormat,
	}
}

func newArchFlag() *cli.StringFlag {
	return &cli.StringFlag{
		Name:    "arch",
		Aliases: []string{"a"},
		Usage:   "compiler/ABI target, e.g. win64_msvc2022_64",
	}
}

func newTargetFlag() *cli.StringFlag {
	return &cli.StringFlag{
		Name:      "target",
		Value:     repository.TargetDesktop,
		Usage:     "Qt target platform: " + strings.Join(repository.ValidTargets, ", "),
		Validator: validateTarget,
	}
}

// validateTarget returns an error if target is non-empty and not in ValidTargets.
func validateTarget(target string) error {
	if target == "" {
		return nil
	}
	if slices.Contains(repository.ValidTargets, target) {
		return nil
	}
	return fmt.Errorf("unknown target %q: valid targets are %s",
		target, strings.Join(repository.ValidTargets, ", "))
}

func newDirFlag() *cli.StringFlag {
	return &cli.StringFlag{
		Name:  "dir",
		Usage: "override Qt install directory",
	}
}

func newYesFlag() *cli.BoolFlag {
	return &cli.BoolFlag{
		Name:    "yes",
		Aliases: []string{"y"},
		Usage:   "skip confirmation prompts",
	}
}

func newForceFlag() *cli.BoolFlag {
	return &cli.BoolFlag{
		Name:  "force",
		Usage: "re-install even if already installed",
	}
}

func newQuietFlag() *cli.BoolFlag {
	return &cli.BoolFlag{
		Name:    "quiet",
		Aliases: []string{"q"},
		Usage:   "suppress progress output (for scripting)",
	}
}

func newDryRunFlag() *cli.BoolFlag {
	return &cli.BoolFlag{
		Name:  "dry-run",
		Usage: "resolve and print archives that would be downloaded, without installing",
	}
}

func newHostFlag() *cli.StringFlag {
	return &cli.StringFlag{
		Name: "host",
		Usage: "host platform override; valid values: " + strings.Join(
			repository.ValidHosts,
			", ",
		) + " (default: auto-detect)",
		Validator: validateHost,
	}
}

// validateHost returns an error if host is non-empty and not in ValidHosts.
func validateHost(host string) error {
	if host == "" {
		return nil
	}
	if slices.Contains(repository.ValidHosts, host) {
		return nil
	}
	return fmt.Errorf("unknown host %q: valid hosts are %s", host, strings.Join(repository.ValidHosts, ", "))
}
