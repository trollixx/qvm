package cli

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// run is a helper that invokes the app with the given args and returns the error.
// All output is captured in-memory; nothing leaks to real stdout/stderr.
func run(t *testing.T, args ...string) error {
	t.Helper()
	streams, _, _ := NewTestIOStreams()
	return newRootCommand(streams).Run(context.Background(), append([]string{"qvm"}, args...))
}

// newTestApp creates an app backed by in-memory buffers for use in unit tests.
func newTestApp() (*app, *bytes.Buffer) {
	streams, out, _ := NewTestIOStreams()
	return &app{streams: streams}, out
}

// assertErrContains is a convenience that requires an error and checks its message.
func assertErrContains(t *testing.T, err error, substr string) {
	t.Helper()
	require.Error(t, err)
	require.ErrorContains(t, err, substr)
}

// -- install -------------------------------------------------------------------

func TestInstall_NoArg(t *testing.T) {
	assertErrContains(t, run(t, "install"), "specify a version")
}

// "qt" alone must give a version-hint error.
func TestInstall_QtNoVersion(t *testing.T) {
	err := run(t, "install", "qt")
	assertErrContains(t, err, "specify a version")
}

// -- list ----------------------------------------------------------------------

func TestList_NoArg(t *testing.T) {
	// Bare `qvm list` shows installed versions (no error).
	err := run(t, "list")
	assert.NoError(t, err)
}

// -- uninstall -----------------------------------------------------------------

func TestUninstall_NoArg(t *testing.T) {
	assertErrContains(t, run(t, "uninstall"), "missing argument")
}

func TestUninstall_NotInstalled(t *testing.T) {
	err := run(t, "uninstall", "6.99.99", "--yes")
	assertErrContains(t, err, "not installed")
}

// -- info ----------------------------------------------------------------------

func TestInfo_NoArg(t *testing.T) {
	assertErrContains(t, run(t, "info"), "missing argument")
}

func TestInfo_NotInstalled(t *testing.T) {
	assertErrContains(t, run(t, "info", "6.99.99"), "not installed")
}

// -- search --------------------------------------------------------------------

func TestSearch_NoArg(t *testing.T) {
	assertErrContains(t, run(t, "search"), "missing argument")
}

// -- config --------------------------------------------------------------------

func TestConfigGet_NoKey(t *testing.T) {
	assertErrContains(t, run(t, "config", "get"), "argument required")
}

func TestConfigGet_UnknownKey_ViaApp(t *testing.T) {
	err := run(t, "config", "get", "nonexistent.key")
	assertErrContains(t, err, "unknown config key")
	assertErrContains(t, err, "nonexistent.key")
}

func TestConfigSet_NoArgs(t *testing.T) {
	assertErrContains(t, run(t, "config", "set"), "key and value required")
}

func TestConfigSet_KeyOnly(t *testing.T) {
	assertErrContains(t, run(t, "config", "set", "install.dir"), "key and value required")
}

func TestConfigSet_UnknownKey_ViaApp(t *testing.T) {
	err := run(t, "config", "set", "bad.key", "value")
	assertErrContains(t, err, "unknown config key")
	assertErrContains(t, err, "bad.key")
}

func TestConfigSet_InvalidInt_ViaApp(t *testing.T) {
	err := run(t, "config", "set", "download.concurrency", "notanumber")
	assertErrContains(t, err, "invalid integer value")
	assertErrContains(t, err, "notanumber")
}

// -- prefix --------------------------------------------------------------------

func TestPrefix_NoArg(t *testing.T) {
	assertErrContains(t, run(t, "prefix"), "missing argument")
}

func TestPrefix_NotInstalled(t *testing.T) {
	err := run(t, "prefix", "6.99.99")
	assertErrContains(t, err, "is not installed")
}

func TestPrefix_NotInstalledWithArch(t *testing.T) {
	err := run(t, "prefix", "6.99.99", "--arch", "win64_msvc2022_64")
	assertErrContains(t, err, "is not installed")
}

// -- cache ---------------------------------------------------------------------

func TestCache_List(t *testing.T) {
	// Listing the cache must not error even when the cache is empty.
	assert.NoError(t, run(t, "cache", "list"))
}

func TestCache_CleanIncompleteEmpty(t *testing.T) {
	// Cleaning incomplete files in an empty cache must not error.
	assert.NoError(t, run(t, "cache", "clean", "--incomplete", "--yes"))
}

// -- mirror --------------------------------------------------------------------

func TestMirror_SelectNoArg(t *testing.T) {
	assertErrContains(t, run(t, "mirror", "select"), "specify --auto or a URL")
}

func TestMirror_SelectAutoAndURLConflict(t *testing.T) {
	assertErrContains(t, run(t, "mirror", "select", "--auto", "https://example.com/"),
		"cannot combine --auto with a URL")
}

// -- list-remote ---------------------------------------------------------------

func TestListRemote_InvalidVersionFilter(t *testing.T) {
	// "abc" is not a valid version.
	err := run(t, "list-remote", "abc")
	assertErrContains(t, err, "invalid version")
}

// -- doctor --------------------------------------------------------------------

func TestDoctor_Runs(t *testing.T) {
	// Doctor should run without error in a clean environment (no installs).
	assert.NoError(t, run(t, "doctor"))
}

// -- --target ------------------------------------------------------------------

func TestInstall_RejectsUnknownTarget(t *testing.T) {
	err := run(t, "install", "6.10.2", "--target", "winrt")
	assertErrContains(t, err, "unknown target")
}
