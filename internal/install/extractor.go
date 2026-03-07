package install

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ulikunitz/xz"

	"github.com/trollixx/qvm/pkg/archive"
)

// maxExtractedFileSize is the per-entry decompression limit to mitigate zip-bomb attacks.
const maxExtractedFileSize = 2 * 1024 * 1024 * 1024 // 2 GiB

// ExtractionEvent is emitted during archive extraction.
type ExtractionEvent struct {
	Archive    string
	BytesDone  int64
	BytesTotal int64
}

// ExtractAll extracts all downloaded archives into destDir.
func ExtractAll(archivePaths []string, destDir string, eventCh chan<- ExtractionEvent) error {
	for _, path := range archivePaths {
		err := extractOne(path, destDir, eventCh)
		if err != nil {
			return err
		}
	}
	return nil
}

func extractOne(archivePath, destDir string, eventCh chan<- ExtractionEvent) error {
	name := filepath.Base(archivePath)

	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".7z"):
		return extract7z(archivePath, destDir, name, eventCh)
	case strings.HasSuffix(lower, ".tar.xz"), strings.HasSuffix(lower, ".tar.gz"):
		return extractTar(archivePath, destDir, name, eventCh)
	default:
		return fmt.Errorf("unsupported archive format: %s", name)
	}
}

func extract7z(src, destDir, name string, eventCh chan<- ExtractionEvent) error {
	err := os.MkdirAll(destDir, 0o755) //nolint:gosec // 0755 for Qt SDK
	if err != nil {
		return err
	}
	progress := func(done, total int64) {
		if eventCh == nil {
			return
		}
		select {
		case eventCh <- ExtractionEvent{Archive: name, BytesDone: done, BytesTotal: total}:
		default:
		}
	}
	return archive.Extract(src, destDir, progress)
}

func extractTar(src, destDir, name string, eventCh chan<- ExtractionEvent) error {
	err := os.MkdirAll(destDir, 0o755) //nolint:gosec // 0755 for Qt SDK
	if err != nil {
		return err
	}

	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening %s: %w", src, err)
	}
	defer f.Close()

	var reader io.Reader
	lower := strings.ToLower(src)
	switch {
	case strings.HasSuffix(lower, ".tar.xz"):
		xzReader, err := xz.NewReader(f)
		if err != nil {
			return fmt.Errorf("creating xz reader for %s: %w", src, err)
		}
		reader = xzReader
	case strings.HasSuffix(lower, ".tar.gz"):
		gzReader, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("creating gzip reader for %s: %w", src, err)
		}
		defer gzReader.Close()
		reader = gzReader
	default:
		return fmt.Errorf("unsupported tar compression: %s", src)
	}

	tr := tar.NewReader(reader)
	cleanDest := filepath.Clean(destDir) + string(os.PathSeparator)

	var bytesWritten int64
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar entry: %w", err)
		}

		target := filepath.Join(destDir, filepath.Clean("/"+header.Name))

		// Path traversal check.
		cleanTarget := filepath.Clean(target)
		if cleanTarget != filepath.Clean(destDir) && !strings.HasPrefix(cleanTarget, cleanDest) {
			return fmt.Errorf("path traversal blocked: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil { //nolint:gosec // 0755 for Qt SDK
				return fmt.Errorf("creating directory %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil { //nolint:gosec // 0755 for Qt SDK
				return fmt.Errorf("creating parent directory for %s: %w", target, err)
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode&0o777))
			if err != nil {
				return fmt.Errorf("creating %s: %w", target, err)
			}
			n, copyErr := io.Copy(out, io.LimitReader(tr, maxExtractedFileSize))
			closeErr := out.Close()
			if copyErr != nil {
				return fmt.Errorf("extracting %s: %w", header.Name, copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("closing %s: %w", target, closeErr)
			}
			bytesWritten += n
			if eventCh != nil {
				select {
				case eventCh <- ExtractionEvent{Archive: name, BytesDone: bytesWritten, BytesTotal: 0}:
				default:
				}
			}
		case tar.TypeSymlink:
			if err := os.Symlink(header.Linkname, target); err != nil {
				return fmt.Errorf("creating symlink %s: %w", target, err)
			}
		}
	}

	return nil
}
