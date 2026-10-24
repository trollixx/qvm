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

// targetPath returns where the archive entry name goes under destDir. Names are
// rooted first, so .. can't climb above destDir; anything that still lands
// outside it is an error.
func targetPath(destDir, name string) (string, error) {
	target := filepath.Join(destDir, filepath.Clean("/"+name))
	if !isWithin(destDir, target) {
		return "", fmt.Errorf("path traversal blocked: %s", name)
	}
	return target, nil
}

// isWithin reports whether the cleaned path is dir or under it.
func isWithin(dir, path string) bool {
	dir, path = filepath.Clean(dir), filepath.Clean(path)
	return path == dir || strings.HasPrefix(path, dir+string(os.PathSeparator))
}

func extractFile(f *sevenzip.File, destDir string, onBytes func(n int)) error {
	target, err := targetPath(destDir, f.Name)
	if err != nil {
		return err
	}
	mode := f.FileInfo().Mode()
	if mode.IsDir() {
		return os.MkdirAll(target, 0o755) //nolint:gosec // 0755 for Qt SDK
	}

	err = os.MkdirAll(filepath.Dir(target), 0o755) //nolint:gosec // 0755 for Qt SDK
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
		return extractSymlink(rc, destDir, target)
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

// extractSymlink reads a link target from rc and creates a symlink at target,
// if it's safe under destDir (see checkSymlink).
func extractSymlink(rc io.Reader, destDir, target string) error {
	linkTarget, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("reading symlink target for %s: %w", target, err)
	}
	err = checkSymlink(destDir, target, string(linkTarget))
	if err != nil {
		return err
	}
	// Replace any existing entry so re-extraction is idempotent.
	_ = os.Remove(target)
	err = os.Symlink(string(linkTarget), target)
	if err != nil {
		return fmt.Errorf("creating symlink %s: %w", target, err)
	}
	return nil
}

// checkSymlink returns an error unless a symlink at target pointing to link
// stays inside destDir. Qt only has relative links within an install, like
// Versions/Current -> A in macOS frameworks; a link out of it would let later
// entries be written outside destDir through the link.
//
// The check works on paths, which match where links really point only if no
// symlink is in the way: so the link can't be in a symlinked directory, and its
// target can't climb back up after a name, as a/.. does, in case a is a symlink.
func checkSymlink(destDir, target, link string) error {
	unsafe := func(why string) error {
		return fmt.Errorf("unsafe symlink %s -> %s: %s", target, link, why)
	}
	l := filepath.FromSlash(link)
	if l == "" || filepath.IsAbs(l) || filepath.VolumeName(l) != "" || strings.HasPrefix(l, string(os.PathSeparator)) {
		return unsafe("not a relative path")
	}
	if climbsAfterName(l) {
		return unsafe(".. after a name")
	}
	if !isWithin(destDir, filepath.Join(filepath.Dir(target), l)) {
		return unsafe("points outside " + destDir)
	}
	dir, err := symlinkedParent(destDir, target)
	if err != nil {
		return fmt.Errorf("checking symlink %s: %w", target, err)
	}
	if dir != "" {
		return unsafe("in symlinked directory " + dir)
	}
	return nil
}

// climbsAfterName reports whether path has a .. after a name, as a/.. has.
func climbsAfterName(path string) bool {
	seenName := false
	for part := range strings.SplitSeq(filepath.ToSlash(path), "/") {
		switch part {
		case "", ".":
		case "..":
			if seenName {
				return true
			}
		default:
			seenName = true
		}
	}
	return false
}

// symlinkedParent returns the first directory between destDir and path that is
// a symlink, or "" if there's none.
func symlinkedParent(destDir, path string) (string, error) {
	rel, err := filepath.Rel(destDir, filepath.Dir(path))
	if err != nil {
		return "", err
	}
	dir := filepath.Clean(destDir)
	for part := range strings.SplitSeq(rel, string(os.PathSeparator)) {
		if part == "." {
			continue
		}
		dir = filepath.Join(dir, part)
		info, statErr := os.Lstat(dir)
		if os.IsNotExist(statErr) {
			return "", nil // nothing below it exists either
		}
		if statErr != nil {
			return "", statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return dir, nil
		}
	}
	return "", nil
}
