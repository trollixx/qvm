package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/trollixx/qvm/internal/config"
	"github.com/trollixx/qvm/internal/install"
	"github.com/trollixx/qvm/internal/platform"
	"github.com/trollixx/qvm/internal/repository"
)

func (a *app) newInstallCommand() *cli.Command {
	return &cli.Command{
		Name:            "install",
		Aliases:         []string{"i"},
		Usage:           "Install a Qt version",
		ArgsUsage:       "<version>",
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
	if arg == "" || arg == "qt" {
		return newHintError("specify a version", "Example: qvm install 6.8.3")
	}

	return a.runInstallQt(ctx, cmd, arg)
}

// resolveInstallArch determines the arch to install. It honors an explicit --arch,
// otherwise auto-detects (desktop only). For non-desktop targets without an
// explicit --arch it returns an error pointing the user at list-remote.
func resolveInstallArch(cmd *cli.Command, version, host, target string) (string, error) {
	arch := cmd.String("arch")
	if arch != "" {
		return arch, nil
	}
	if target != "" && target != repository.TargetDesktop {
		return "", newHintError(
			fmt.Sprintf("--arch is required when --target=%s", target),
			fmt.Sprintf("Run 'qvm list-remote %s' to see available arches for this target.", version),
		)
	}
	if host != "" {
		arch = platform.DefaultArchForHost(host, version)
	}
	if arch == "" {
		arch = platform.Current().DefaultArch(version)
	}
	return arch, nil
}

// buildInstallOptions converts CLI flags into an install.Options.
func buildInstallOptions(cmd *cli.Command, cfg *config.Config, version, arch string) install.Options {
	installRoot := cmd.String("dir")
	if installRoot == "" {
		installRoot = cfg.Install.Dir
	}
	return install.Options{
		Version:     version,
		Arch:        arch,
		Modules:     cmd.StringSlice("modules"),
		Docs:        cmd.Bool("docs"),
		Examples:    cmd.Bool("examples"),
		Sources:     cmd.Bool("sources"),
		DebugInfo:   cmd.Bool("debug-symbols"),
		InstallRoot: installRoot,
		Concurrency: cfg.Download.Concurrency,
		Timeout:     cfg.Download.TimeoutSeconds,
		Force:       cmd.Bool("force"),
		DryRun:      cmd.Bool("dry-run"),
	}
}

func (a *app) runInstallQt(ctx context.Context, cmd *cli.Command, version string) error {
	host := cmd.String("host")
	target := cmd.String("target")

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	installer, err := buildDeps(cfg, host, target)
	if err != nil {
		return fmt.Errorf("initializing dependencies: %w", err)
	}

	arch, err := resolveInstallArch(cmd, version, host, target)
	if err != nil {
		return err
	}

	opts := buildInstallOptions(cmd, cfg, version, arch)
	dryRun := opts.DryRun
	modules := opts.Modules

	quiet := cmd.Bool("quiet")
	progressCh := make(chan install.ProgressEvent, 256)
	doneCh := make(chan struct{})
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
		// "Already up to date" is the dry-run answer, not an error.
		if errors.Is(installErr, install.ErrUpToDate) {
			fmt.Fprintf(a.streams.Out,
				"Dry run - Qt %s (%s) is already installed; nothing to download.\n", version, arch)
			return nil
		}
		return installErr
	}

	if errors.Is(installErr, install.ErrUpToDate) {
		fmt.Fprintf(a.streams.Out, "Qt %s (%s) is already installed. Use --force to reinstall.\n", version, arch)
		return nil
	}
	if installErr != nil {
		return withHint(fmt.Errorf("installation failed: %w", installErr), "Run 'qvm doctor' to diagnose issues.")
	}

	fmt.Fprintf(a.streams.Out, "\nQt %s (%s) installed successfully.\n", version, arch)

	if !quiet && len(modules) == 0 {
		fmt.Fprintf(a.streams.Out, "\nTip: Add-on modules are available (charts, webengine, multimedia, ...).\n")
		fmt.Fprintf(a.streams.Out, "  Run 'qvm list %s' to see them, or reinstall with:\n", version)
		fmt.Fprintf(a.streams.Out, "  qvm install %s -m <module1>,<module2>\n", version)
	}

	return nil
}

func (a *app) printDryRun(ev install.ProgressEvent) {
	switch {
	case ev.Archive == "" && ev.ArchiveTotal > 0:
		// Header.
		total := ""
		if ev.BytesTotal > 0 {
			total = " (" + formatSize(ev.BytesTotal) + " total)"
		}
		fmt.Fprintf(a.streams.Out, "Dry run - would download %d archive(s)%s:\n", ev.ArchiveTotal, total)
	case ev.Archive == "":
		// Footer.
		if ev.BytesTotal > 0 {
			fmt.Fprintf(a.streams.Out, "\nTotal: %s\n", formatSize(ev.BytesTotal))
		}
	default:
		size := ""
		if ev.BytesTotal > 0 {
			size = "  " + formatSize(ev.BytesTotal)
		}
		fmt.Fprintf(a.streams.Out, "  %s%s\n", ev.Archive, size)
	}
}

// ProgressPrinter renders install progress events to stderr.
// isTTY controls whether cursor-movement escape sequences are used.
type ProgressPrinter struct {
	isTTY bool
	w     io.Writer
	lines map[string]int // archive name -> line index (0-based from first download line)
	total int            // number of lines printed so far
}

func (p *ProgressPrinter) print(ev install.ProgressEvent) {
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
	case "warning":
		fmt.Fprintf(p.w, "warning: %s\n", ev.Warning)
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
