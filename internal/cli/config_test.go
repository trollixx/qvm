package cli

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trollixx/qvm/internal/config"
)

func defaultCfg() *config.Config {
	cfg := &config.Config{}
	cfg.Install.Dir = "/some/qt"
	cfg.Install.ToolsDir = ""
	cfg.Repository.URL = "https://download.qt.io/online/qtsdkrepository/"
	cfg.Repository.Mirrors = []string{"https://mirror1.example.com/", "https://mirror2.example.com/"}
	cfg.Download.Concurrency = 4
	cfg.Download.TimeoutSeconds = 300
	return cfg
}

// ── configGet ─────────────────────────────────────────────────────────────────

func TestConfigGet_KnownStringKey(t *testing.T) {
	cfg := defaultCfg()
	val, err := configGet(cfg, "install.dir")
	require.NoError(t, err)
	assert.Equal(t, "/some/qt", val)
}

func TestConfigGet_KnownIntKey(t *testing.T) {
	cfg := defaultCfg()
	val, err := configGet(cfg, "download.concurrency")
	require.NoError(t, err)
	assert.Equal(t, 4, val)
}

func TestConfigGet_KnownSliceKey(t *testing.T) {
	cfg := defaultCfg()
	val, err := configGet(cfg, "repository.mirrors")
	require.NoError(t, err)
	mirrors, ok := val.([]string)
	require.True(t, ok)
	assert.Equal(t, []string{"https://mirror1.example.com/", "https://mirror2.example.com/"}, mirrors)
}

func TestConfigGet_CaseInsensitive(t *testing.T) {
	cfg := defaultCfg()
	val, err := configGet(cfg, "Install.Dir")
	require.NoError(t, err)
	assert.Equal(t, "/some/qt", val)
}

func TestConfigGet_UnknownKey(t *testing.T) {
	_, err := configGet(defaultCfg(), "no.such.key")
	require.Error(t, err)
	assert.ErrorContains(t, err, "unknown config key")
	assert.ErrorContains(t, err, "no.such.key")
}

func TestConfigGet_TimeoutSeconds(t *testing.T) {
	// "download.timeout_seconds" is the canonical key (the old "download.timeout" alias was removed).
	cfg := defaultCfg()
	val, err := configGet(cfg, "download.timeout_seconds")
	require.NoError(t, err)
	assert.Equal(t, 300, val)
}

// ── configSet ─────────────────────────────────────────────────────────────────

func TestConfigSet_String(t *testing.T) {
	cfg := defaultCfg()
	require.NoError(t, configSet(cfg, "install.dir", "/new/qt"))
	assert.Equal(t, "/new/qt", cfg.Install.Dir)
}

func TestConfigSet_Int(t *testing.T) {
	cfg := defaultCfg()
	require.NoError(t, configSet(cfg, "download.concurrency", "8"))
	assert.Equal(t, 8, cfg.Download.Concurrency)
}

func TestConfigSet_Slice_Multiple(t *testing.T) {
	cfg := defaultCfg()
	require.NoError(t, configSet(cfg, "repository.mirrors", "https://a.com/, https://b.com/"))
	assert.Equal(t, []string{"https://a.com/", "https://b.com/"}, cfg.Repository.Mirrors)
}

func TestConfigSet_Slice_Single(t *testing.T) {
	cfg := defaultCfg()
	require.NoError(t, configSet(cfg, "repository.mirrors", "https://only.com/"))
	assert.Equal(t, []string{"https://only.com/"}, cfg.Repository.Mirrors)
}

func TestConfigSet_UnknownKey(t *testing.T) {
	err := configSet(defaultCfg(), "bad.key", "value")
	require.Error(t, err)
	assert.ErrorContains(t, err, "unknown config key")
}

func TestConfigSet_InvalidInt(t *testing.T) {
	err := configSet(defaultCfg(), "download.concurrency", "abc")
	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid integer value")
	assert.ErrorContains(t, err, "abc")
}

func TestConfigSet_RoundTrip(t *testing.T) {
	cfg := defaultCfg()
	require.NoError(t, configSet(cfg, "download.timeout_seconds", "120"))
	val, err := configGet(cfg, "download.timeout_seconds")
	require.NoError(t, err)
	assert.Equal(t, int64(120), int64(val.(int)))
}

// ── configList ────────────────────────────────────────────────────────────────

func TestConfigList_ReturnsAllKeys(t *testing.T) {
	pairs := configList(defaultCfg())
	keys := make([]string, len(pairs))
	for i, p := range pairs {
		keys[i] = p[0]
	}
	sort.Strings(keys)

	// Every key in configKeyMap must appear in the list output.
	for key := range configKeyMap {
		assert.Contains(t, keys, key, "key %q missing from config list", key)
	}
}

func TestConfigList_RequiredValuesPresent(t *testing.T) {
	pairs := configList(defaultCfg())
	byKey := make(map[string]string, len(pairs))
	for _, p := range pairs {
		byKey[p[0]] = p[1]
	}

	// These fields are always set to non-empty defaults.
	required := []string{"install.dir", "repository.url", "download.concurrency", "download.timeout_seconds"}
	for _, key := range required {
		assert.NotEmpty(t, byKey[key], "value for required key %q should not be empty", key)
	}

	// install.tools_dir is intentionally empty (means "derive from install.dir at runtime").
	_, hasDerived := byKey["install.tools_dir"]
	assert.True(t, hasDerived, "install.tools_dir should appear in list even when empty")
}
