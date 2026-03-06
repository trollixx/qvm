package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/trollixx/qvm/internal/repository"
	"github.com/urfave/cli/v3"
)

// showHelpOnNotFound is a CommandNotFound handler that shows the command's help
// instead of printing "No help topic for ...". This is needed because positional
// args (e.g. "qt@5.15.2") are mistaken for subcommands when --help is used.
func showHelpOnNotFound(_ context.Context, cmd *cli.Command, _ string) {
	cli.ShowSubcommandHelp(cmd)
}

// validateFormat returns an error if format is not a supported output format.
func validateFormat(format string) error {
	switch format {
	case "text", "json":
		return nil
	default:
		return fmt.Errorf("unknown format %q: supported formats are text, json", format)
	}
}

// Shared flag definitions used across commands.

var formatFlag = &cli.StringFlag{
	Name:      "format",
	Aliases:   []string{"f"},
	Value:     "text",
	Usage:     "output format: text or json",
	Validator: validateFormat,
}

var archFlag = &cli.StringFlag{
	Name:    "arch",
	Aliases: []string{"a"},
	Usage:   "compiler/ABI target, e.g. win64_msvc2022_64",
}

var targetFlag = &cli.StringFlag{
	Name:  "target",
	Value: "desktop",
	Usage: "Qt target platform: desktop, android, wasm, ios, winrt",
}

var dirFlag = &cli.StringFlag{
	Name:  "dir",
	Usage: "override Qt install directory",
}

var yesFlag = &cli.BoolFlag{
	Name:    "yes",
	Aliases: []string{"y"},
	Usage:   "skip confirmation prompts",
}

var forceFlag = &cli.BoolFlag{
	Name:  "force",
	Usage: "re-install even if already installed",
}

var quietFlag = &cli.BoolFlag{
	Name:    "quiet",
	Aliases: []string{"q"},
	Usage:   "suppress progress output (for scripting)",
}

var dryRunFlag = &cli.BoolFlag{
	Name:  "dry-run",
	Usage: "resolve and print archives that would be downloaded, without installing",
}

var hostFlag = &cli.StringFlag{
	Name:      "host",
	Usage:     "host platform override; valid values: " + strings.Join(repository.ValidHosts, ", ") + " (default: auto-detect)",
	Validator: validateHost,
}

// validateHost returns an error if host is non-empty and not in ValidHosts.
func validateHost(host string) error {
	if host == "" {
		return nil
	}
	for _, h := range repository.ValidHosts {
		if host == h {
			return nil
		}
	}
	return fmt.Errorf("unknown host %q: valid hosts are %s", host, strings.Join(repository.ValidHosts, ", "))
}
