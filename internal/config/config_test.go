package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/adrg/xdg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isolate points the config file at a temp directory and returns its path.
func isolate(t *testing.T) string {
	t.Helper()
	// Registered first, so it runs after Setenv restores the environment.
	t.Cleanup(xdg.Reload)
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	xdg.Reload()
	return filepath.Join(dir, "qvm", "config.toml")
}

func writeConfig(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func TestPath(t *testing.T) {
	want := isolate(t)
	got, err := Path()
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestLoad_NoFileGivesDefaults(t *testing.T) {
	isolate(t)
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, defaults(), cfg)
	assert.Equal(t, DefaultInstallDir(), cfg.Install.Dir)
	assert.Equal(t, DefaultRepositoryURL, cfg.Repository.URL)
	assert.Equal(t, 4, cfg.Download.Concurrency)
	assert.Equal(t, 300, cfg.Download.TimeoutSeconds)
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	path := isolate(t)
	cfg := defaults()
	cfg.Qt.Default = "6.8.3"
	cfg.Install.Dir = filepath.Join(t.TempDir(), "Qt")
	cfg.Repository.URL = "https://mirror.example.com/qt/"
	cfg.Repository.Mirrors = []string{"https://a.example.com/", "https://b.example.com/"}
	cfg.Repository.Blacklist = []string{"https://slow.example.com/"}
	cfg.Download.Concurrency = 8
	cfg.Download.TimeoutSeconds = 60

	require.NoError(t, Save(cfg))
	_, err := os.Stat(path)
	require.NoError(t, err, "Save creates the config directory and file")

	loaded, err := Load()
	require.NoError(t, err)
	assert.Equal(t, cfg, loaded)
}

// Keys left out or set to zero get their defaults; the others are kept.
func TestLoad_FillsBlanks(t *testing.T) {
	path := isolate(t)
	writeConfig(t, path, `
[qt]
default = "6.8.3"

[install]
dir = ""

[repository]
mirrors = ["https://a.example.com/"]

[download]
concurrency = 0
`)

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "6.8.3", cfg.Qt.Default)
	assert.Equal(t, []string{"https://a.example.com/"}, cfg.Repository.Mirrors)
	assert.Equal(t, DefaultInstallDir(), cfg.Install.Dir)
	assert.Equal(t, DefaultRepositoryURL, cfg.Repository.URL)
	assert.Equal(t, DefaultConcurrency(), cfg.Download.Concurrency)
	assert.Equal(t, DefaultTimeoutSeconds(), cfg.Download.TimeoutSeconds)
}

func TestLoad_InvalidFile(t *testing.T) {
	path := isolate(t)
	writeConfig(t, path, "[install\ndir = ")

	_, err := Load()
	assert.Error(t, err)
}

func TestDefaultInstallDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		assert.Equal(t, `C:\Qt`, DefaultInstallDir())
		return
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	assert.Equal(t, home+"/Qt", DefaultInstallDir())
}
