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
	// Bare `qvm list` now shows installed versions (no error).
	err := run(t, "list")
	assert.NoError(t, err)
}

func TestList_UnknownTarget(t *testing.T) {
	// Non-numeric args that can't be parsed as a version filter are rejected.
	err := run(t, "list", "notavalidthing")
	assertErrContains(t, err, "invalid version")
}

// -- uninstall -----------------------------------------------------------------

func TestUninstall_NoArg(t *testing.T) {
	assertErrContains(t, run(t, "uninstall"), "missing argument")
}

// Uninstall treats bare arg as a version (stripping optional qt@ prefix).
func TestUninstall_NonVersion(t *testing.T) {
	// "qtcreator" is stripped of "qt" prefix -> version parse will fail downstream,
	// but won't crash - it just won't find anything in the registry.
	err := run(t, "uninstall", "6.99.99", "--yes")
	assertErrContains(t, err, "not installed")
}

// -- info ----------------------------------------------------------------------

func TestInfo_NoArg(t *testing.T) {
	assertErrContains(t, run(t, "info"), "missing argument")
}

func TestInfo_NotInstalled(t *testing.T) {
	// Bare version that isn't installed gives "not installed".
	assertErrContains(t, run(t, "info", "6.99.99"), "not installed")
}

func TestInfo_QtPrefix(t *testing.T) {
	// qt@<version> still works.
	assertErrContains(t, run(t, "info", "qt@6.99.99"), "not installed")
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
