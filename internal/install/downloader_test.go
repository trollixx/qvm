package install

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trollixx/qvm/internal/repository"
)

const payload = "0123456789abcdefghijklmnopqrstuvwxyz"

// serveArchive serves payload at any path, with Range support as mirrors have,
// and returns the server's URL and a func listing each request's Range header.
func serveArchive(t *testing.T, handler http.HandlerFunc) (string, func() []string) {
	t.Helper()
	if handler == nil {
		handler = func(w http.ResponseWriter, r *http.Request) {
			http.ServeContent(w, r, "archive.7z", time.Time{}, strings.NewReader(payload))
		}
	}
	var mu sync.Mutex
	var ranges []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		ranges = append(ranges, r.Header.Get("Range"))
		mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), ranges...)
	}
}

func archiveRef(baseURL, name string) repository.ArchiveRef {
	sum := sha1.Sum([]byte(payload))
	return repository.ArchiveRef{
		URL: baseURL + "/" + name, Filename: name, Size: int64(len(payload)), SHA1: hex.EncodeToString(sum[:]),
	}
}

func download(t *testing.T, dir string, refs ...repository.ArchiveRef) ([]string, error) {
	t.Helper()
	return NewDownloader(2, 10, dir).DownloadAll(context.Background(), refs, nil)
}

func assertPayload(t *testing.T, path string) {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, payload, string(got))
}

func TestDownloadAll(t *testing.T) {
	url, requests := serveArchive(t, nil)
	dir := t.TempDir()
	events := make(chan DownloadEvent, 256)

	paths, err := NewDownloader(2, 10, dir).DownloadAll(context.Background(),
		[]repository.ArchiveRef{archiveRef(url, "a.7z"), archiveRef(url, "b.7z")}, events)
	require.NoError(t, err)
	close(events)

	assert.Equal(t, []string{filepath.Join(dir, "a.7z"), filepath.Join(dir, "b.7z")}, paths, "in input order")
	for _, p := range paths {
		assertPayload(t, p)
		assert.NoFileExists(t, p+".part")
	}
	assert.Equal(t, []string{"", ""}, requests())

	done := map[string]bool{}
	for ev := range events {
		if ev.Done {
			done[ev.Filename] = true
			assert.Equal(t, int64(len(payload)), ev.BytesDone)
		}
	}
	assert.Equal(t, map[string]bool{"a.7z": true, "b.7z": true}, done)
}

// An interrupted download resumes from its .part file.
func TestDownload_ResumesPart(t *testing.T) {
	url, requests := serveArchive(t, nil)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.7z.part"), []byte(payload[:10]), 0o600))

	paths, err := download(t, dir, archiveRef(url, "a.7z"))
	require.NoError(t, err)
	assertPayload(t, paths[0])
	assert.Equal(t, []string{"bytes=10-"}, requests())
}

// A server that ignores Range sends the whole file, which replaces the .part.
func TestDownload_ServerIgnoresRange(t *testing.T) {
	url, requests := serveArchive(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(payload))
	})
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.7z.part"), []byte("stale"), 0o600))

	paths, err := download(t, dir, archiveRef(url, "a.7z"))
	require.NoError(t, err)
	assertPayload(t, paths[0])
	assert.Equal(t, []string{"bytes=5-"}, requests())
}

// A complete .part gets 416 for the rest, and is used as it is.
func TestDownload_CompletePartIsPromoted(t *testing.T) {
	url, requests := serveArchive(t, nil)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.7z.part"), []byte(payload), 0o600))

	paths, err := download(t, dir, archiveRef(url, "a.7z"))
	require.NoError(t, err)
	assertPayload(t, paths[0])
	assert.NoFileExists(t, paths[0]+".part")
	assert.Equal(t, []string{"bytes=36-"}, requests())
}

func TestDownload_CachedFileSkipsRequest(t *testing.T) {
	url, requests := serveArchive(t, nil)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.7z"), []byte(payload), 0o600))

	paths, err := download(t, dir, archiveRef(url, "a.7z"))
	require.NoError(t, err)
	assertPayload(t, paths[0])
	assert.Empty(t, requests(), "a cached file matching its SHA-1 isn't downloaded again")
}

func TestDownload_CorruptCacheIsReplaced(t *testing.T) {
	url, requests := serveArchive(t, nil)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.7z"), []byte("corrupt"), 0o600))

	paths, err := download(t, dir, archiveRef(url, "a.7z"))
	require.NoError(t, err)
	assertPayload(t, paths[0])
	assert.Equal(t, []string{""}, requests())
}

func TestDownloadAll_HTTPError(t *testing.T) {
	url, _ := serveArchive(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := download(t, t.TempDir(), archiveRef(url, "a.7z"))
	require.ErrorContains(t, err, "downloading a.7z")
	assert.ErrorContains(t, err, "HTTP 404")
}

func TestResumeOffset(t *testing.T) {
	part := filepath.Join(t.TempDir(), "a.7z.part")
	assert.Zero(t, resumeOffset(part, 36), "no .part")

	require.NoError(t, os.WriteFile(part, []byte(payload[:10]), 0o600))
	assert.Equal(t, int64(10), resumeOffset(part, 36))
	assert.Equal(t, int64(10), resumeOffset(part, 0), "size unknown")

	assert.Zero(t, resumeOffset(part, 5), "a .part bigger than the archive is corrupt")
	assert.NoFileExists(t, part)
}

func TestCopyWithProgress(t *testing.T) {
	var out strings.Builder
	events := make(chan DownloadEvent, 16)
	done, err := copyWithProgress(context.Background(), &out, strings.NewReader(payload[10:]),
		"a.7z", 10, int64(len(payload)), events)
	require.NoError(t, err)
	close(events)

	assert.Equal(t, int64(len(payload)), done, "counts from the resume offset")
	assert.Equal(t, payload[10:], out.String())
	var last DownloadEvent
	for ev := range events {
		last = ev
	}
	assert.Equal(t, int64(len(payload)), last.BytesDone)
	assert.Equal(t, int64(len(payload)), last.BytesTotal)
}

func TestCopyWithProgress_Canceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out strings.Builder
	done, err := copyWithProgress(ctx, &out, strings.NewReader(payload), "a.7z", 0, 0, nil)
	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, done)
	assert.Empty(t, out.String())
}
