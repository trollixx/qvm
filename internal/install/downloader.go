package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	retryablehttp "github.com/hashicorp/go-retryablehttp"

	"github.com/trollixx/qvm/internal/repository"
)

// DownloadEvent is emitted as archives are downloaded.
type DownloadEvent struct {
	Filename   string
	BytesDone  int64
	BytesTotal int64
	Speed      float64 // bytes per second
	Done       bool
	Err        error
}

// Downloader downloads archives in parallel with a bounded concurrency.
type Downloader struct {
	client      *retryablehttp.Client
	concurrency int
	destDir     string
}

// DownloadCacheDir returns the centralized cache directory used to store
// in-progress (.part) and completed archive files across runs.
// On Windows: %LOCALAPPDATA%\qvm\downloads.
// On Linux:   ~/.cache/qvm/downloads.
// On macOS:   ~/Library/Caches/qvm/downloads.
func DownloadCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("determining download cache dir: %w", err)
	}
	dir := filepath.Join(base, "qvm", "downloads")
	err = os.MkdirAll(dir, 0o750)
	if err != nil {
		return "", fmt.Errorf("creating download cache dir: %w", err)
	}
	return dir, nil
}

// NewDownloader creates a Downloader.
func NewDownloader(concurrency, timeoutSeconds int, destDir string) *Downloader {
	rc := retryablehttp.NewClient()
	rc.RetryMax = 3
	rc.RetryWaitMin = 1 * time.Second
	rc.RetryWaitMax = 8 * time.Second
	rc.Logger = nil
	rc.HTTPClient.Timeout = time.Duration(timeoutSeconds) * time.Second

	return &Downloader{client: rc, concurrency: concurrency, destDir: destDir}
}

// DownloadAll downloads all archives in parallel, emitting events on eventCh.
// Returns the local file paths in the same order as archives.
func (d *Downloader) DownloadAll(
	ctx context.Context,
	archives []repository.ArchiveRef,
	eventCh chan<- DownloadEvent,
) ([]string, error) {
	sem := make(chan struct{}, d.concurrency)
	paths := make([]string, len(archives))
	errs := make([]error, len(archives))
	var wg sync.WaitGroup

	for i, arch := range archives {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()

			path, err := d.downloadOne(ctx, arch, eventCh)
			paths[i] = path
			errs[i] = err
		})
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("downloading %s: %w", archives[i].Filename, err)
		}
	}
	return paths, nil
}

func (d *Downloader) downloadOne(
	ctx context.Context,
	arch repository.ArchiveRef,
	eventCh chan<- DownloadEvent,
) (string, error) {
	dest := filepath.Join(d.destDir, arch.Filename)
	part := dest + ".part"

	if checkCachedDownload(dest, arch, eventCh) {
		return dest, nil
	}
	rangeStart := resumeOffset(part, arch.Size)

	req, err := retryablehttp.NewRequest(http.MethodGet, arch.URL, nil)
	if err != nil {
		return "", err
	}
	req = req.WithContext(ctx)
	if rangeStart > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", rangeStart))
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		// .part file is already complete - just promote it.
		err = os.Rename(part, dest)
		if err != nil {
			return "", err
		}
		sendEvent(eventCh, DownloadEvent{Filename: arch.Filename, Done: true})
		return dest, nil
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	flag := os.O_CREATE | os.O_WRONLY
	if resp.StatusCode == http.StatusPartialContent {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
		rangeStart = 0
	}

	f, err := os.OpenFile(part, flag, 0o600)
	if err != nil {
		return "", err
	}

	total := resp.ContentLength + rangeStart
	done, copyErr := copyWithProgress(ctx, f, resp.Body, arch.Filename, rangeStart, total, eventCh)

	// Close before rename - required on Windows.
	cerr := f.Close()
	if copyErr == nil {
		copyErr = cerr
	}
	if copyErr != nil {
		return "", copyErr
	}

	err = os.Rename(part, dest)
	if err != nil {
		return "", err
	}

	sendEvent(eventCh, DownloadEvent{Filename: arch.Filename, Done: true, BytesDone: done, BytesTotal: total})
	return dest, nil
}

// checkCachedDownload returns true if dest already matches arch.SHA1. Corrupt cached files are removed.
func checkCachedDownload(dest string, arch repository.ArchiveRef, eventCh chan<- DownloadEvent) bool {
	if _, err := os.Stat(dest); err != nil {
		return false
	}
	if VerifyFile(dest, arch.SHA1) == nil {
		sendEvent(eventCh, DownloadEvent{Filename: arch.Filename, Done: true})
		return true
	}
	_ = os.Remove(dest)
	return false
}

// resumeOffset returns the size of an existing .part file if it is a valid resume target, else 0.
// A part file larger than expectedSize is treated as corrupt and removed.
func resumeOffset(part string, expectedSize int64) int64 {
	fi, err := os.Stat(part)
	if err != nil {
		return 0
	}
	partSize := fi.Size()
	if expectedSize > 0 && partSize > expectedSize {
		_ = os.Remove(part)
		return 0
	}
	return partSize
}

// copyWithProgress streams body into f in 32 KiB chunks, emitting progress events.
// rangeStart is the offset within the logical file where writing begins (for resume).
// Returns the total bytes written through this call plus rangeStart (i.e. the cumulative offset).
// Honors ctx cancellation between chunk reads.
func copyWithProgress(
	ctx context.Context, f io.Writer, body io.Reader, filename string,
	rangeStart, total int64, eventCh chan<- DownloadEvent,
) (int64, error) {
	done := rangeStart
	start := time.Now()
	buf := make([]byte, 32*1024)
	for {
		// Cooperative cancellation. The HTTP request context already cancels
		// the underlying connection on Done, but a fast non-blocking check
		// here means we abort before the next read syscall.
		select {
		case <-ctx.Done():
			return done, ctx.Err()
		default:
		}
		n, readErr := body.Read(buf)
		if n > 0 {
			_, werr := f.Write(buf[:n])
			if werr != nil {
				return done, werr
			}
			done += int64(n)
			elapsed := time.Since(start).Seconds()
			speed := float64(done-rangeStart) / elapsed
			sendEvent(eventCh, DownloadEvent{
				Filename:   filename,
				BytesDone:  done,
				BytesTotal: total,
				Speed:      speed,
			})
		}
		if errors.Is(readErr, io.EOF) {
			return done, nil
		}
		if readErr != nil {
			return done, readErr
		}
	}
}

func sendEvent(ch chan<- DownloadEvent, ev DownloadEvent) {
	if ch == nil {
		return
	}
	select {
	case ch <- ev:
	default:
	}
}
