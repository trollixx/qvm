package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/trollixx/qvm/internal/config"
	"github.com/trollixx/qvm/internal/install"
	"github.com/trollixx/qvm/internal/platform"
)

func (a *app) newInstallCommand() *cli.Command {
	return &cli.Command{
		Name:            "install",
		Aliases:         []string{"i"},
		Usage:           "Install a Qt version or tool",
		ArgsUsage:       "qt@<version> | <tool>[@<version>]",
		CommandNotFound: showHelpOnNotFound,
		Flags: []cli.Flag{
			// What to install.
			&cli.StringSliceFlag{
				Name:    "modules",
				Aliases: []string{"m"},
				Usage:   "comma-separated list of add-on modules to install (e.g. charts,webengine)",
			},
			newArchFlag(),
			newTargetFlag(),
			newHostFlag(),

			// Extra content.
			&cli.BoolFlag{
				Name:  "docs",
				Usage: "install documentation for all selected modules",
			},
			&cli.BoolFlag{
				Name:  "examples",
				Usage: "install examples for all selected modules",
			},
			&cli.BoolFlag{
				Name:  "sources",
				Usage: "install Qt sources",
			},
			&cli.BoolFlag{
				Name:  "debug-symbols",
				Usage: "install debug symbol files",
			},

			// Behavior.
			newForceFlag(),
			newDryRunFlag(),
			newQuietFlag(),
			newDirFlag(),
		},
		Action: a.runInstall,
	}
}

func (a *app) runInstall(ctx context.Context, cmd *cli.Command) error {
	arg := cmd.Args().Get(0)
	if arg == "" {
		return errors.New("missing argument\n\n" +
			"Usage:\n" +
			"  qvm install qt@<version>         Install a Qt SDK\n" +
			"  qvm install <tool>@<version>     Install a tool\n\n" +
			"Examples:\n" +
			"  qvm install qt@6.8.3\n" +
			"  qvm install qtcreator@15.0.0")
	}

	if arg == "qt" {
		return a.runInstallQtPickVersion(ctx, cmd)
	}
	if version, ok := strings.CutPrefix(arg, "qt@"); ok {
		return a.runInstallQt(ctx, cmd, version)
	}
	return a.runInstallTool(ctx, cmd, arg)
}

// runInstallQtPickVersion handles "qvm install qt" - no version specified.
func (a *app) runInstallQtPickVersion(_ context.Context, _ *cli.Command) error {
	return errors.New("specify a version: qvm install qt@<version>\n\nExample: qvm install qt@6.8.3")
}

func (a *app) runInstallQt(ctx context.Context, cmd *cli.Command, version string) error {
	force := cmd.Bool("force")
	host := cmd.String("host")

	modules := cmd.StringSlice("modules")
	sources := cmd.Bool("sources")
	debugInfo := cmd.Bool("debug-symbols")

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	installer, err := buildDeps(cfg, host)
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

	installRoot := cmd.String("dir")
	if installRoot == "" {
		installRoot = cfg.Install.Dir
	}

	opts := install.Options{
		Version:     version,
		Arch:        arch,
		Modules:     modules,
		Docs:        cmd.Bool("docs"),
		Examples:    cmd.Bool("examples"),
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
	printer := a.streams.NewProgressPrinter()
	go func() {
		defer close(doneCh)
		for ev := range progressCh {
			if ev.Phase == "dryrun" {
				a.printDryRun(ev)
			} else if !quiet {
				printer.print(ev)
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
		fmt.Fprintf(a.streams.Out, "Qt %s (%s) is already installed. Use --force to reinstall.\n", version, arch)
		return nil
	}
	if installErr != nil {
		return fmt.Errorf("installation failed: %w\n\nRun 'qvm doctor' to diagnose issues.", installErr)
	}

	fmt.Fprintf(a.streams.Out, "\nQt %s (%s) installed successfully.\n", version, arch)

	if !quiet && len(modules) == 0 {
		fmt.Fprintf(a.streams.Out, "\nTip: Add-on modules are available (charts, webengine, multimedia, ...).\n")
		fmt.Fprintf(a.streams.Out, "  Run 'qvm list qt@%s' to see them, or reinstall with:\n", version)
		fmt.Fprintf(a.streams.Out, "  qvm install qt@%s -m <module1>,<module2>\n", version)
	}

	return nil
}

func (a *app) runInstallTool(ctx context.Context, cmd *cli.Command, arg string) error {
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

	installer, err := buildDeps(cfg, cmd.String("host"))
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

	printer := a.streams.NewProgressPrinter()
	go func() {
		defer close(doneCh)
		for ev := range progressCh {
			if !quiet {
				printer.print(ev)
			}
		}
	}()

	installErr := installer.InstallTool(ctx, toolOpts, progressCh)
	close(progressCh)
	<-doneCh

	if installErr != nil {
		return fmt.Errorf("tool installation failed: %w\n\nRun 'qvm doctor' to diagnose issues.", installErr)
	}

	fmt.Fprintf(a.streams.Out, "\nTool %s@%s installed successfully.\n", toolName, toolVersion)
	return nil
}

func (a *app) printDryRun(ev install.ProgressEvent) {
	if ev.Archive == "" {
		// Header event - emitted once with ArchiveTotal set.
		fmt.Fprintf(a.streams.Out, "Dry run - would download %d archive(s):\n", ev.ArchiveTotal)
		return
	}
	size := ""
	if ev.BytesTotal > 0 {
		size = "  " + formatSize(ev.BytesTotal)
	}
	fmt.Fprintf(a.streams.Out, "  %s%s\n", ev.Archive, size)
}

// progressPrinter renders install progress events to stderr.
// isTTY controls whether cursor-movement escape sequences are used.
type progressPrinter struct {
	isTTY bool
	w     io.Writer
	lines map[string]int // archive name -> line index (0-based from first download line)
	total int            // number of lines printed so far
}

func (p *progressPrinter) print(ev install.ProgressEvent) {
	switch ev.Phase {
	case "resolving":
		fmt.Fprintf(p.w, "Resolving archives...\n")
	case "downloading":
		if ev.Archive == "" {
			// Reset tracker for each install.
			p.lines = make(map[string]int)
			p.total = 0
			return
		}
		if !p.isTTY {
			// Non-TTY: only print once when a file finishes downloading.
			if ev.BytesTotal > 0 && ev.BytesDone == ev.BytesTotal {
				if _, reported := p.lines[ev.Archive]; !reported {
					p.lines[ev.Archive] = 0
					fmt.Fprintf(p.w, "  Downloaded %s\n", ev.Archive)
				}
			}
			return
		}

		line := formatDownloadLine(ev)
		idx, exists := p.lines[ev.Archive]
		if !exists {
			// New file - append a new line.
			idx = p.total
			p.lines[ev.Archive] = idx
			p.total++
			fmt.Fprintf(p.w, "%s\n", line)
		} else {
			// Existing file - move cursor up, overwrite, move back down.
			up := p.total - idx
			fmt.Fprintf(p.w, "\033[%dA\r\033[K%s\033[%dB\r", up, line, up)
		}
	case "verifying":
		fmt.Fprintf(p.w, "Verifying %s...\n", ev.Archive)
	case "extracting":
		if ev.Archive != "" {
			if p.isTTY {
				pct := ""
				if ev.BytesTotal > 0 {
					pct = fmt.Sprintf(" %.0f%%", float64(ev.BytesDone)/float64(ev.BytesTotal)*100)
				}
				fmt.Fprintf(p.w, "\r\033[KExtracting %s%s", ev.Archive, pct)
			}
			// Non-TTY: skip per-file extraction updates entirely.
		} else {
			fmt.Fprintf(p.w, "Extracting archives...\n")
		}
	case "patching":
		if p.isTTY {
			fmt.Fprintf(p.w, "\r\033[K")
		}
		fmt.Fprintf(p.w, "Patching qt.conf...\n")
	case "registering":
		fmt.Fprintf(p.w, "Registering installation...\n")
	case "done":
		if p.isTTY {
			fmt.Fprintf(p.w, "\r\033[K")
		}
		fmt.Fprintf(p.w, "Done.\n")
	}
}

func formatDownloadLine(ev install.ProgressEvent) string {
	pct := ""
	if ev.BytesTotal > 0 {
		pct = fmt.Sprintf(" %3.0f%%", float64(ev.BytesDone)/float64(ev.BytesTotal)*100)
	}
	speed := ""
	if ev.Speed > 0 {
		speed = fmt.Sprintf(" @ %s/s", formatSize(int64(ev.Speed)))
	}
	return fmt.Sprintf("  %s%s%s", ev.Archive, pct, speed)
}
