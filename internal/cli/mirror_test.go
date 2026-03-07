package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// -- dedupURLs -----------------------------------------------------------------

func TestDedupURLs_NoDuplicates(t *testing.T) {
	result := dedupURLs("https://a.example/", []string{"https://b.example/", "https://c.example/"})
	assert.Equal(t, []string{"https://a.example/", "https://b.example/", "https://c.example/"}, result)
}

func TestDedupURLs_PrimaryInFallbacks(t *testing.T) {
	result := dedupURLs("https://a.example/", []string{"https://b.example/", "https://a.example/"})
	assert.Equal(t, []string{"https://a.example/", "https://b.example/"}, result)
}

func TestDedupURLs_DuplicateFallbacks(t *testing.T) {
	result := dedupURLs("https://a.example/", []string{"https://b.example/", "https://b.example/"})
	assert.Equal(t, []string{"https://a.example/", "https://b.example/"}, result)
}

func TestDedupURLs_EmptyFallbacks(t *testing.T) {
	result := dedupURLs("https://a.example/", nil)
	assert.Equal(t, []string{"https://a.example/"}, result)
}

// -- mirrorDisplayName ---------------------------------------------------------

func TestMirrorDisplayName_StripHTTPS(t *testing.T) {
	assert.Equal(
		t,
		"download.qt.io/online/qtsdkrepository",
		mirrorDisplayName("https://download.qt.io/online/qtsdkrepository/"),
	)
}

func TestMirrorDisplayName_StripHTTP(t *testing.T) {
	assert.Equal(
		t,
		"download.qt.io/online/qtsdkrepository",
		mirrorDisplayName("http://download.qt.io/online/qtsdkrepository/"),
	)
}

func TestMirrorDisplayName_NoTrailingSlash(t *testing.T) {
	assert.Equal(t, "download.qt.io/path", mirrorDisplayName("https://download.qt.io/path"))
}

// -- filterBlacklist -----------------------------------------------------------

func TestFilterBlacklist_RemovesBlacklisted(t *testing.T) {
	mirrors := []string{"https://a.example/", "https://b.example/", "https://c.example/"}
	result := filterBlacklist(mirrors, []string{"https://b.example/"})
	assert.Equal(t, []string{"https://a.example/", "https://c.example/"}, result)
}

func TestFilterBlacklist_EmptyBlacklist(t *testing.T) {
	mirrors := []string{"https://a.example/", "https://b.example/"}
	result := filterBlacklist(mirrors, nil)
	assert.Equal(t, mirrors, result)
}

func TestFilterBlacklist_NormalizesTrailingSlash(t *testing.T) {
	// Blacklist entry without trailing slash should still match mirror with slash.
	mirrors := []string{"https://a.example/"}
	result := filterBlacklist(mirrors, []string{"https://a.example"})
	assert.Empty(t, result)
}

func TestFilterBlacklist_AllBlacklisted(t *testing.T) {
	mirrors := []string{"https://a.example/", "https://b.example/"}
	result := filterBlacklist(mirrors, []string{"https://a.example/", "https://b.example/"})
	assert.Empty(t, result)
}

// -- isBlacklisted -------------------------------------------------------------

func TestIsBlacklisted_True(t *testing.T) {
	assert.True(t, isBlacklisted("https://bad.example/", []string{"https://bad.example/"}))
}

func TestIsBlacklisted_False(t *testing.T) {
	assert.False(t, isBlacklisted("https://good.example/", []string{"https://bad.example/"}))
}

func TestIsBlacklisted_NormalizesBothSides(t *testing.T) {
	// URL without slash vs. blacklist entry with slash.
	assert.True(t, isBlacklisted("https://bad.example", []string{"https://bad.example/"}))
}

// -- normalizeURL --------------------------------------------------------------

func TestNormalizeURL_AddsSlash(t *testing.T) {
	assert.Equal(t, "https://a.example/", normalizeURL("https://a.example"))
}

func TestNormalizeURL_PreservesExistingSlash(t *testing.T) {
	assert.Equal(t, "https://a.example/", normalizeURL("https://a.example/"))
}

// -- mirror command routing ----------------------------------------------------

func TestMirrorSelect_NoArgs(t *testing.T) {
	assertErrContains(t, run(t, "mirror", "select"), "usage:")
}

func TestMirrorSelect_AutoAndURL(t *testing.T) {
	assertErrContains(t, run(t, "mirror", "select", "--auto", "https://example.com/"), "cannot combine --auto")
}
