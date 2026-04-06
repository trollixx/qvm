package platform

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/trollixx/qvm/pkg/qtmeta"
)

// Windows implements Platform for Windows.
type Windows struct{}

func (w *Windows) DefaultInstallDir() string {
	return `C:\Qt`
}

func (w *Windows) DefaultArch(qtVersion string) string {
	v, err := qtmeta.ParseVersion(qtVersion)
	if err != nil {
		return archWin64MSVC2019
	}
	if v.GTE(qtmeta.MustParseVersion(qt66)) {
		return archWin64MSVC2022
	}
	// Qt 6.0-6.5: both msvc2019 and msvc2022 exist; prefer what's installed.
	if hasMSVC("2019") {
		return archWin64MSVC2019
	}
	if hasMSVC("2022") {
		return archWin64MSVC2022
	}
	return archWin64MSVC2019
}

func (w *Windows) CheckCompilerPresent(arch string) (bool, string) {
	switch {
	case strings.Contains(arch, "msvc2022"):
		if hasMSVC("2022") {
			return true, "MSVC 2022 detected"
		}
		return false, "MSVC 2022 not found. Install Visual Studio 2022 or Build Tools."
	case strings.Contains(arch, "msvc2019"):
		if hasMSVC("2019") {
			return true, "MSVC 2019 detected"
		}
		return false, "MSVC 2019 not found. Install Visual Studio 2019 or Build Tools."
	case strings.Contains(arch, "mingw"):
		path, err := exec.LookPath("g++")
		if err != nil {
			return false, "MinGW g++ not found in PATH"
		}
		return true, "MinGW found at " + path
	}
	return true, ""
}

// hasMSVC checks whether any MSVC component for the given year is installed
// by querying vswhere with a specific version range.
func hasMSVC(year string) bool {
	vswhere := `C:\Program Files (x86)\Microsoft Visual Studio\Installer\vswhere.exe`

	// Use precise version ranges instead of string matching.
	var versionRange string
	switch year {
	case "2022":
		versionRange = "[17.0,)"
	case "2019":
		versionRange = "[16.0,17.0)"
	default:
		return false
	}

	out, err := exec.CommandContext(context.Background(), vswhere, "-version", versionRange, "-property", "installationPath").
		Output()
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		return true
	}

	// Fallback: check if cl.exe is on PATH.
	_, err = exec.LookPath("cl.exe")
	return err == nil
}

// VCRedistPresent checks whether the Visual C++ redistributable is installed.
func VCRedistPresent(year string) bool {
	// Check registry or known DLL presence.
	// Simplified check: look for vcruntime DLL in system32.
	dlls := map[string]string{
		"2022": `C:\Windows\System32\vcruntime140.dll`,
		"2019": `C:\Windows\System32\vcruntime140.dll`,
	}
	dll, ok := dlls[year]
	if !ok {
		return true
	}
	_, err := os.Stat(dll)
	return err == nil
}
