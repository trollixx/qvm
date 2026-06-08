package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseExecArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantArch    string
		wantVersion string
		wantChild   []string
		wantHelp    bool
	}{
		{
			"version then command",
			[]string{"6.8.3", "--", "qmake", "-v"},
			"",
			"6.8.3",
			[]string{"qmake", "-v"},
			false,
		},
		{
			"no version, separator",
			[]string{"--", "qmake", "-v"},
			"",
			"",
			[]string{"qmake", "-v"},
			false,
		},
		{
			"no separator is all command",
			[]string{"qmake", "-v"},
			"",
			"",
			[]string{"qmake", "-v"},
			false,
		},
		{
			"version without separator is the command",
			[]string{"6.8.3", "qmake"},
			"",
			"",
			[]string{"6.8.3", "qmake"},
			false,
		},
		{
			"arch flag then version",
			[]string{"-a", "win64", "6.8.3", "--", "cmake"},
			"win64",
			"6.8.3",
			[]string{"cmake"},
			false,
		},
		{
			"arch flag without separator",
			[]string{"-a", "win64", "qmake"},
			"win64",
			"",
			[]string{"qmake"},
			false,
		},
		{
			"arch flag without separator, child args",
			[]string{"--arch=win64", "cmake", "-S", "."},
			"win64",
			"",
			[]string{"cmake", "-S", "."},
			false,
		},
		{
			"long arch flag, no version",
			[]string{"--arch", "win64", "--", "cmake"},
			"win64",
			"",
			[]string{"cmake"},
			false,
		},
		{
			"arch equals form",
			[]string{"--arch=win64", "6.8.3", "--", "cmake"},
			"win64",
			"6.8.3",
			[]string{"cmake"},
			false,
		},
		{"help flag", []string{"--help"}, "", "", nil, true},
		{"short help flag", []string{"-h"}, "", "", nil, true},
		{"help before separator", []string{"--help", "--", "qmake"}, "", "", nil, true},
		{"empty", nil, "", "", nil, false},
		{"separator with no command", []string{"6.8.3", "--"}, "", "6.8.3", []string{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseExecArgs(tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.wantArch, got.arch)
			assert.Equal(t, tt.wantVersion, got.version)
			assert.Equal(t, tt.wantChild, got.child)
			assert.Equal(t, tt.wantHelp, got.help)
		})
	}
}

func TestParseExecArgs_Errors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			"two tokens before separator",
			[]string{"6.8.3", "extra", "--", "qmake"},
			"unexpected arguments before '--'",
		},
		{"arch flag missing value", []string{"-a", "--", "qmake"}, "missing value for -a"},
		{"empty long arch value", []string{"--arch=", "--", "qmake"}, "missing value for --arch"},
		{"empty short arch value", []string{"-a=", "--", "qmake"}, "missing value for -a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseExecArgs(tt.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestMergedEnv_OverridesParentValue(t *testing.T) {
	t.Setenv("QVM_TEST_MERGE", "parent")
	env := mergedEnv(map[string]string{"QVM_TEST_MERGE": "child"})

	assert.Contains(t, env, "QVM_TEST_MERGE=child")
	assert.NotContains(t, env, "QVM_TEST_MERGE=parent")
}

func TestMergedEnv_AddsNewVariable(t *testing.T) {
	env := mergedEnv(map[string]string{"QVM_TEST_NEW_VAR": "value"})
	assert.Contains(t, env, "QVM_TEST_NEW_VAR=value")
}

func TestMergedEnv_PreservesUnrelatedVariables(t *testing.T) {
	t.Setenv("QVM_TEST_KEEP", "kept")
	env := mergedEnv(map[string]string{"QVM_TEST_OTHER": "x"})
	assert.Contains(t, env, "QVM_TEST_KEEP=kept")
}

func TestMergedEnv_CaseInsensitiveOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only key folding")
	}
	t.Setenv("Qvm_Test_Case", "parent")
	env := mergedEnv(map[string]string{"QVM_TEST_CASE": "child"})

	assert.Contains(t, env, "QVM_TEST_CASE=child")
	for _, e := range env {
		assert.False(
			t,
			strings.HasPrefix(e, "Qvm_Test_Case="),
			"old-cased entry must be replaced: %s",
			e,
		)
	}
}

func TestEnvValue(t *testing.T) {
	env := []string{"FOO=bar", "Path=C:\\bin"}
	assert.Equal(t, "bar", envValue(env, "FOO"))
	assert.Empty(t, envValue(env, "MISSING"))
	if runtime.GOOS == "windows" {
		assert.Equal(t, "C:\\bin", envValue(env, "PATH"), "lookup is case-insensitive on Windows")
	} else {
		assert.Empty(t, envValue(env, "PATH"), "lookup is case-sensitive elsewhere")
	}
}

func TestRunChild_PropagatesExitCode(t *testing.T) {
	args := []string{"sh", "-c", "exit 5"}
	if runtime.GOOS == "windows" {
		args = []string{"cmd", "/c", "exit 5"}
	}
	code, err := runChild(t.Context(), args, os.Environ())
	require.NoError(t, err)
	assert.Equal(t, 5, code)
}

func TestRunChild_SignaledChildExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("signal termination is a Unix concept")
	}
	// SIGKILL'ed child: shells report 128+signal (137), not os.Exit(-1)'s 255.
	code, err := runChild(t.Context(), []string{"sh", "-c", "kill -9 $$"}, os.Environ())
	require.NoError(t, err)
	assert.Equal(t, 137, code)
}

// writeTestExecutable creates an executable file and returns its full path.
func writeTestExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755))
	return path
}

func TestLookInEnvPath_FindsExecutable(t *testing.T) {
	dir := t.TempDir()
	want := writeTestExecutable(t, dir, "mytool")

	empty := t.TempDir()
	pathEnv := empty + string(os.PathListSeparator) + dir
	got, err := lookInEnvPath("mytool", pathEnv, "")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestLookInEnvPath_PathLikeNamesPassThrough(t *testing.T) {
	got, err := lookInEnvPath(`.\local\tool`, "", "")
	require.NoError(t, err)
	assert.Equal(t, `.\local\tool`, got)

	got, err = lookInEnvPath("./tool", "", "")
	require.NoError(t, err)
	assert.Equal(t, "./tool", got)
}

func TestLookInEnvPath_NotFound(t *testing.T) {
	_, err := lookInEnvPath("definitely-missing-tool", t.TempDir(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in PATH")
}

func TestLookInEnvPath_SkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	name := "mytool"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	require.NoError(t, os.Mkdir(filepath.Join(dir, name), 0o755))

	_, err := lookInEnvPath("mytool", dir, "")
	require.Error(t, err)
}

func TestLookInEnvPath_WindowsPathExt(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PATHEXT is Windows-only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "build.cmd")
	require.NoError(t, os.WriteFile(path, []byte("@echo off\n"), 0o644))

	got, err := lookInEnvPath("build", dir, ".COM;.EXE;.BAT;.CMD")
	require.NoError(t, err)
	assert.Equal(t, path, got)
}

func TestLookInEnvPath_UnixRequiresExecutableBit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no executable bit on Windows")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mytool"), []byte("data"), 0o644))

	_, err := lookInEnvPath("mytool", dir, "")
	require.Error(t, err)
}
