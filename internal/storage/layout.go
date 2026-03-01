package storage

import (
	"path/filepath"
)

// InstallDir returns the directory where a Qt SDK should be installed.
// root is the Qt install root (e.g. C:\Qt), version e.g. "6.10.0", arch e.g. "msvc2022_64".
func InstallDir(root, version, arch string) string {
	return filepath.Join(root, version, arch)
}

// ToolInstallDir returns the directory for a tool.
func ToolInstallDir(toolsRoot, toolName string) string {
	return filepath.Join(toolsRoot, displayNameForTool(toolName))
}

// TempDir returns a temporary extraction directory within the install root.
func TempDir(installDir string) string {
	return installDir + ".qvm-tmp"
}

func displayNameForTool(name string) string {
	names := map[string]string{
		"qtcreator":  "QtCreator",
		"cmake":      "CMake",
		"ifw":        "InstallerFramework",
		"mingw":      "mingw1310_64",
		"llvm_mingw": "llvm-mingw",
		"ninja":      "Ninja",
		"openssl":    "OpenSSL",
		"vcredist":   "vcredist",
	}
	if d, ok := names[name]; ok {
		return d
	}
	return name
}
