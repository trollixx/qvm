package archive

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// basicArchive was made with 7-Zip and holds:
//
//	top.txt           "top\n"
//	dir/sub/file.txt  "hello qvm\n"
//	dir/empty/
const basicArchive = "testdata/basic.7z"

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, want, string(got))
}

func TestExtract(t *testing.T) {
	dest := t.TempDir()
	require.NoError(t, Extract(basicArchive, dest, nil))

	assertFile(t, filepath.Join(dest, "top.txt"), "top\n")
	assertFile(t, filepath.Join(dest, "dir", "sub", "file.txt"), "hello qvm\n")
	info, err := os.Stat(filepath.Join(dest, "dir", "empty"))
	require.NoError(t, err)
	assert.True(t, info.IsDir(), "empty directories are kept")
}

// Extracting over existing files replaces them, as a reinstall does.
func TestExtract_Overwrites(t *testing.T) {
	dest := t.TempDir()
	top := filepath.Join(dest, "top.txt")
	require.NoError(t, os.WriteFile(top, []byte("stale content, longer than the new"), 0o644))

	require.NoError(t, Extract(basicArchive, dest, nil))
	assertFile(t, top, "top\n")
}

func TestExtract_Progress(t *testing.T) {
	var last, total int64
	calls := 0
	require.NoError(t, Extract(basicArchive, t.TempDir(), func(done, all int64) {
		assert.GreaterOrEqual(t, done, last, "progress must not go backwards")
		last, total = done, all
		calls++
	}))

	assert.Positive(t, calls)
	assert.Equal(t, int64(len("top\n")+len("hello qvm\n")), total)
	assert.Equal(t, total, last, "progress ends at the total")
}

func TestExtract_NotAnArchive(t *testing.T) {
	src := filepath.Join(t.TempDir(), "bad.7z")
	require.NoError(t, os.WriteFile(src, []byte("not a 7z archive"), 0o644))

	assert.ErrorContains(t, Extract(src, t.TempDir(), nil), "opening archive")
}

func TestTargetPath(t *testing.T) {
	dest := t.TempDir()
	tests := []struct{ name, want string }{
		{"dir/sub/file.txt", filepath.Join(dest, "dir", "sub", "file.txt")},
		{"dir/", filepath.Join(dest, "dir")},
		{"", dest},
		// Hostile names are rooted first, so they stay under dest.
		{"../evil.txt", filepath.Join(dest, "evil.txt")},
		{"a/../../evil.txt", filepath.Join(dest, "evil.txt")},
		{"/etc/evil.txt", filepath.Join(dest, "etc", "evil.txt")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := targetPath(dest, tc.name)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// Backslashes separate paths only on Windows, but either way these stay in dest.
func TestTargetPath_Backslashes(t *testing.T) {
	dest := t.TempDir()
	for _, name := range []string{`..\..\evil.txt`, `C:\evil.txt`, `\\server\share\evil.txt`} {
		got, err := targetPath(dest, name)
		require.NoError(t, err, name)
		assert.True(t, strings.HasPrefix(got, dest+string(os.PathSeparator)), "%s -> %s", name, got)
	}
}

func TestExtractSymlink(t *testing.T) {
	link := filepath.Join(t.TempDir(), "Current")
	err := extractSymlink(strings.NewReader("A"), link)
	if err != nil && runtime.GOOS == "windows" {
		t.Skipf("can't create symlinks here: %v", err)
	}
	require.NoError(t, err)
	got, err := os.Readlink(link)
	require.NoError(t, err)
	assert.Equal(t, "A", got)

	// Extracting it again replaces the link.
	require.NoError(t, extractSymlink(strings.NewReader("B"), link))
	got, err = os.Readlink(link)
	require.NoError(t, err)
	assert.Equal(t, "B", got)
}
