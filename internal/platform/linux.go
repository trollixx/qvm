package platform

import (
	"context"
	"os"
	"os/exec"
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

func (l *Linux) DefaultArch(qtVersion string) string {
	return "gcc_64"
}

func (l *Linux) CheckCompilerPresent(arch string) (bool, string) {
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
