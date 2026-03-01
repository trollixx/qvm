package cli

import (
	"fmt"

	"github.com/urfave/cli/v3"
)

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
	Name:    "format",
	Aliases: []string{"f"},
	Value:   "text",
	Usage:   "output format: text or json",
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
