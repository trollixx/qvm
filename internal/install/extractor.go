package install

import (
	"archive/tar"
	"compress/gzip"
	"errors"
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

// openTarDecompressor returns a decompressed reader for src based on its filename suffix.
// The caller is responsible for invoking the returned close function when done.
func openTarDecompressor(src string, f io.Reader) (io.Reader, func() error, error) {
	lower := strings.ToLower(src)
	switch {
	case strings.HasSuffix(lower, ".tar.xz"):
		r, err := xz.NewReader(f)
		if err != nil {
			return nil, nil, fmt.Errorf("creating xz reader for %s: %w", src, err)
		}
		return r, func() error { return nil }, nil
	case strings.HasSuffix(lower, ".tar.gz"):
		r, err := gzip.NewReader(f)
		if err != nil {
			return nil, nil, fmt.Errorf("creating gzip reader for %s: %w", src, err)
		}
		return r, r.Close, nil
	default:
		return nil, nil, fmt.Errorf("unsupported tar compression: %s", src)
	}
}

// extractTarEntry writes a single tar entry (dir/regular/symlink) to target.
// Returns the number of bytes written for regular files; zero for other types.
func extractTarEntry(tr *tar.Reader, header *tar.Header, target string) (int64, error) {
	switch header.Typeflag {
	case tar.TypeDir:
		err := os.MkdirAll(target, 0o755) //nolint:gosec // 0755 for Qt SDK
		if err != nil {
			return 0, fmt.Errorf("creating directory %s: %w", target, err)
		}
		return 0, nil
	case tar.TypeReg:
		err := os.MkdirAll(filepath.Dir(target), 0o755) //nolint:gosec // 0755 for Qt SDK
		if err != nil {
			return 0, fmt.Errorf("creating parent directory for %s: %w", target, err)
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode&0o777))
		if err != nil {
			return 0, fmt.Errorf("creating %s: %w", target, err)
		}
		n, copyErr := io.Copy(out, io.LimitReader(tr, maxExtractedFileSize))
		closeErr := out.Close()
		if copyErr != nil {
			return 0, fmt.Errorf("extracting %s: %w", header.Name, copyErr)
		}
		if closeErr != nil {
			return 0, fmt.Errorf("closing %s: %w", target, closeErr)
		}
		return n, nil
	case tar.TypeSymlink:
		err := os.Symlink(header.Linkname, target)
		if err != nil {
			return 0, fmt.Errorf("creating symlink %s: %w", target, err)
		}
		return 0, nil
	}
	return 0, nil
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

	reader, closeReader, err := openTarDecompressor(src, f)
	if err != nil {
		return err
	}
	defer func() { _ = closeReader() }()

	tr := tar.NewReader(reader)
	cleanDest := filepath.Clean(destDir) + string(os.PathSeparator)

	var bytesWritten int64
	var header *tar.Header
	for {
		header, err = tr.Next()
		if errors.Is(err, io.EOF) {
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

		n, entryErr := extractTarEntry(tr, header, target)
		if entryErr != nil {
			return entryErr
		}
		if header.Typeflag == tar.TypeReg {
			bytesWritten += n
			if eventCh != nil {
				select {
				case eventCh <- ExtractionEvent{Archive: name, BytesDone: bytesWritten, BytesTotal: 0}:
				default:
				}
			}
		}
	}

	return nil
}
