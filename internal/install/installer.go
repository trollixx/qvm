package install

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/trollixx/qvm/internal/repository"
	"github.com/trollixx/qvm/internal/storage"
)

// ErrUpToDate is returned by Install when the requested content is already installed
// and there is nothing new to download.
var ErrUpToDate = errors.New("already up to date")

const (
	defaultConcurrency    = 4
	defaultTimeoutSeconds = 300
)

func randSuffix() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fallback: use current time if OS entropy source is unavailable.
		t := time.Now().UnixNano()
		b[0], b[1], b[2], b[3] = byte(t), byte(t>>8), byte(t>>16), byte(t>>24)
	}
	return hex.EncodeToString(b[:])
}

// ProgressEvent is emitted by the installer pipeline.
type ProgressEvent struct {
	Phase        string  // "resolving", "downloading", "extracting", "patching", "registering"
	Archive      string  // current archive name
	Percent      float64 // 0-100 overall progress
	BytesDone    int64
	BytesTotal   int64
	Speed        float64 // bytes/sec (download phase only)
	ArchiveIndex int     // 1-based index of the archive being processed (0 = unknown)
	ArchiveTotal int     // total number of archives in this install (0 = unknown)
	Warning      string  // non-empty message when Phase == "warning"
}

// Options configures a Qt SDK installation.
type Options struct {
	Version     string
	Arch        string
	Modules     []string
	Docs        bool
	Examples    bool
	Sources     bool
	DebugInfo   bool
	InstallRoot string // e.g. C:\Qt
	Concurrency int
	Timeout     int  // seconds
	Force       bool // re-install even if already installed
	DryRun      bool // resolve and report archives without downloading
}

// ToolOptions configures a tool installation.
type ToolOptions struct {
	Name        string
	Version     string
	InstallDir  string
	Concurrency int
	Timeout     int
}

// Installer orchestrates the download -> verify -> extract -> patch -> register pipeline.
type Installer struct {
	resolver *repository.Resolver
	registry *storage.RegistryManager
}

// NewInstaller creates an Installer.
func NewInstaller(resolver *repository.Resolver, registry *storage.RegistryManager) *Installer {
	return &Installer{resolver: resolver, registry: registry}
}

// Install performs a full Qt SDK installation.
// The caller owns progressCh and is responsible for closing it after Install returns.
// Install only sends on the channel; it never closes it.
func (inst *Installer) Install(ctx context.Context, opts Options, progressCh chan<- ProgressEvent) error {
	// Check for an existing installation and compute the delta unless --force.
	var existingQt *storage.InstalledQt
	if !opts.Force {
		reg, err := inst.registry.Load()
		if err == nil {
			for i := range reg.Qt {
				if reg.Qt[i].Version == opts.Version && reg.Qt[i].Arch == opts.Arch {
					existingQt = &reg.Qt[i]
					break
				}
			}
		}
	}

	resolveOpts := repository.ResolveOptions{
		Version: opts.Version,
		Arch:    opts.Arch,
	}

	normalizedModules := normalizeModuleNames(opts.Modules)

	if existingQt != nil {
		// Delta: only resolve what is not already installed.
		resolveOpts.SkipEssentials = true
		resolveOpts.Modules = diffSlices(normalizedModules, existingQt.Modules)
		resolveOpts.AllModules = mergeSlices(existingQt.Modules, normalizedModules)
		resolveOpts.Docs = opts.Docs && !existingQt.Extras.Docs
		resolveOpts.Examples = opts.Examples && !existingQt.Extras.Examples
		resolveOpts.Sources = opts.Sources && !existingQt.Extras.Sources
		resolveOpts.DebugInfo = opts.DebugInfo && !existingQt.Extras.DebugInfo

		if len(resolveOpts.Modules) == 0 &&
			!resolveOpts.Docs &&
			!resolveOpts.Examples &&
			!resolveOpts.Sources &&
			!resolveOpts.DebugInfo {
			return ErrUpToDate
		}
	} else {
		resolveOpts.Modules = normalizedModules
		resolveOpts.Docs = opts.Docs
		resolveOpts.Examples = opts.Examples
		resolveOpts.Sources = opts.Sources
		resolveOpts.DebugInfo = opts.DebugInfo
	}

	// Resolve archives.
	sendProgress(progressCh, ProgressEvent{Phase: "resolving"})
	archives, err := inst.resolver.Resolve(ctx, resolveOpts)
	if err != nil {
		return fmt.Errorf("resolving archives: %w", err)
	}

	// Dry run: report what would be downloaded and stop.
	if opts.DryRun {
		sendProgress(progressCh, ProgressEvent{Phase: "dryrun", ArchiveTotal: len(archives)})
		for _, a := range archives {
			sendProgress(progressCh, ProgressEvent{
				Phase:      "dryrun",
				Archive:    a.Ref.Filename,
				BytesTotal: a.Ref.Size,
			})
		}
		return nil
	}

	installDir := storage.InstallDir(opts.InstallRoot, opts.Version, opts.Arch)

	// Stable cache dir: survives interruption so downloads can be resumed.
	dlDir, err := DownloadCacheDir()
	if err != nil {
		return fmt.Errorf("download cache dir: %w", err)
	}

	// Separate temp dir only for extraction - always starts fresh.
	extractTmp := storage.TempDir(installDir) + "-" + randSuffix()
	defer os.RemoveAll(extractTmp)

	err = os.MkdirAll(extractTmp, 0o755) //nolint:gosec // 0755 for Qt SDK
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}

	// Extract ArchiveRef slice.
	refs := make([]repository.ArchiveRef, len(archives))
	for i, a := range archives {
		refs[i] = a.Ref
	}

	// Download.
	archiveTotal := len(refs)
	sendProgress(progressCh, ProgressEvent{Phase: "downloading", ArchiveTotal: archiveTotal})
	dlCh := make(chan DownloadEvent, 256)
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = defaultConcurrency
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeoutSeconds
	}
	downloader := NewDownloader(concurrency, timeout, dlDir)

	// Collect download events and forward to progress channel.
	go func() {
		completed := 0
		for ev := range dlCh {
			if ev.Err != nil {
				return
			}
			if ev.Done {
				completed++
			}
			sendProgress(progressCh, ProgressEvent{
				Phase:        "downloading",
				Archive:      ev.Filename,
				BytesDone:    ev.BytesDone,
				BytesTotal:   ev.BytesTotal,
				Speed:        ev.Speed,
				ArchiveIndex: completed,
				ArchiveTotal: archiveTotal,
			})
		}
	}()

	localPaths, err := downloader.DownloadAll(ctx, refs, dlCh)
	close(dlCh)
	if err != nil {
		return fmt.Errorf("downloading: %w", err)
	}

	// Verify checksums.
	for i, path := range localPaths {
		if refs[i].SHA1 != "" {
			sendProgress(progressCh, ProgressEvent{Phase: "verifying", Archive: refs[i].Filename})
			err = VerifyFile(path, refs[i].SHA1)
			if err != nil {
				return err
			}
		}
	}

	// Extract into temp extraction dir.
	extractDir := filepath.Join(extractTmp, "extracted")
	sendProgress(progressCh, ProgressEvent{Phase: "extracting"})
	exCh := make(chan ExtractionEvent, 256)
	go func() {
		for ev := range exCh {
			pct := float64(0)
			if ev.BytesTotal > 0 {
				pct = float64(ev.BytesDone) / float64(ev.BytesTotal) * 100
			}
			sendProgress(progressCh, ProgressEvent{
				Phase:      "extracting",
				Archive:    ev.Archive,
				BytesDone:  ev.BytesDone,
				BytesTotal: ev.BytesTotal,
				Percent:    pct,
			})
		}
	}()
	err = ExtractAll(localPaths, extractDir, exCh)
	if err != nil {
		close(exCh)
		return fmt.Errorf("extracting: %w", err)
	}
	close(exCh)

	// Move extracted content to final install dir.
	err = os.MkdirAll(filepath.Dir(installDir), 0o755) //nolint:gosec // 0755 for Qt SDK
	if err != nil {
		return err
	}
	err = installFiles(extractDir, installDir)
	if err != nil {
		return fmt.Errorf("installing files: %w", err)
	}

	// Patch qt.conf.
	sendProgress(progressCh, ProgressEvent{Phase: "patching"})
	patchErr := PatchQtConf(installDir)
	if patchErr != nil {
		// Non-fatal; surface as a warning through the progress channel.
		sendProgress(progressCh, ProgressEvent{Phase: "warning", Warning: "patching qt.conf failed: " + patchErr.Error()})
	}

	// Calculate installed size.
	var totalSize int64
	for _, a := range archives {
		totalSize += a.Ref.Size
	}

	// Register - merge with existing entry when doing a delta install.
	sendProgress(progressCh, ProgressEvent{Phase: "registering"})
	entry := storage.InstalledQt{
		Version:     opts.Version,
		Arch:        opts.Arch,
		InstallDir:  installDir,
		InstalledAt: time.Now(),
	}
	if existingQt != nil {
		entry.InstalledAt = existingQt.InstalledAt
		entry.Modules = mergeSlices(existingQt.Modules, opts.Modules)
		entry.Extras = storage.InstalledExtras{
			Docs:      existingQt.Extras.Docs || opts.Docs,
			Examples:  existingQt.Extras.Examples || opts.Examples,
			Sources:   existingQt.Extras.Sources || opts.Sources,
			DebugInfo: existingQt.Extras.DebugInfo || opts.DebugInfo,
		}
		entry.SizeBytes = existingQt.SizeBytes + totalSize
	} else {
		entry.Modules = opts.Modules
		entry.Extras = storage.InstalledExtras{
			Docs:      opts.Docs,
			Examples:  opts.Examples,
			Sources:   opts.Sources,
			DebugInfo: opts.DebugInfo,
		}
		entry.SizeBytes = totalSize
	}
	err = inst.registry.AddQt(entry)
	if err != nil {
		return fmt.Errorf("registering installation: %w", err)
	}

	sendProgress(progressCh, ProgressEvent{Phase: "done", Percent: 100})
	return nil
}

// InstallTool performs a tool installation.
func (inst *Installer) InstallTool(ctx context.Context, opts ToolOptions, progressCh chan<- ProgressEvent) error {
	sendProgress(progressCh, ProgressEvent{Phase: "resolving"})

	archives, err := inst.resolver.ResolveTool(ctx, opts.Name, opts.Version)
	if err != nil {
		return err
	}

	refs := make([]repository.ArchiveRef, len(archives))
	for i, a := range archives {
		refs[i] = a.Ref
	}

	dlDir, err := DownloadCacheDir()
	if err != nil {
		return fmt.Errorf("download cache dir: %w", err)
	}

	extractTmp := opts.InstallDir + ".qvm-tmp-" + randSuffix()
	defer os.RemoveAll(extractTmp)

	err = os.MkdirAll(extractTmp, 0o755) //nolint:gosec // 0755 for Qt SDK
	if err != nil {
		return err
	}

	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = defaultConcurrency
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeoutSeconds
	}

	sendProgress(progressCh, ProgressEvent{Phase: "downloading"})
	downloader := NewDownloader(concurrency, timeout, dlDir)
	localPaths, err := downloader.DownloadAll(ctx, refs, nil)
	if err != nil {
		return err
	}

	sendProgress(progressCh, ProgressEvent{Phase: "extracting"})
	err = ExtractAll(localPaths, opts.InstallDir, nil)
	if err != nil {
		return err
	}

	sendProgress(progressCh, ProgressEvent{Phase: "registering"})
	entry := storage.InstalledTool{
		Name:        opts.Name,
		Version:     opts.Version,
		InstallDir:  opts.InstallDir,
		InstalledAt: time.Now(),
	}
	return inst.registry.AddTool(entry)
}

func sendProgress(ch chan<- ProgressEvent, ev ProgressEvent) {
	if ch == nil {
		return
	}
	select {
	case ch <- ev:
	default:
	}
}

// installFiles moves src to dst, falling back to a copy if rename fails (e.g. cross-device).
// If dst already exists, files from src are merged into it instead.
func installFiles(src, dst string) error {
	_, err := os.Stat(dst)
	switch {
	case err == nil:
		// dst exists - merge src into it.
		return copyDir(src, dst)
	case !os.IsNotExist(err):
		// Unexpected error (e.g. permission denied) - propagate instead of silently attempting a copy.
		return fmt.Errorf("checking install dir: %w", err)
	}
	// dst does not exist - try a fast rename first.
	renameErr := os.Rename(src, dst)
	if renameErr == nil {
		return nil
	}
	return copyDir(src, dst)
}

// copyDir copies src directory tree to dst.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	err = os.MkdirAll(filepath.Dir(dst), 0o755) //nolint:gosec // 0755 for Qt SDK
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// normalizeModuleNames ensures each module name has the "qt" prefix,
// matching the canonical form used in Qt's metadata and the registry.
func normalizeModuleNames(modules []string) []string {
	out := make([]string, len(modules))
	for i, m := range modules {
		if !strings.HasPrefix(m, "qt") {
			m = "qt" + m
		}
		out[i] = m
	}
	return out
}

// diffSlices returns elements in requested that are not in installed.
func diffSlices(requested, installed []string) []string {
	set := make(map[string]struct{}, len(installed))
	for _, s := range installed {
		set[s] = struct{}{}
	}
	var diff []string
	for _, s := range requested {
		if _, found := set[s]; !found {
			diff = append(diff, s)
		}
	}
	return diff
}

// mergeSlices returns the union of existing and new without duplicates,
// preserving order (existing first, then any new additions).
func mergeSlices(existing, additions []string) []string {
	set := make(map[string]struct{}, len(existing))
	result := make([]string, 0, len(existing)+len(additions))
	result = append(result, existing...)
	for _, s := range existing {
		set[s] = struct{}{}
	}
	for _, s := range additions {
		if _, found := set[s]; !found {
			result = append(result, s)
		}
	}
	return result
}
