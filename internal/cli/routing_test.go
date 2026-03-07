package cli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// run is a helper that invokes the app with the given args and returns the error.
func run(t *testing.T, args ...string) error {
	t.Helper()
	return NewApp().Run(context.Background(), append([]string{"qvm"}, args...))
}

// assertErrContains is a convenience that requires an error and checks its message.
func assertErrContains(t *testing.T, err error, substr string) {
	t.Helper()
	require.Error(t, err)
	require.ErrorContains(t, err, substr)
}

// -- install -------------------------------------------------------------------

func TestInstall_NoArg(t *testing.T) {
	assertErrContains(t, run(t, "install"), "missing argument")
}

// "qt" alone must give a version-hint error,
// not a 404 trying to fetch tools_qt/Updates.xml.
func TestInstall_QtNoVersion(t *testing.T) {
	err := run(t, "install", "qt")
	assertErrContains(t, err, "specify a version")
	assertErrContains(t, err, "qt@")
}

// Tool with no @version must require a version.
func TestInstall_Tool_NoVersion(t *testing.T) {
	err := run(t, "install", "qtcreator")
	assertErrContains(t, err, "version")
	assertErrContains(t, err, "qtcreator")
}

// "qt" must not be treated as a tool: verify the error is about version
// selection, not a 404 fetching tools_qt/Updates.xml.
func TestInstall_QtRouting_NotTreatedAsTool(t *testing.T) {
	err := run(t, "install", "qt")
	require.Error(t, err)
	// Must NOT mention tools_qt or Updates.xml.
	assert.NotContains(t, err.Error(), "tools_qt")
	assert.NotContains(t, err.Error(), "Updates.xml")
}

// -- list ----------------------------------------------------------------------

func TestList_NoArg(t *testing.T) {
	// Bare `qvm list` now shows installed versions (no error).
	err := run(t, "list")
	assert.NoError(t, err)
}

func TestList_UnknownTarget(t *testing.T) {
	err := run(t, "list", "notavalidthing")
	assertErrContains(t, err, "unknown list target")
	assertErrContains(t, err, "notavalidthing")
}

// -- uninstall -----------------------------------------------------------------

func TestUninstall_NoArg(t *testing.T) {
	assertErrContains(t, run(t, "uninstall"), "missing argument")
}

// Tool uninstall requires @version.
func TestUninstall_Tool_NoVersion(t *testing.T) {
	// "qtcreator" without @version should error, not attempt removal.
	err := run(t, "uninstall", "qtcreator", "--yes")
	assertErrContains(t, err, "version")
}

// -- info ----------------------------------------------------------------------

func TestInfo_NoArg(t *testing.T) {
	assertErrContains(t, run(t, "info"), "missing argument")
}

func TestInfo_NotQtAt(t *testing.T) {
	// Non-qt@ args are treated as tool lookups; with empty registry, "not installed".
	assertErrContains(t, run(t, "info", "something"), "not installed")
}

func TestInfo_QueueOnly(t *testing.T) {
	// "qt" without @version is treated as a tool lookup, not Qt SDK.
	assertErrContains(t, run(t, "info", "qt"), "not installed")
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
