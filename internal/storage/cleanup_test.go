package storage

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testVersion = "6.7.0"
	testArch    = "win64_msvc2022_64"
)

func TestIsInstallDir(t *testing.T) {
	// Use platform-appropriate absolute paths.
	var root, relative string
	if runtime.GOOS == "windows" {
		root = `D:\Somewhere\Qt`
		relative = `Qt\6.7.0\win64_msvc2022_64`
	} else {
		root = "/opt/somewhere/Qt"
		relative = "Qt/6.7.0/win64_msvc2022_64"
	}

	tests := []struct {
		name    string
		dir     string
		version string
		arch    string
		want    bool
	}{
		{"install under any root", filepath.Join(root, testVersion, testArch), testVersion, testArch, true},
		{"trailing separator", filepath.Join(root, testVersion, testArch) + string(os.PathSeparator),
			testVersion, testArch, true},
		{"root itself", root, testVersion, testArch, false},
		{"version dir", filepath.Join(root, testVersion), testVersion, testArch, false},
		{"other arch", filepath.Join(root, testVersion, "win64_mingw"), testVersion, testArch, false},
		{"other version", filepath.Join(root, "6.8.3", testArch), testVersion, testArch, false},
		{"relative path", relative, testVersion, testArch, false},
		{"empty dir", "", testVersion, testArch, false},
		{"empty version", filepath.Join(root, testVersion, testArch), "", testArch, false},
		{"empty arch", filepath.Join(root, testVersion, testArch), testVersion, "", false},
		{"dot-dot arch", filepath.Join(root, testVersion, ".."), testVersion, "..", false},
		{"arch with separator", filepath.Join(root, testVersion, "a", "b"), testVersion, "a/b", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isInstallDir(tt.dir, tt.version, tt.arch))
		})
	}
}

func TestIsInstallDir_TraversalAttack(t *testing.T) {
	var traversal string
	if runtime.GOOS == "windows" {
		traversal = `C:\Qt\6.7.0\win64_msvc2022_64\..\..\..\Windows\System32`
	} else {
		traversal = "/home/user/Qt/6.7.0/win64_msvc2022_64/../../../../etc"
	}
	assert.False(t, isInstallDir(traversal, testVersion, testArch),
		"a path that only names the version and arch on the way must be rejected")
}

func TestCleanup(t *testing.T) {
	// Set up a temp directory tree that mimics a real install.
	tmpRoot := t.TempDir()
	installDir := InstallDir(tmpRoot, testVersion, testArch)
	require.NoError(t, os.MkdirAll(installDir, 0o755))
	// Put a sentinel file inside to verify removal.
	require.NoError(t, os.WriteFile(filepath.Join(installDir, "bin.txt"), []byte("data"), 0o644))

	mgr := NewRegistryManagerAt(filepath.Join(tmpRoot, "registry.json"))
	require.NoError(t, mgr.AddQt(InstalledQt{
		Version:     testVersion,
		Arch:        testArch,
		InstallDir:  installDir,
		InstalledAt: time.Now().UTC(),
	}))

	require.NoError(t, Cleanup(mgr, testVersion, testArch))

	// Directory should be gone.
	_, err := os.Stat(installDir)
	assert.True(t, os.IsNotExist(err), "install directory should be removed")

	// Registry entry should be gone.
	reg, err := mgr.Load()
	require.NoError(t, err)
	assert.Empty(t, reg.Qt)
}

func TestCleanup_RemovesEmptyVersionParent(t *testing.T) {
	tmpRoot := t.TempDir()
	versionDir := filepath.Join(tmpRoot, testVersion)
	installDir := filepath.Join(versionDir, testArch)
	require.NoError(t, os.MkdirAll(installDir, 0o755))

	mgr := NewRegistryManagerAt(filepath.Join(t.TempDir(), "registry.json"))
	require.NoError(t, mgr.AddQt(InstalledQt{
		Version:     testVersion,
		Arch:        testArch,
		InstallDir:  installDir,
		InstalledAt: time.Now().UTC(),
	}))

	require.NoError(t, Cleanup(mgr, testVersion, testArch))

	// Empty parent (the "6.7.0" dir) should also be removed.
	_, err := os.Stat(versionDir)
	assert.True(t, os.IsNotExist(err), "empty version directory should be removed")

	// But the install root must remain.
	_, err = os.Stat(tmpRoot)
	require.NoError(t, err, "install root must be preserved")
}

func TestCleanup_KeepsParentWhenNotEmpty(t *testing.T) {
	tmpRoot := t.TempDir()
	versionDir := filepath.Join(tmpRoot, testVersion)
	installA := filepath.Join(versionDir, testArch)
	installB := filepath.Join(versionDir, "win64_mingw")
	require.NoError(t, os.MkdirAll(installA, 0o755))
	require.NoError(t, os.MkdirAll(installB, 0o755))

	mgr := NewRegistryManagerAt(filepath.Join(t.TempDir(), "registry.json"))
	require.NoError(t, mgr.AddQt(InstalledQt{
		Version:    testVersion,
		Arch:       testArch,
		InstallDir: installA,
	}))
	require.NoError(t, mgr.AddQt(InstalledQt{
		Version:    testVersion,
		Arch:       "win64_mingw",
		InstallDir: installB,
	}))

	require.NoError(t, Cleanup(mgr, testVersion, testArch))

	// Sibling arch dir still on disk → version dir must remain.
	_, err := os.Stat(versionDir)
	require.NoError(t, err, "version dir must remain because a sibling arch is still present")
	_, err = os.Stat(installB)
	require.NoError(t, err)
}

func TestCleanup_NotRegistered(t *testing.T) {
	tmpRoot := t.TempDir()
	mgr := NewRegistryManagerAt(filepath.Join(tmpRoot, "registry.json"))

	err := Cleanup(mgr, "6.99.0", testArch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestCleanup_RejectsUnsafePath(t *testing.T) {
	tmpRoot := t.TempDir()
	outsideDir := t.TempDir() // not laid out as <root>/<version>/<arch>

	mgr := NewRegistryManagerAt(filepath.Join(tmpRoot, "registry.json"))

	// Manually save a registry entry pointing at an unrelated directory.
	reg := &Registry{
		Version: 1,
		Qt: []InstalledQt{
			{
				Version:     testVersion,
				Arch:        testArch,
				InstallDir:  outsideDir,
				InstalledAt: time.Now().UTC(),
			},
		},
	}
	require.NoError(t, mgr.Save(reg))

	err := Cleanup(mgr, testVersion, testArch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a qvm install directory")

	// The unrelated directory must still exist.
	_, statErr := os.Stat(outsideDir)
	assert.NoError(t, statErr, "unrelated directory must not be deleted")
}
