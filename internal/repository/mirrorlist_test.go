package repository_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trollixx/qvm/internal/repository"
)

// -- parseMirrorListHTML -------------------------------------------------------

var mirrorListFixtureHTML = []byte(`<!DOCTYPE html>
<html><head><title>Qt Mirror List</title></head><body>
<nav>
  <a href="https://download.qt.io/">Qt Downloads</a>
  <a href="https://www.qt.io/">Qt Home</a>
  <a href="https://qt.io/products">Qt Products</a>
  <a href="https://master.qt.io/">Qt Master</a>
</nav>
<table>
  <tr>
    <td>Europe</td>
    <td><a href="https://ftp.fau.de/qtproject/">ftp.fau.de</a>
        <a href="rsync://ftp.fau.de/qtproject/">rsync</a></td>
  </tr>
  <tr>
    <td>Europe</td>
    <td><a href="https://mirrors.dotsrc.org/qtproject/">mirrors.dotsrc.org</a></td>
  </tr>
  <tr>
    <td>Europe</td>
    <!-- URL without trailing slash - should be normalized -->
    <td><a href="https://mirrors.ukfast.co.uk/sites/qt.io">mirrors.ukfast.co.uk</a></td>
  </tr>
  <tr>
    <td>Asia</td>
    <!-- Duplicate - should appear only once -->
    <td><a href="https://ftp.fau.de/qtproject/">ftp.fau.de (dup)</a></td>
  </tr>
  <tr>
    <td>Asia</td>
    <td><a href="https://ftp.jaist.ac.jp/pub/qtproject/">ftp.jaist.ac.jp</a></td>
  </tr>
</table>
<footer><a href="https://download.qt.io/static/mirrorlist/">This page</a></footer>
</body></html>`)

func TestParseMirrorListHTML_ExtractsMirrors(t *testing.T) {
	mirrors := repository.ParseMirrorListHTML(mirrorListFixtureHTML)

	assert.ElementsMatch(t, []string{
		"https://ftp.fau.de/qtproject/",
		"https://mirrors.dotsrc.org/qtproject/",
		"https://mirrors.ukfast.co.uk/sites/qt.io/",
		"https://ftp.jaist.ac.jp/pub/qtproject/",
	}, mirrors)
}

func TestParseMirrorListHTML_ExcludesQtOwnDomains(t *testing.T) {
	mirrors := repository.ParseMirrorListHTML(mirrorListFixtureHTML)

	for _, m := range mirrors {
		assert.NotContains(t, m, "download.qt.io", "should exclude download.qt.io")
		assert.NotContains(t, m, "master.qt.io", "should exclude master.qt.io")
		assert.NotContains(t, m, "www.qt.io", "should exclude www.qt.io")
	}
}

func TestParseMirrorListHTML_NormalizesTrailingSlash(t *testing.T) {
	mirrors := repository.ParseMirrorListHTML(mirrorListFixtureHTML)

	for _, m := range mirrors {
		assert.True(t, len(m) > 0 && m[len(m)-1] == '/', "mirror %q should end with /", m)
	}
}

func TestParseMirrorListHTML_DeduplicatesMirrors(t *testing.T) {
	mirrors := repository.ParseMirrorListHTML(mirrorListFixtureHTML)

	seen := map[string]int{}
	for _, m := range mirrors {
		seen[m]++
	}
	for url, count := range seen {
		assert.Equal(t, 1, count, "mirror %q should appear exactly once", url)
	}
}

func TestParseMirrorListHTML_Empty(t *testing.T) {
	mirrors := repository.ParseMirrorListHTML([]byte(`<html><body>No mirrors here.</body></html>`))
	assert.Empty(t, mirrors)
}

// -- MirrorListCache -----------------------------------------------------------

func TestMirrorListCache_NotExistsBeforeSave(t *testing.T) {
	c := repository.NewMirrorListCacheAt(filepath.Join(t.TempDir(), "mirrors.json"))
	assert.False(t, c.Exists())
}

func TestMirrorListCache_LoadReturnsNilBeforeSave(t *testing.T) {
	c := repository.NewMirrorListCacheAt(filepath.Join(t.TempDir(), "mirrors.json"))
	mirrors, err := c.Load()
	require.NoError(t, err)
	assert.Nil(t, mirrors)
}

func TestMirrorListCache_SaveAndLoad(t *testing.T) {
	c := repository.NewMirrorListCacheAt(filepath.Join(t.TempDir(), "mirrors.json"))

	input := []string{
		"https://ftp.fau.de/qtproject/",
		"https://mirrors.dotsrc.org/qtproject/",
	}
	require.NoError(t, c.Save(input))

	got, err := c.Load()
	require.NoError(t, err)
	assert.Equal(t, input, got)
}

func TestMirrorListCache_ExistsAfterSave(t *testing.T) {
	c := repository.NewMirrorListCacheAt(filepath.Join(t.TempDir(), "mirrors.json"))
	require.NoError(t, c.Save([]string{"https://example.com/"}))
	assert.True(t, c.Exists())
}

func TestMirrorListCache_SaveOverwritesPrevious(t *testing.T) {
	c := repository.NewMirrorListCacheAt(filepath.Join(t.TempDir(), "mirrors.json"))

	require.NoError(t, c.Save([]string{"https://old.example.com/"}))
	require.NoError(t, c.Save([]string{"https://new.example.com/"}))

	got, err := c.Load()
	require.NoError(t, err)
	assert.Equal(t, []string{"https://new.example.com/"}, got)
}

func TestMirrorListCache_Path(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mirrors.json")
	c := repository.NewMirrorListCacheAt(path)
	assert.Equal(t, path, c.Path())
}
