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
		err = os.RemoveAll(q.InstallDir)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing %s: %w", q.InstallDir, err)
		}
	}

	return reg.RemoveQt(version, arch)
}

