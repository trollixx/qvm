package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/trollixx/qvm/internal/install"
	"github.com/trollixx/qvm/internal/repository"
)

func (a *app) newCacheCommand() *cli.Command {
	return &cli.Command{
		Name:   "cache",
		Usage:  "Manage the download cache",
		Action: a.runCacheList,
		Commands: []*cli.Command{
			{
				Name:   "list",
				Usage:  "List cached download files",
				Action: a.runCacheList,
			},
			{
				Name:  "clean",
				Usage: "Remove cached download files",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "incomplete",
						Usage: "Remove only incomplete (.part) files, leave fully downloaded archives",
					},
					&cli.BoolFlag{
						Name:  "metadata",
						Usage: "Remove cached repository metadata (XML files)",
					},
					&cli.BoolFlag{
						Name:  "all",
						Usage: "Remove all download archives and metadata cache",
					},
					newYesFlag(),
				},
				Action: a.runCacheClean,
			},
		},
	}
}

func (a *app) runCacheList(_ context.Context, _ *cli.Command) error {
	dir, err := install.DownloadCacheDir()
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading cache dir: %w", err)
	}

	// Skip the sentinel .keep file used to create the directory.
	var files []os.DirEntry
	for _, e := range entries {
		if e.Name() == ".keep" {
			continue
		}
		files = append(files, e)
	}

	fmt.Fprintf(a.streams.Out, "Download cache: %s\n", dir)

	if len(files) == 0 {
		fmt.Fprintln(a.streams.Out, "  (empty)")
		return nil
	}

	var totalBytes int64
	for _, e := range files {
		info, infoErr := e.Info()
		if infoErr != nil {
			continue
		}
		name := e.Name()
		status := "complete"
		if rest, ok := strings.CutSuffix(name, ".part"); ok {
			name = rest
			status = "partial"
		}
		totalBytes += info.Size()
		fmt.Fprintf(a.streams.Out, "  %-60s  %8s  %s\n", name, formatSize(info.Size()), status)
	}

	fmt.Fprintf(a.streams.Out, "\nTotal: %s\n", formatSize(totalBytes))
	return nil
}

func (a *app) runCacheClean(_ context.Context, cmd *cli.Command) error {
	metadataFlag := cmd.Bool("metadata")
	incompleteFlag := cmd.Bool("incomplete")
	allFlag := cmd.Bool("all")
	autoYes := cmd.Bool("yes")

	// Derive what to clean.
	// --metadata alone suppresses download cleaning; any other combination includes it.
	cleanDownloads := !metadataFlag || incompleteFlag || allFlag
	incompleteOnly := incompleteFlag && !allFlag
	cleanMetadata := metadataFlag || allFlag

	type candidate struct {
		path string
		size int64
	}

	// Collect download cache candidates.
	var dlCandidates []candidate
	var dlSkipped int
	if cleanDownloads {
		dlDir, err := install.DownloadCacheDir()
		if err != nil {
			return err
		}
		entries, err := os.ReadDir(dlDir)
		if err != nil {
			return fmt.Errorf("reading cache dir: %w", err)
		}
		for _, e := range entries {
			if e.Name() == ".keep" {
				continue
			}
			isPart := strings.HasSuffix(e.Name(), ".part")
			if incompleteOnly && !isPart {
				dlSkipped++
				continue
			}
			info, _ := e.Info()
			var size int64
			if info != nil {
				size = info.Size()
			}
			dlCandidates = append(dlCandidates, candidate{
				path: filepath.Join(dlDir, e.Name()),
				size: size,
			})
		}
	}

	// Collect metadata cache candidates.
	var metaCandidates []candidate
	if cleanMetadata {
		cache, err := repository.NewCache()
		if err != nil {
			return fmt.Errorf("opening metadata cache: %w", err)
		}
		entries, err := os.ReadDir(cache.Dir())
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("reading metadata cache dir: %w", err)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			info, _ := e.Info()
			var size int64
			if info != nil {
				size = info.Size()
			}
			metaCandidates = append(metaCandidates, candidate{
				path: filepath.Join(cache.Dir(), e.Name()),
				size: size,
			})
		}
	}

	if len(dlCandidates) == 0 && dlSkipped == 0 && len(metaCandidates) == 0 {
		fmt.Fprintln(a.streams.Out, "Nothing to remove.")
		return nil
	}

	if len(dlCandidates) == 0 && dlSkipped > 0 && len(metaCandidates) == 0 {
		fmt.Fprintln(a.streams.Out, "No partial files to remove.")
		return nil
	}

	// Show confirmation.
	if !autoYes {
		if len(dlCandidates) > 0 {
			var dlBytes int64
			for _, c := range dlCandidates {
				dlBytes += c.size
			}
			label := "download archive(s)"
			if incompleteOnly {
				label = "partial download file(s)"
			}
			fmt.Fprintf(a.streams.Out, "  %d %s (%s)\n", len(dlCandidates), label, formatSize(dlBytes))
		}
		if len(metaCandidates) > 0 {
			var metaBytes int64
			for _, c := range metaCandidates {
				metaBytes += c.size
			}
			fmt.Fprintf(a.streams.Out, "  %d metadata file(s) (%s)\n", len(metaCandidates), formatSize(metaBytes))
		}
		if !a.confirm("Remove the above?") {
			fmt.Fprintln(a.streams.Out, "Aborted.")
			return nil
		}
	}

	// Remove download cache files.
	var dlRemoved int
	var dlFreed int64
	for _, c := range dlCandidates {
		err := os.Remove(c.path)
		if err != nil {
			fmt.Fprintf(a.streams.ErrOut, "warning: removing %s: %v\n", filepath.Base(c.path), err)
			continue
		}
		dlRemoved++
		dlFreed += c.size
	}

	// Remove metadata cache files.
	var metaRemoved int
	var metaFreed int64
	for _, c := range metaCandidates {
		err := os.Remove(c.path)
		if err != nil {
			fmt.Fprintf(a.streams.ErrOut, "warning: removing %s: %v\n", filepath.Base(c.path), err)
			continue
		}
		metaRemoved++
		metaFreed += c.size
	}

	// Summary.
	if dlRemoved > 0 {
		label := "archive(s)"
		if incompleteOnly {
			label = "partial file(s)"
		}
		fmt.Fprintf(a.streams.Out, "Removed %d download %s, freed %s.\n", dlRemoved, label, formatSize(dlFreed))
		if dlSkipped > 0 {
			fmt.Fprintf(
				a.streams.Out,
				"Kept %d complete archive(s) (use without --incomplete to remove all).\n",
				dlSkipped,
			)
		}
	}
	if metaRemoved > 0 {
		fmt.Fprintf(a.streams.Out, "Removed %d metadata file(s), freed %s.\n", metaRemoved, formatSize(metaFreed))
	}

	return nil
}
