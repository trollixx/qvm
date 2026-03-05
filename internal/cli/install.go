package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/trollixx/qvm/internal/config"
	"github.com/trollixx/qvm/internal/install"
	"github.com/trollixx/qvm/internal/platform"
	"github.com/urfave/cli/v3"
)

var stderrIsTTY = isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())

func newInstallCommand() *cli.Command {
	return &cli.Command{
		Name:      "install",
		Aliases:   []string{"i"},
		Usage:     "Install a Qt version or tool",
		ArgsUsage: "qt@<version> | <tool>[@<version>]",
		Flags: []cli.Flag{
			archFlag,
			targetFlag,
			&cli.StringSliceFlag{
				Name:    "modules",
				Aliases: []string{"m"},
				Usage:   "comma-separated list of add-on modules to install (e.g. charts,webengine)",
			},
			&cli.StringSliceFlag{
				Name:  "docs",
				Usage: "install documentation; list modules (--docs=charts,webengine) or omit value for all (--docs)",
			},
			&cli.StringSliceFlag{
				Name:  "examples",
				Usage: "install examples; list modules (--examples=charts,webengine) or omit value for all (--examples)",
			},
			&cli.BoolFlag{
				Name:  "sources",
				Usage: "install Qt sources",
			},
			&cli.BoolFlag{
				Name:  "debug-info",
				Usage: "install debug information files",
			},
			forceFlag,
			dirFlag,
			quietFlag,
			dryRunFlag,
			hostFlag,
		},
		Action: runInstall,
	}
}

func runInstall(ctx context.Context, cmd *cli.Command) error {
	arg := cmd.Args().Get(0)
	if arg == "" {
		return fmt.Errorf("missing argument\n\nUsage:\n  qvm install qt@<version>         Install a Qt SDK\n  qvm install <tool>@<version>     Install a tool\n\nExamples:\n  qvm install qt@6.8.3\n  qvm install qtcreator@15.0.0")
	}

	if arg == "qt" {
		return runInstallQtPickVersion(ctx, cmd)
	}
	if strings.HasPrefix(arg, "qt@") {
		return runInstallQt(ctx, cmd, strings.TrimPrefix(arg, "qt@"))
	}
	return runInstallTool(ctx, cmd, arg)
}

// runInstallQtPickVersion handles "qvm install qt" — no version specified.
func runInstallQtPickVersion(_ context.Context, _ *cli.Command) error {
	return fmt.Errorf("specify a version: qvm install qt@<version>\n\nExample: qvm install qt@6.8.3")
}

func runInstallQt(ctx context.Context, cmd *cli.Command, version string) error {
	force := cmd.Bool("force")
	host := cmd.String("host")

	modules := cmd.StringSlice("modules")
	docs := cmd.StringSlice("docs")
	examples := cmd.StringSlice("examples")
	sources := cmd.Bool("sources")
	debugInfo := cmd.Bool("debug-info")

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	_, installer, _, err := buildDeps(cfg, host)
	if err != nil {
		return fmt.Errorf("initializing dependencies: %w", err)
	}

	// Determine arch.
	arch := cmd.String("arch")
	if arch == "" {
		if host != "" {
			arch = platform.DefaultArchForHost(host, version)
		}
		if arch == "" {
			arch = platform.Current().DefaultArch(version)
		}
	}

	// Normalize docs/examples: if flag was set with no values, use ["*"].
	if cmd.IsSet("docs") && len(docs) == 0 {
		docs = []string{"*"}
	}
	if cmd.IsSet("examples") && len(examples) == 0 {
		examples = []string{"*"}
	}

	installRoot := cmd.String("dir")
	if installRoot == "" {
		installRoot = cfg.Install.Dir
	}

	opts := install.Options{
		Version:     version,
		Arch:        arch,
		Modules:     modules,
		Docs:        docs,
		Examples:    examples,
		Sources:     sources,
		DebugInfo:   debugInfo,
		InstallRoot: installRoot,
		Concurrency: cfg.Download.Concurrency,
		Timeout:     cfg.Download.TimeoutSeconds,
		Force:       force,
		DryRun:      cmd.Bool("dry-run"),
	}

	quiet := cmd.Bool("quiet")
	progressCh := make(chan install.ProgressEvent, 256)
	doneCh := make(chan struct{})

	dryRun := cmd.Bool("dry-run")
	go func() {
		defer close(doneCh)
		for ev := range progressCh {
			if ev.Phase == "dryrun" {
				printDryRun(ev)
			} else if !quiet {
				printProgress(ev)
			}
		}
	}()

	installErr := installer.Install(ctx, opts, progressCh)
	close(progressCh)
	<-doneCh

	if dryRun {
		return installErr
	}

	if errors.Is(installErr, install.ErrUpToDate) {
		fmt.Fprintf(os.Stdout, "Qt %s (%s) is already installed. Use --force to reinstall.\n", version, arch)
		return nil
	}
	if installErr != nil {
		return fmt.Errorf("installation failed: %w\n\nRun 'qvm doctor' to diagnose issues.", installErr)
	}

	fmt.Fprintf(os.Stdout, "\nQt %s (%s) installed successfully.\n", version, arch)

	if !quiet && len(modules) == 0 {
		fmt.Fprintf(os.Stdout, "\nTip: Add-on modules are available (charts, webengine, multimedia, ...).\n")
		fmt.Fprintf(os.Stdout, "  Run 'qvm list qt@%s' to see them, or reinstall with:\n", version)
		fmt.Fprintf(os.Stdout, "  qvm install qt@%s -m <module1>,<module2>\n", version)
	}

	return nil
}

func runInstallTool(ctx context.Context, cmd *cli.Command, arg string) error {
	toolName := arg
	toolVersion := ""

	if idx := strings.Index(arg, "@"); idx >= 0 {
		toolName = arg[:idx]
		toolVersion = arg[idx+1:]
	}

	if toolVersion == "" {
		return fmt.Errorf("specify a version: qvm install %s@<version>", toolName)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	_, installer, _, err := buildDeps(cfg, cmd.String("host"))
	if err != nil {
		return fmt.Errorf("initializing dependencies: %w", err)
	}

	installDir := cmd.String("dir")
	if installDir == "" {
		installDir = cfg.ToolsDir()
	}

	toolOpts := install.ToolOptions{
		Name:        toolName,
		Version:     toolVersion,
		InstallDir:  installDir,
		Concurrency: cfg.Download.Concurrency,
		Timeout:     cfg.Download.TimeoutSeconds,
	}

	quiet := cmd.Bool("quiet")
	progressCh := make(chan install.ProgressEvent, 256)
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		for ev := range progressCh {
			if !quiet {
				printProgress(ev)
			}
		}
	}()

	installErr := installer.InstallTool(ctx, toolOpts, progressCh)
	close(progressCh)
	<-doneCh

	if installErr != nil {
		return fmt.Errorf("tool installation failed: %w\n\nRun 'qvm doctor' to diagnose issues.", installErr)
	}

	fmt.Fprintf(os.Stdout, "\nTool %s@%s installed successfully.\n", toolName, toolVersion)
	return nil
}

func printDryRun(ev install.ProgressEvent) {
	if ev.Archive == "" {
		// Header event — emitted once with ArchiveTotal set.
		fmt.Fprintf(os.Stdout, "Dry run — would download %d archive(s):\n", ev.ArchiveTotal)
		return
	}
	size := ""
	if ev.BytesTotal > 0 {
		size = "  " + formatSize(ev.BytesTotal)
	}
	fmt.Fprintf(os.Stdout, "  %s%s\n", ev.Archive, size)
}

func printProgress(ev install.ProgressEvent) {
	// \r\033[K moves cursor to start of line and clears to end (ANSI escape).
	cr := "\r\033[K"
	if !stderrIsTTY {
		cr = "\n"
	}

	switch ev.Phase {
	case "resolving":
		fmt.Fprintf(os.Stderr, "Resolving archives...\n")
	case "downloading":
		if ev.Archive != "" {
			counter := ""
			if ev.ArchiveTotal > 0 {
				counter = fmt.Sprintf(" [%d/%d]", ev.ArchiveIndex, ev.ArchiveTotal)
			}
			pct := ""
			if ev.BytesTotal > 0 {
				pct = fmt.Sprintf(" %.0f%%", float64(ev.BytesDone)/float64(ev.BytesTotal)*100)
			}
			speed := ""
			if ev.Speed > 0 {
				speed = fmt.Sprintf(" @ %s/s", formatSize(int64(ev.Speed)))
			}
			fmt.Fprintf(os.Stderr, "%sDownloading %s%s%s%s", cr, ev.Archive, counter, pct, speed)
		}
	case "verifying":
		fmt.Fprintf(os.Stderr, "\nVerifying %s...\n", ev.Archive)
	case "extracting":
		if ev.Archive != "" {
			pct := ""
			if ev.BytesTotal > 0 {
				pct = fmt.Sprintf(" %.0f%%", float64(ev.BytesDone)/float64(ev.BytesTotal)*100)
			}
			fmt.Fprintf(os.Stderr, "%sExtracting %s%s", cr, ev.Archive, pct)
		} else {
			fmt.Fprintf(os.Stderr, "\nExtracting archives...\n")
		}
	case "patching":
		fmt.Fprintf(os.Stderr, "\nPatching qt.conf...\n")
	case "registering":
		fmt.Fprintf(os.Stderr, "Registering installation...\n")
	case "done":
		fmt.Fprintf(os.Stderr, "Done.\n")
	}
}
