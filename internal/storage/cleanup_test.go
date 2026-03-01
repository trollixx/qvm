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

func TestIsSafePath(t *testing.T) {
	// Use platform-appropriate absolute paths.
	var root, child, outside, relative string
	if runtime.GOOS == "windows" {
		root = `C:\Qt`
		child = `C:\Qt\6.7.0\msvc2022_64`
		outside = `D:\Other\dir`
		relative = `Qt\6.7.0`
	} else {
		root = "/home/user/Qt"
		child = "/home/user/Qt/6.7.0/gcc_64"
		outside = "/tmp/other"
		relative = "Qt/6.7.0"
	}

	tests := []struct {
		name       string
		installDir string
		root       string
		want       bool
	}{
		{
			name:       "valid subdirectory",
			installDir: child,
			root:       root,
			want:       true,
		},
		{
			name:       "root itself is safe",
			installDir: root,
			root:       root,
			want:       true,
		},
		{
			name:       "empty installDir",
			installDir: "",
			root:       root,
			want:       false,
		},
		{
			name:       "empty root",
			installDir: child,
			root:       "",
			want:       false,
		},
		{
			name:       "both empty",
			installDir: "",
			root:       "",
			want:       false,
		},
		{
			name:       "relative path rejected",
			installDir: relative,
			root:       root,
			want:       false,
		},
		{
			name:       "outside root",
			installDir: outside,
			root:       root,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isSafePath(tt.installDir, tt.root))
		})
	}
}

func TestIsSafePath_TraversalAttack(t *testing.T) {
	var root, traversal string
	if runtime.GOOS == "windows" {
		root = `C:\Qt`
		traversal = `C:\Qt\..\Windows\System32`
	} else {
		root = "/home/user/Qt"
		traversal = "/home/user/Qt/../../etc/passwd"
	}
	assert.False(t, isSafePath(traversal, root),
		"path traversal outside root must be rejected")
}

func TestCleanup(t *testing.T) {
	// Set up a temp directory tree that mimics a real install.
	tmpRoot := t.TempDir()
	installDir := filepath.Join(tmpRoot, "6.7.0", "msvc2022_64")
	require.NoError(t, os.MkdirAll(installDir, 0o755))
	// Put a sentinel file inside to verify removal.
	require.NoError(t, os.WriteFile(filepath.Join(installDir, "bin.txt"), []byte("data"), 0o644))

	mgr := NewRegistryManagerAt(filepath.Join(tmpRoot, "registry.json"))
	require.NoError(t, mgr.AddQt(InstalledQt{
		Version:     "6.7.0",
		Arch:        "win64_msvc2022_64",
		InstallDir:  installDir,
		InstalledAt: time.Now().UTC(),
	}))

	require.NoError(t, Cleanup(mgr, "6.7.0", "win64_msvc2022_64", tmpRoot))

	// Directory should be gone.
	_, err := os.Stat(installDir)
	assert.True(t, os.IsNotExist(err), "install directory should be removed")

	// Registry entry should be gone.
	reg, err := mgr.Load()
	require.NoError(t, err)
	assert.Empty(t, reg.Qt)
}

func TestCleanup_NotRegistered(t *testing.T) {
	tmpRoot := t.TempDir()
	mgr := NewRegistryManagerAt(filepath.Join(tmpRoot, "registry.json"))

	err := Cleanup(mgr, "6.99.0", "win64_msvc2022_64", tmpRoot)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestCleanup_RejectsUnsafePath(t *testing.T) {
	tmpRoot := t.TempDir()
	outsideDir := t.TempDir() // separate temp dir, outside tmpRoot

	mgr := NewRegistryManagerAt(filepath.Join(tmpRoot, "registry.json"))

	// Manually save a registry entry pointing outside the root.
	reg := &Registry{
		Version: 1,
		Qt: []InstalledQt{
			{
				Version:     "6.7.0",
				Arch:        "bad_arch",
				InstallDir:  outsideDir,
				InstalledAt: time.Now().UTC(),
			},
		},
	}
	require.NoError(t, mgr.Save(reg))

	err := Cleanup(mgr, "6.7.0", "bad_arch", tmpRoot)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside expected root")

	// The outside directory must still exist.
	_, statErr := os.Stat(outsideDir)
	assert.NoError(t, statErr, "directory outside root must not be deleted")
}

func TestCleanupTool(t *testing.T) {
	tmpRoot := t.TempDir()
	installDir := filepath.Join(tmpRoot, "Tools", "QtCreator")
	require.NoError(t, os.MkdirAll(installDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(installDir, "qtcreator.exe"), []byte("exe"), 0o644))

	mgr := NewRegistryManagerAt(filepath.Join(tmpRoot, "registry.json"))
	require.NoError(t, mgr.AddTool(InstalledTool{
		Name:        "qtcreator",
		Version:     "13.0.0",
		InstallDir:  installDir,
		InstalledAt: time.Now().UTC(),
	}))

	require.NoError(t, CleanupTool(mgr, "qtcreator", "13.0.0", tmpRoot))

	_, err := os.Stat(installDir)
	assert.True(t, os.IsNotExist(err), "tool directory should be removed")

	reg, err := mgr.Load()
	require.NoError(t, err)
	assert.Empty(t, reg.Tools)
}

func TestCleanupTool_NotRegistered(t *testing.T) {
	tmpRoot := t.TempDir()
	mgr := NewRegistryManagerAt(filepath.Join(tmpRoot, "registry.json"))

	err := CleanupTool(mgr, "nonexistent", "1.0.0", tmpRoot)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestCleanupTool_RejectsUnsafePath(t *testing.T) {
	tmpRoot := t.TempDir()
	outsideDir := t.TempDir()

	mgr := NewRegistryManagerAt(filepath.Join(tmpRoot, "registry.json"))
	reg := &Registry{
		Version: 1,
		Tools: []InstalledTool{
			{
				Name:        "badtool",
				Version:     "1.0.0",
				InstallDir:  outsideDir,
				InstalledAt: time.Now().UTC(),
			},
		},
	}
	require.NoError(t, mgr.Save(reg))

	err := CleanupTool(mgr, "badtool", "1.0.0", tmpRoot)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside expected root")

	_, statErr := os.Stat(outsideDir)
	assert.NoError(t, statErr, "directory outside root must not be deleted")
}
