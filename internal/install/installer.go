package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
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

func randSuffix() string {
	return fmt.Sprintf("%08x", rand.Uint32())
}

// ProgressEvent is emitted by the installer pipeline.
type ProgressEvent struct {
	Phase        string  // "resolving", "downloading", "extracting", "patching", "registering"
	Archive      string  // current archive name
	Percent      float64 // 0–100 overall progress
	BytesDone    int64
	BytesTotal   int64
	Speed        float64 // bytes/sec (download phase only)
	ArchiveIndex int     // 1-based index of the archive being processed (0 = unknown)
	ArchiveTotal int     // total number of archives in this install (0 = unknown)
}

// Options configures a Qt SDK installation.
type Options struct {
	Version     string
	Arch        string
	Modules     []string
	Docs        []string // "*" = all selected
	Examples    []string
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

// Installer orchestrates the download → verify → extract → patch → register pipeline.
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
		if reg, err := inst.registry.Load(); err == nil {
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

	if existingQt != nil {
		// Delta: only resolve what is not already installed.
		resolveOpts.SkipEssentials = true
		resolveOpts.Modules = diffSlices(normalizeModuleNames(opts.Modules), existingQt.Modules)
		resolveOpts.Docs = diffDocs(opts.Docs, existingQt.Extras.Docs)
		resolveOpts.Examples = diffDocs(opts.Examples, existingQt.Extras.Examples)
		resolveOpts.Sources = opts.Sources && !existingQt.Extras.Sources
		resolveOpts.DebugInfo = opts.DebugInfo && !existingQt.Extras.DebugInfo

		if len(resolveOpts.Modules) == 0 &&
			len(resolveOpts.Docs) == 0 &&
			len(resolveOpts.Examples) == 0 &&
			!resolveOpts.Sources &&
			!resolveOpts.DebugInfo {
			return ErrUpToDate
		}
	} else {
		resolveOpts.Modules = normalizeModuleNames(opts.Modules)
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

	// Separate temp dir only for extraction — always starts fresh.
	extractTmp := storage.TempDir(installDir) + "-" + randSuffix()
	defer os.RemoveAll(extractTmp)

	if err := os.MkdirAll(extractTmp, 0o755); err != nil {
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
		concurrency = 4
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 300
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
			if err := VerifyFile(path, refs[i].SHA1); err != nil {
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
	if err := ExtractAll(localPaths, extractDir, exCh); err != nil {
		close(exCh)
		return fmt.Errorf("extracting: %w", err)
	}
	close(exCh)

	// Move extracted content to final install dir.
	if err := os.MkdirAll(filepath.Dir(installDir), 0o755); err != nil {
		return err
	}
	// If installDir already exists, merge (for adding modules later).
	if _, err := os.Stat(installDir); os.IsNotExist(err) {
		if err := os.Rename(extractDir, installDir); err != nil {
			// Rename across drives may fail; fall back to copy.
			if err2 := copyDir(extractDir, installDir); err2 != nil {
				return fmt.Errorf("moving to install dir: %w", err2)
			}
		}
	} else {
		// Merge: copy new files over existing.
		if err := copyDir(extractDir, installDir); err != nil {
			return fmt.Errorf("merging into install dir: %w", err)
		}
	}

	// Patch qt.conf.
	sendProgress(progressCh, ProgressEvent{Phase: "patching"})
	if err := PatchQtConf(installDir); err != nil {
		// Non-fatal; warn but continue.
		fmt.Fprintf(os.Stderr, "warning: patching qt.conf failed: %v\n", err)
	}

	// Calculate installed size.
	var totalSize int64
	for _, a := range archives {
		totalSize += a.Ref.Size
	}

	// Register — merge with existing entry when doing a delta install.
	sendProgress(progressCh, ProgressEvent{Phase: "registering"})
	entry := storage.InstalledQt{
		Version:    opts.Version,
		Arch:       opts.Arch,
		InstallDir: installDir,
		InstalledAt: time.Now(),
	}
	if existingQt != nil {
		entry.InstalledAt = existingQt.InstalledAt
		entry.Modules = mergeSlices(existingQt.Modules, opts.Modules)
		entry.Extras = storage.InstalledExtras{
			Docs:      mergeDocs(existingQt.Extras.Docs, opts.Docs),
			Examples:  mergeDocs(existingQt.Extras.Examples, opts.Examples),
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
	if err := inst.registry.AddQt(entry); err != nil {
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

	if err := os.MkdirAll(extractTmp, 0o755); err != nil {
		return err
	}

	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 300
	}

	sendProgress(progressCh, ProgressEvent{Phase: "downloading"})
	downloader := NewDownloader(concurrency, timeout, dlDir)
	localPaths, err := downloader.DownloadAll(ctx, refs, nil)
	if err != nil {
		return err
	}

	sendProgress(progressCh, ProgressEvent{Phase: "extracting"})
	if err := ExtractAll(localPaths, opts.InstallDir, nil); err != nil {
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

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = copyBuf(out, in)
	return err
}

func copyBuf(dst *os.File, src *os.File) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, err := src.Read(buf)
		if n > 0 {
			written, werr := dst.Write(buf[:n])
			total += int64(written)
			if werr != nil {
				return total, werr
			}
		}
		if err != nil {
			if err == io.EOF {
				return total, nil
			}
			return total, err
		}
	}
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

// diffDocs computes the delta for docs/examples lists, handling "*" (all) semantics.
// If installed already contains "*", everything is already installed → return nil.
// If requested is "*" and installed has some entries, still need to install remaining → return "*".
func diffDocs(requested, installed []string) []string {
	if len(requested) == 0 {
		return nil
	}
	// If already installed everything, nothing to do.
	if len(installed) == 1 && installed[0] == "*" {
		return nil
	}
	// If requesting everything, pass through.
	if len(requested) == 1 && requested[0] == "*" {
		return requested
	}
	return diffSlices(requested, installed)
}

// mergeSlices returns the union of existing and new without duplicates,
// preserving order (existing first, then any new additions).
func mergeSlices(existing, additions []string) []string {
	set := make(map[string]struct{}, len(existing))
	result := make([]string, len(existing))
	copy(result, existing)
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

// mergeDocs merges existing and new doc/example lists, handling "*" semantics.
func mergeDocs(existing, additions []string) []string {
	// If either side is "*" (all), the result is "*".
	for _, s := range existing {
		if s == "*" {
			return existing
		}
	}
	for _, s := range additions {
		if s == "*" {
			return additions
		}
	}
	return mergeSlices(existing, additions)
}
