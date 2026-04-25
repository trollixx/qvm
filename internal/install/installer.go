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
		//nolint:gosec // intentional low-byte truncation for entropy fallback
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

// Installer orchestrates the download -> verify -> extract -> patch -> register pipeline.
type Installer struct {
	resolver *repository.Resolver
	registry *storage.RegistryManager
}

// NewInstaller creates an Installer.
func NewInstaller(resolver *repository.Resolver, registry *storage.RegistryManager) *Installer {
	return &Installer{resolver: resolver, registry: registry}
}

// findExisting returns the registered install matching opts, or nil if not found or --force.
func (inst *Installer) findExisting(opts Options) *storage.InstalledQt {
	if opts.Force {
		return nil
	}
	reg, err := inst.registry.Load()
	if err != nil {
		return nil
	}
	for i := range reg.Qt {
		if reg.Qt[i].Version == opts.Version && reg.Qt[i].Arch == opts.Arch {
			return &reg.Qt[i]
		}
	}
	return nil
}

// buildResolveOptions computes resolver options for a fresh install or a delta over existingQt.
// The bool return is true when the delta is empty (nothing to do).
func buildResolveOptions(
	opts Options, existingQt *storage.InstalledQt, modules []string,
) (repository.ResolveOptions, bool) {
	ro := repository.ResolveOptions{Version: opts.Version, Arch: opts.Arch}
	if existingQt == nil {
		ro.Modules = modules
		ro.Docs = opts.Docs
		ro.Examples = opts.Examples
		ro.Sources = opts.Sources
		ro.DebugInfo = opts.DebugInfo
		return ro, false
	}
	ro.SkipEssentials = true
	ro.Modules = diffSlices(modules, existingQt.Modules)
	ro.AllModules = mergeSlices(existingQt.Modules, modules)
	ro.Docs = opts.Docs && !existingQt.Extras.Docs
	ro.Examples = opts.Examples && !existingQt.Extras.Examples
	ro.Sources = opts.Sources && !existingQt.Extras.Sources
	ro.DebugInfo = opts.DebugInfo && !existingQt.Extras.DebugInfo
	upToDate := len(ro.Modules) == 0 && !ro.Docs && !ro.Examples && !ro.Sources && !ro.DebugInfo
	return ro, upToDate
}

// runDryRun emits dryrun progress events for each resolved archive without downloading.
func runDryRun(archives []repository.ResolvedArchive, progressCh chan<- ProgressEvent) {
	sendProgress(progressCh, ProgressEvent{Phase: "dryrun", ArchiveTotal: len(archives)})
	for _, a := range archives {
		sendProgress(progressCh, ProgressEvent{
			Phase:      "dryrun",
			Archive:    a.Ref.Filename,
			BytesTotal: a.Ref.Size,
		})
	}
}

// runDownload executes the download phase: emits progress, starts the forwarder, and calls DownloadAll.
func runDownload(
	ctx context.Context, opts Options, refs []repository.ArchiveRef, dlDir string, progressCh chan<- ProgressEvent,
) ([]string, error) {
	archiveTotal := len(refs)
	sendProgress(progressCh, ProgressEvent{Phase: "downloading", ArchiveTotal: archiveTotal})
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = defaultConcurrency
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeoutSeconds
	}
	downloader := NewDownloader(concurrency, timeout, dlDir)
	dlCh := make(chan DownloadEvent, 256)
	go forwardDownloadProgress(dlCh, progressCh, archiveTotal)
	localPaths, err := downloader.DownloadAll(ctx, refs, dlCh)
	close(dlCh)
	if err != nil {
		return nil, fmt.Errorf("downloading: %w", err)
	}
	return localPaths, nil
}

// runExtract executes the extraction phase: starts the forwarder and calls ExtractAll.
func runExtract(localPaths []string, extractDir string, progressCh chan<- ProgressEvent) error {
	sendProgress(progressCh, ProgressEvent{Phase: "extracting"})
	exCh := make(chan ExtractionEvent, 256)
	go forwardExtractionProgress(exCh, progressCh)
	err := ExtractAll(localPaths, extractDir, exCh)
	close(exCh)
	if err != nil {
		return fmt.Errorf("extracting: %w", err)
	}
	return nil
}

// forwardDownloadProgress consumes dlCh and re-emits events on progressCh with aggregated counts.
// Returns when dlCh is closed.
func forwardDownloadProgress(dlCh <-chan DownloadEvent, progressCh chan<- ProgressEvent, archiveTotal int) {
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
}

// forwardExtractionProgress consumes exCh and re-emits events on progressCh with computed percent.
// Returns when exCh is closed.
func forwardExtractionProgress(exCh <-chan ExtractionEvent, progressCh chan<- ProgressEvent) {
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
}

// verifyDownloads checks the SHA-1 of each downloaded archive whose ref has a non-empty SHA1.
func verifyDownloads(paths []string, refs []repository.ArchiveRef, progressCh chan<- ProgressEvent) error {
	for i, path := range paths {
		if refs[i].SHA1 == "" {
			continue
		}
		sendProgress(progressCh, ProgressEvent{Phase: "verifying", Archive: refs[i].Filename})
		err := VerifyFile(path, refs[i].SHA1)
		if err != nil {
			return err
		}
	}
	return nil
}

// buildRegistryEntry assembles the storage entry for a fresh install or a delta merge.
// Module names are stored in their canonical (qt-prefixed) form so that subsequent
// installs can correctly diff against them regardless of how the user spelled them.
func buildRegistryEntry(
	opts Options, existingQt *storage.InstalledQt, installDir string, totalSize int64,
) storage.InstalledQt {
	entry := storage.InstalledQt{
		Version:     opts.Version,
		Arch:        opts.Arch,
		InstallDir:  installDir,
		InstalledAt: time.Now(),
	}
	canonicalModules := normalizeModuleNames(opts.Modules)
	if existingQt == nil {
		entry.Modules = canonicalModules
		entry.Extras = storage.InstalledExtras{
			Docs:      opts.Docs,
			Examples:  opts.Examples,
			Sources:   opts.Sources,
			DebugInfo: opts.DebugInfo,
		}
		entry.SizeBytes = totalSize
		return entry
	}
	entry.InstalledAt = existingQt.InstalledAt
	entry.Modules = mergeSlices(existingQt.Modules, canonicalModules)
	entry.Extras = storage.InstalledExtras{
		Docs:      existingQt.Extras.Docs || opts.Docs,
		Examples:  existingQt.Extras.Examples || opts.Examples,
		Sources:   existingQt.Extras.Sources || opts.Sources,
		DebugInfo: existingQt.Extras.DebugInfo || opts.DebugInfo,
	}
	entry.SizeBytes = existingQt.SizeBytes + totalSize
	return entry
}

// Install performs a full Qt SDK installation.
// The caller owns progressCh and is responsible for closing it after Install returns.
// Install only sends on the channel; it never closes it.
//
// A per-install-directory lock is held for the entire pipeline so that two
// concurrent qvm processes targeting the same version+arch serialize their
// download/extract/register steps and never clobber each other's files.
// Parallel installs to *different* version+arch combinations are unaffected.
//
//nolint:funlen // orchestration pipeline; phases are each extracted to their own helper
func (inst *Installer) Install(ctx context.Context, opts Options, progressCh chan<- ProgressEvent) error {
	installDir := storage.InstallDir(opts.InstallRoot, opts.Version, opts.Arch)

	// Dry-run is read-only and side-effect-free; no lock needed.
	if opts.DryRun {
		resolveOpts, upToDate := buildResolveOptions(opts, inst.findExisting(opts), normalizeModuleNames(opts.Modules))
		if upToDate {
			return ErrUpToDate
		}
		sendProgress(progressCh, ProgressEvent{Phase: "resolving"})
		archives, err := inst.resolver.Resolve(ctx, resolveOpts)
		if err != nil {
			return fmt.Errorf("resolving archives: %w", err)
		}
		runDryRun(archives, progressCh)
		return nil
	}

	// Hold a process-level lock on the install dir for the whole pipeline.
	unlock, err := storage.LockFile(installDir)
	if err != nil {
		return fmt.Errorf("acquiring install lock: %w", err)
	}
	defer unlock()

	// Compute existing state and diff *after* acquiring the lock so a parallel
	// install that completed while we were waiting is reflected.
	existingQt := inst.findExisting(opts)
	resolveOpts, upToDate := buildResolveOptions(opts, existingQt, normalizeModuleNames(opts.Modules))
	if upToDate {
		return ErrUpToDate
	}

	// Resolve archives.
	sendProgress(progressCh, ProgressEvent{Phase: "resolving"})
	archives, err := inst.resolver.Resolve(ctx, resolveOpts)
	if err != nil {
		return fmt.Errorf("resolving archives: %w", err)
	}

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

	localPaths, err := runDownload(ctx, opts, refs, dlDir, progressCh)
	if err != nil {
		return err
	}

	err = verifyDownloads(localPaths, refs, progressCh)
	if err != nil {
		return err
	}

	extractDir := filepath.Join(extractTmp, "extracted")
	err = runExtract(localPaths, extractDir, progressCh)
	if err != nil {
		return err
	}

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
		sendProgress(
			progressCh,
			ProgressEvent{Phase: "warning", Warning: "patching qt.conf failed: " + patchErr.Error()},
		)
	}

	// Calculate installed size.
	var totalSize int64
	for _, a := range archives {
		totalSize += a.Ref.Size
	}

	sendProgress(progressCh, ProgressEvent{Phase: "registering"})
	err = inst.registry.AddQt(buildRegistryEntry(opts, existingQt, installDir, totalSize))
	if err != nil {
		return fmt.Errorf("registering installation: %w", err)
	}

	sendProgress(progressCh, ProgressEvent{Phase: "done", Percent: 100})
	return nil
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
