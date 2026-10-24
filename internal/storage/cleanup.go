package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// isInstallDir reports whether dir is where qvm installs version and arch: an
// absolute path ending in <version>/<arch>, as [InstallDir] makes it, under any
// root. This keeps a bad registry entry from removing an unrelated directory,
// while installs made with --dir, or before install.dir changed, still qualify.
func isInstallDir(dir, version, arch string) bool {
	if !isPathElem(version) || !isPathElem(arch) || !filepath.IsAbs(dir) {
		return false
	}
	clean := filepath.Clean(dir)
	return filepath.Base(clean) == arch && filepath.Base(filepath.Dir(clean)) == version
}

// isPathElem reports whether s is a single path element other than . and ..
func isPathElem(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, `/\`)
}

// Cleanup removes an installed Qt version directory and updates the registry.
// Only directories laid out as qvm installs them are removed.
func Cleanup(reg *RegistryManager, version, arch string) error {
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
		if !isInstallDir(q.InstallDir, q.Version, q.Arch) {
			return fmt.Errorf("refusing to remove %s: not a qvm install directory for Qt %s (%s)",
				q.InstallDir, q.Version, q.Arch)
		}
		err = os.RemoveAll(q.InstallDir)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing %s: %w", q.InstallDir, err)
		}
		// Best-effort: remove the now-empty version directory (e.g. C:\Qt\6.8.3)
		// if no other archs of this version remain. os.Remove only succeeds when
		// the directory is empty, so this is safe.
		_ = os.Remove(filepath.Dir(filepath.Clean(q.InstallDir)))
	}

	return reg.RemoveQt(version, arch)
}
