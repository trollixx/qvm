package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// isSafePath validates that installDir is a non-empty absolute path that resides
// under expectedRoot. This prevents accidental removal of unrelated directories.
func isSafePath(installDir, expectedRoot string) bool {
	if installDir == "" || expectedRoot == "" {
		return false
	}
	if !filepath.IsAbs(installDir) {
		return false
	}
	cleanDir := filepath.Clean(installDir)
	cleanRoot := filepath.Clean(expectedRoot)
	return cleanDir == cleanRoot || strings.HasPrefix(cleanDir, cleanRoot+string(os.PathSeparator))
}

// Cleanup removes an installed Qt version directory and updates the registry.
// expectedRoot is the Qt install root directory (e.g. C:\Qt); only paths under
// this root will be removed.
func Cleanup(reg *RegistryManager, version, arch, expectedRoot string) error {
	r, err := reg.Load()
	if err != nil {
		return err
	}

	var toRemove []InstalledQt
	for _, q := range r.Qt {
		if q.Version == version && (arch == "" || q.Arch == arch) {
			toRemove = append(toRemove, q)
		}
	}
	if len(toRemove) == 0 {
		return fmt.Errorf("Qt %s (arch: %s) is not registered", version, arch)
	}

	for _, q := range toRemove {
		if !isSafePath(q.InstallDir, expectedRoot) {
			return fmt.Errorf("refusing to remove %s: path is outside expected root %s", q.InstallDir, expectedRoot)
		}
		if err := os.RemoveAll(q.InstallDir); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing %s: %w", q.InstallDir, err)
		}
	}

	return reg.RemoveQt(version, arch)
}

// CleanupTool removes an installed tool directory and updates the registry.
// expectedRoot is the tools root directory; only paths under this root will be removed.
func CleanupTool(reg *RegistryManager, name, version, expectedRoot string) error {
	r, err := reg.Load()
	if err != nil {
		return err
	}

	var toRemove []InstalledTool
	for _, t := range r.Tools {
		if t.Name == name && (version == "" || t.Version == version) {
			toRemove = append(toRemove, t)
		}
	}
	if len(toRemove) == 0 {
		return fmt.Errorf("tool %s@%s is not registered", name, version)
	}

	for _, t := range toRemove {
		if !isSafePath(t.InstallDir, expectedRoot) {
			return fmt.Errorf("refusing to remove %s: path is outside expected root %s", t.InstallDir, expectedRoot)
		}
		if err := os.RemoveAll(t.InstallDir); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing %s: %w", t.InstallDir, err)
		}
	}

	return reg.RemoveTool(name, version)
}
