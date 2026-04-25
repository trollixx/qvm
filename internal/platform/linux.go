package platform

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Linux implements Platform for Linux.
type Linux struct{}

func (l *Linux) DefaultInstallDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/Qt"
	}
	return home + "/Qt"
}

func (l *Linux) DefaultArch(_ string) string {
	if runtime.GOARCH == "arm64" {
		return "linux_gcc_arm64"
	}
	return "gcc_64"
}

func (l *Linux) CheckCompilerPresent(_ string) (bool, string) {
	// Check for GCC.
	path, err := exec.LookPath("gcc")
	if err != nil {
		return false, "GCC not found in PATH; install via your package manager (e.g. apt install g++)"
	}
	// Get version.
	out, err := exec.CommandContext(context.Background(), path, "--version").Output()
	if err != nil {
		return true, ""
	}
	ver := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	return true, ver
}
