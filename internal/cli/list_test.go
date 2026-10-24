package cli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/trollixx/qvm/internal/repository"
)

func versionInfos(versions ...string) []repository.QtVersionInfo {
	out := make([]repository.QtVersionInfo, len(versions))
	for i, v := range versions {
		out[i] = repository.QtVersionInfo{Version: v}
	}
	return out
}

func versionsOf(vers []repository.QtVersionInfo) []string {
	out := make([]string, len(vers))
	for i, v := range vers {
		out[i] = v.Version
	}
	return out
}

// Versions sort numerically, so 6.10 comes before 6.9.
func TestSortVersionsDesc(t *testing.T) {
	vers := versionInfos("6.8.3", "6.10.0", "6.9.1", "6.10.2", "6.2.4")
	sortVersionsDesc(vers)
	assert.Equal(t, []string{"6.10.2", "6.10.0", "6.9.1", "6.8.3", "6.2.4"}, versionsOf(vers))
}

func TestNewestLTSPatch(t *testing.T) {
	vers := []repository.QtVersionInfo{
		{Version: "6.10.2"},
		{Version: "6.8.3", IsLTS: true},
		{Version: "6.8.2", IsLTS: true},
		{Version: "6.5.3", IsLTS: true},
	}
	assert.Equal(t, "6.8.3", newestLTSPatch(vers), "newest patch of the newest LTS series")

	assert.Empty(t, newestLTSPatch(versionInfos("6.10.2", "6.9.3")), "no LTS versions")
}

func TestPrintVersionRow(t *testing.T) {
	installed := map[string][]string{
		"6.8.3":  {"win64_msvc2022_64"},
		"6.10.2": {"win64_msvc2022_64", "win64_mingw"},
	}
	row := func(v repository.QtVersionInfo, label, recommended string) string {
		a, out := newTestApp()
		a.printVersionRow(v, label, installed, recommended)
		return out.String()
	}

	got := row(repository.QtVersionInfo{
		Version: "6.8.3", ReleaseDate: time.Date(2025, 3, 20, 0, 0, 0, 0, time.UTC),
	}, "LTS", "6.8.3")
	assert.Equal(t, "  6.8.3      LTS              2025-03-20  ✓ installed (win64_msvc2022_64)\n", got,
		"installed wins over recommended")

	assert.Contains(t, row(repository.QtVersionInfo{Version: "6.10.2"}, "", ""), "✓ installed (2 archs)")
	assert.Contains(t, row(repository.QtVersionInfo{Version: "6.8.2"}, "LTS", "6.8.2"), "← recommended")
	assert.NotContains(t, row(repository.QtVersionInfo{Version: "6.9.3"}, "", "6.8.3"), "recommended")
}
