package archive

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/bodgit/sevenzip"
)

// ProgressFunc is called periodically during extraction with bytes extracted so far.
type ProgressFunc func(bytesExtracted, bytesTotal int64)

// Extract extracts a .7z archive at src into destDir.
// progress is called with extraction progress (may be nil).
func Extract(src, destDir string, progress ProgressFunc) error {
	r, err := sevenzip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("opening archive %s: %w", src, err)
	}
	defer r.Close()

	// Calculate total uncompressed size.
	var total int64
	for _, f := range r.File {
		total += int64(f.FileInfo().Size())
	}

	var done atomic.Int64
	for _, f := range r.File {
		var cb func(n int)
		if progress != nil {
			cb = func(n int) {
				progress(done.Add(int64(n)), total)
			}
		}
		if err := extractFile(f, destDir, cb); err != nil {
			return err
		}
		// Ensure done is at least the file size even if cb was nil.
		if progress == nil {
			done.Add(int64(f.FileInfo().Size()))
		}
	}
	return nil
}

// countingWriter wraps an [io.Writer] and calls onWrite(n) after each Write.
type countingWriter struct {
	w       io.Writer
	onWrite func(n int)
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	if n > 0 && cw.onWrite != nil {
		cw.onWrite(n)
	}
	return n, err
}

func extractFile(f *sevenzip.File, destDir string, onBytes func(n int)) error {
	// Sanitize path.
	target := filepath.Join(destDir, filepath.Clean("/"+f.Name))
	// Path traversal check: target must be within destDir.
	cleanDest := filepath.Clean(destDir) + string(os.PathSeparator)
	cleanTarget := filepath.Clean(target)
	if cleanTarget != filepath.Clean(destDir) && !strings.HasPrefix(cleanTarget, cleanDest) {
		return fmt.Errorf("path traversal blocked: %s", f.Name)
	}
	if f.FileInfo().IsDir() {
		return os.MkdirAll(target, 0o755)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("creating directory for %s: %w", target, err)
	}

	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("opening %s in archive: %w", f.Name, err)
	}
	defer rc.Close()

	out, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("creating %s: %w", target, err)
	}
	defer out.Close()

	var dst io.Writer = out
	if onBytes != nil {
		dst = &countingWriter{w: out, onWrite: onBytes}
	}
	if _, err := io.Copy(dst, rc); err != nil {
		return fmt.Errorf("extracting %s: %w", f.Name, err)
	}
	return nil
}
