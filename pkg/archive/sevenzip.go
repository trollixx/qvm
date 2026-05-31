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
		total += f.FileInfo().Size()
	}

	var done atomic.Int64
	for _, f := range r.File {
		var cb func(n int)
		if progress != nil {
			cb = func(n int) {
				progress(done.Add(int64(n)), total)
			}
		}
		err = extractFile(f, destDir, cb)
		if err != nil {
			return err
		}
		// Ensure done is at least the file size even if cb was nil.
		if progress == nil {
			done.Add(f.FileInfo().Size())
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
	mode := f.FileInfo().Mode()
	if mode.IsDir() {
		return os.MkdirAll(target, 0o755) //nolint:gosec // 0755 for Qt SDK
	}

	err := os.MkdirAll(filepath.Dir(target), 0o755) //nolint:gosec // 0755 for Qt SDK
	if err != nil {
		return fmt.Errorf("creating directory for %s: %w", target, err)
	}

	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("opening %s in archive: %w", f.Name, err)
	}
	defer rc.Close()

	// Symlink entries (common in Qt's macOS frameworks) store their link target
	// as the file content. Recreate them as real symlinks rather than copying
	// the target string into a regular file.
	if mode&os.ModeSymlink != 0 {
		return extractSymlink(rc, target)
	}

	// Preserve the archive's Unix permission bits so executables (qmake, etc.)
	// and shared libraries keep their execute bit on macOS/Linux. On Windows the
	// permission bits are inert. os.Create would drop them (0644), making the
	// extracted tools non-executable.
	perm := mode.Perm()
	if perm == 0 {
		perm = 0o644
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("creating %s: %w", target, err)
	}
	defer out.Close()

	var dst io.Writer = out
	if onBytes != nil {
		dst = &countingWriter{w: out, onWrite: onBytes}
	}
	_, err = io.Copy(dst, rc)
	if err != nil {
		return fmt.Errorf("extracting %s: %w", f.Name, err)
	}
	// OpenFile applies the umask; chmod to the exact mode so the execute bit
	// survives a restrictive umask.
	err = out.Chmod(perm)
	if err != nil {
		return fmt.Errorf("setting permissions on %s: %w", target, err)
	}
	return nil
}

// extractSymlink reads a link target from rc and creates a symlink at target.
func extractSymlink(rc io.Reader, target string) error {
	linkTarget, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("reading symlink target for %s: %w", target, err)
	}
	// Replace any existing entry so re-extraction is idempotent.
	_ = os.Remove(target)
	err = os.Symlink(string(linkTarget), target)
	if err != nil {
		return fmt.Errorf("creating symlink %s: %w", target, err)
	}
	return nil
}
