package storage

import (
	"path/filepath"
)

// InstallDir returns the directory where a Qt SDK should be installed.
// root is the Qt install root (e.g. C:\Qt), version e.g. "6.10.0", arch e.g. "msvc2022_64".
func InstallDir(root, version, arch string) string {
	return filepath.Join(root, version, arch)
}

// TempDir returns a temporary extraction directory within the install root.
func TempDir(installDir string) string {
	return installDir + ".qvm-tmp"
}
