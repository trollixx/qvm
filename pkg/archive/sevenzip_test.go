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
	dest := t.TempDir()
	link := filepath.Join(dest, "Current")
	err := extractSymlink(strings.NewReader("A"), dest, link)
	if err != nil && runtime.GOOS == "windows" {
		t.Skipf("can't create symlinks here: %v", err)
	}
	require.NoError(t, err)
	got, err := os.Readlink(link)
	require.NoError(t, err)
	assert.Equal(t, "A", got)

	// Extracting it again replaces the link.
	require.NoError(t, extractSymlink(strings.NewReader("B"), dest, link))
	got, err = os.Readlink(link)
	require.NoError(t, err)
	assert.Equal(t, "B", got)
}

// An unsafe link fails without being created.
func TestExtractSymlink_Unsafe(t *testing.T) {
	dest := t.TempDir()
	link := filepath.Join(dest, "lnk")
	err := extractSymlink(strings.NewReader("../outside"), dest, link)
	require.ErrorContains(t, err, "unsafe symlink")
	_, err = os.Lstat(link)
	assert.True(t, os.IsNotExist(err), "the link must not exist")
}

func TestCheckSymlink(t *testing.T) {
	dest := t.TempDir()
	at := func(parts ...string) string { return filepath.Join(append([]string{dest}, parts...)...) }
	tests := []struct {
		target, link, wantErr string
	}{
		// Links like Qt's.
		{at("QtCore.framework", "Versions", "Current"), "A", ""},
		{at("QtCore.framework", "QtCore"), "Versions/Current/QtCore", ""},
		{at("lib", "libQt6Core.so"), "libQt6Core.so.6", ""},
		{at("lib", "sub", "l"), "../other", ""},
		{at("l"), ".", ""},
		{at("a", "l"), "./b", ""},

		{at("l"), "", "not a relative path"},
		{at("l"), "/etc/passwd", "not a relative path"},
		{at("l"), "../outside", "points outside"},
		{at("a", "l"), "../../outside", "points outside"},
		{at("l"), "a/..", ".. after a name"},
		{at("l"), "a/../b", ".. after a name"},
	}
	if runtime.GOOS == "windows" {
		tests = append(tests,
			struct{ target, link, wantErr string }{at("l"), `C:\Windows`, "not a relative path"},
			struct{ target, link, wantErr string }{at("l"), `..\..\outside`, "points outside"},
		)
	}
	for _, tc := range tests {
		t.Run(tc.link, func(t *testing.T) {
			err := checkSymlink(dest, tc.target, tc.link)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

// A link can't be made in a symlinked directory: the check on its target
// would be relative to where the path seems to be, not where it really is.
func TestCheckSymlink_InSymlinkedDir(t *testing.T) {
	dest := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dest, "real"), 0o755))
	if err := os.Symlink("real", filepath.Join(dest, "lnk")); err != nil {
		t.Skipf("can't create symlinks here: %v", err)
	}

	require.NoError(t, checkSymlink(dest, filepath.Join(dest, "real", "l"), "x"))
	err := checkSymlink(dest, filepath.Join(dest, "lnk", "l"), "x")
	require.ErrorContains(t, err, "in symlinked directory "+filepath.Join(dest, "lnk"))
}

func TestClimbsAfterName(t *testing.T) {
	for path, want := range map[string]bool{
		"A": false, "../x": false, "../../x/y": false, "./x": false, "a//b": false,
		"a/..": true, "a/../b": true, "../a/../b": true, "a/./..": true,
	} {
		assert.Equal(t, want, climbsAfterName(path), path)
	}
}
