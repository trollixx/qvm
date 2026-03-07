package install

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractOne_UnknownFormat(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "archive.zip")
	require.NoError(t, os.WriteFile(archivePath, []byte("not a real archive"), 0o644))

	err := extractOne(archivePath, dir, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported archive format")
	assert.Contains(t, err.Error(), "archive.zip")
}

func TestExtractOne_UnknownFormat_Table(t *testing.T) {
	tests := []struct {
		name     string
		filename string
	}{
		{"zip file", "archive.zip"},
		{"rar file", "archive.rar"},
		{"bz2 file", "archive.tar.bz2"},
		{"plain file", "readme.txt"},
		{"no extension", "archive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			archivePath := filepath.Join(dir, tt.filename)
			require.NoError(t, os.WriteFile(archivePath, []byte("data"), 0o644))

			err := extractOne(archivePath, dir, nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unsupported archive format")
		})
	}
}

// createTarGz creates an in-memory tar.gz archive with the given entries.
func createTarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer

	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Mode:     0o644,
			Size:     int64(len(e.content)),
			Typeflag: e.typeflag,
		}
		if e.typeflag == tar.TypeDir {
			hdr.Mode = 0o755
			hdr.Size = 0
		}
		require.NoError(t, tw.WriteHeader(hdr))
		if len(e.content) > 0 {
			_, err := tw.Write([]byte(e.content))
			require.NoError(t, err)
		}
	}

	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())
	return buf.Bytes()
}

type tarEntry struct {
	name     string
	content  string
	typeflag byte
}

func TestExtractTar_ValidTarGz(t *testing.T) {
	entries := []tarEntry{
		{name: "dir/", typeflag: tar.TypeDir},
		{name: "dir/hello.txt", content: "Hello, World!", typeflag: tar.TypeReg},
		{name: "dir/sub/", typeflag: tar.TypeDir},
		{name: "dir/sub/nested.txt", content: "nested content", typeflag: tar.TypeReg},
	}

	archiveData := createTarGz(t, entries)

	dir := t.TempDir()
	archivePath := filepath.Join(dir, "test.tar.gz")
	require.NoError(t, os.WriteFile(archivePath, archiveData, 0o644))

	destDir := filepath.Join(dir, "output")
	err := extractTar(archivePath, destDir, "test.tar.gz", nil)
	require.NoError(t, err)

	// Verify extracted files.
	data, err := os.ReadFile(filepath.Join(destDir, "dir", "hello.txt"))
	require.NoError(t, err)
	assert.Equal(t, "Hello, World!", string(data))

	data, err = os.ReadFile(filepath.Join(destDir, "dir", "sub", "nested.txt"))
	require.NoError(t, err)
	assert.Equal(t, "nested content", string(data))
}

func TestExtractTar_EmitsEvents(t *testing.T) {
	entries := []tarEntry{
		{name: "file.txt", content: "some data", typeflag: tar.TypeReg},
	}

	archiveData := createTarGz(t, entries)

	dir := t.TempDir()
	archivePath := filepath.Join(dir, "events.tar.gz")
	require.NoError(t, os.WriteFile(archivePath, archiveData, 0o644))

	destDir := filepath.Join(dir, "output")
	eventCh := make(chan ExtractionEvent, 64)

	err := extractTar(archivePath, destDir, "events.tar.gz", eventCh)
	require.NoError(t, err)
	close(eventCh)

	var events []ExtractionEvent
	for ev := range eventCh {
		events = append(events, ev)
	}
	assert.NotEmpty(t, events)
	assert.Equal(t, "events.tar.gz", events[0].Archive)
}

func TestExtractTar_PathTraversalBlocked(t *testing.T) {
	// Manually construct a tar.gz with a path-traversal entry.
	entries := []tarEntry{
		{name: "../../evil.txt", content: "malicious", typeflag: tar.TypeReg},
	}

	archiveData := createTarGz(t, entries)

	dir := t.TempDir()
	archivePath := filepath.Join(dir, "evil.tar.gz")
	require.NoError(t, os.WriteFile(archivePath, archiveData, 0o644))

	destDir := filepath.Join(dir, "output")
	err := extractTar(archivePath, destDir, "evil.tar.gz", nil)

	if runtime.GOOS == "windows" {
		// On Windows, filepath.Clean("/" + "../../evil.txt") resolves to
		// "\evil.txt", so filepath.Join places it safely inside destDir.
		// The traversal is neutralized by Go's filepath handling.
		require.NoError(t, err)
		// File should exist inside destDir, not outside.
		_, err = os.Stat(filepath.Join(destDir, "evil.txt"))
		require.NoError(t, err, "file should be safely extracted inside destDir")
		_, err = os.Stat(filepath.Join(dir, "evil.txt"))
		assert.True(t, os.IsNotExist(err), "file should not exist outside destDir")
	} else {
		require.Error(t, err)
		assert.Contains(t, err.Error(), "path traversal blocked")
		// Ensure the evil file was not created outside destDir.
		_, err = os.Stat(filepath.Join(dir, "evil.txt"))
		assert.True(t, os.IsNotExist(err), "evil file should not exist outside destDir")
	}
}

func TestExtractTar_NilEventChannel(t *testing.T) {
	entries := []tarEntry{
		{name: "safe.txt", content: "ok", typeflag: tar.TypeReg},
	}

	archiveData := createTarGz(t, entries)

	dir := t.TempDir()
	archivePath := filepath.Join(dir, "nil-ch.tar.gz")
	require.NoError(t, os.WriteFile(archivePath, archiveData, 0o644))

	destDir := filepath.Join(dir, "output")
	// nil channel should not panic.
	err := extractTar(archivePath, destDir, "nil-ch.tar.gz", nil)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(destDir, "safe.txt"))
	require.NoError(t, err)
	assert.Equal(t, "ok", string(data))
}
