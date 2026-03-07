package platform

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

// Darwin implements Platform for macOS.
type Darwin struct{}

func (d *Darwin) DefaultInstallDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/Qt"
	}
	return home + "/Qt"
}

func (d *Darwin) DefaultArch(qtVersion string) string {
	return "macos"
}

func (d *Darwin) CheckCompilerPresent(arch string) (bool, string) {
	// Check for clang via Xcode command line tools.
	path, err := exec.LookPath("clang++")
	if err != nil {
		return false, "clang++ not found; install Xcode Command Line Tools: xcode-select --install"
	}
	out, err := exec.CommandContext(context.Background(), path, "--version").Output()
	if err != nil {
		return true, ""
	}
	ver := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	return true, ver
}
