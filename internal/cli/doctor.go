package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/trollixx/qvm/internal/config"
	"github.com/trollixx/qvm/internal/platform"
	"github.com/trollixx/qvm/internal/repository"
	"github.com/trollixx/qvm/internal/storage"
)

const (
	checkOK   = "\u2713"
	checkWarn = "\u26a0"
	checkFail = "\u2717"
)

func newDoctorCommand() *cli.Command {
	return &cli.Command{
		Name:   "doctor",
		Usage:  "Run environment health checks",
		Action: runDoctor,
	}
}

func runDoctor(ctx context.Context, cmd *cli.Command) error {
	_ = ctx
	_ = cmd

	fmt.Fprintf(os.Stdout, "qvm v%s   %s\n\n", Version, osDescription())

	// Load config once for all checks.
	cfg, cfgErr := config.Load()
	if cfgErr != nil {
		printCheck(checkFail, "Config file", "error: "+cfgErr.Error())
	} else {
		printCheck(checkOK, "Config file", "readable")
	}

	// 2. Metadata cache age.
	checkMetadataCache()

	// 3. Disk space.
	checkDiskSpace(cfg)

	// 4. Qt install dir.
	if cfgErr == nil {
		checkInstallDir(cfg)
	}

	fmt.Fprintln(os.Stdout)

	// 5. Registered Qt installations.
	fmt.Fprintln(os.Stdout, "Installations")
	registry, regErr := storage.NewRegistryManager()
	if regErr != nil {
		printCheck(checkFail, "registry", "could not open registry: "+regErr.Error())
		return nil
	}

	reg, loadErr := registry.Load()
	if loadErr != nil {
		printCheck(checkFail, "registry", "could not load registry: "+loadErr.Error())
		return nil
	}

	if len(reg.Qt) == 0 {
		fmt.Fprintln(os.Stdout, "  (no Qt installations)")
	} else {
		for _, q := range reg.Qt {
			checkQtInstallation(q)
		}
	}

	// 6. Tools.
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "Tools")
	if len(reg.Tools) == 0 {
		fmt.Fprintln(os.Stdout, "  (no tools installed)")
	} else {
		for _, t := range reg.Tools {
			checkToolInstallation(t)
		}
	}

	return nil
}

func checkMetadataCache() {
	cache, err := repository.NewCache()
	if err != nil {
		printCheck(checkWarn, "Metadata cache", "could not open cache: "+err.Error())
		return
	}

	cacheDir := cache.Dir()
	entries, err := os.ReadDir(cacheDir)
	if err != nil || len(entries) == 0 {
		printCheck(checkWarn, "Metadata cache", "empty (run 'qvm list qt' to populate)")
		return
	}

	// Find the most recently modified cache file.
	var newest time.Time
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}

	if newest.IsZero() {
		printCheck(checkWarn, "Metadata cache", "no cache files found")
		return
	}

	age := time.Since(newest)
	ageStr := formatAge(age)

	if age > 7*24*time.Hour {
		printCheck(checkWarn, "Metadata cache", fmt.Sprintf("stale (updated %s ago)", ageStr))
	} else {
		printCheck(checkOK, "Metadata cache", fmt.Sprintf("OK  (updated %s ago)", ageStr))
	}
}

func checkDiskSpace(cfg *config.Config) {
	if cfg == nil {
		printCheck(checkWarn, "Disk space", "could not determine install dir")
		return
	}

	dir := cfg.Install.Dir
	if dir == "" {
		dir = "."
	}

	free, _, spaceErr := diskSpace(dir)
	if spaceErr != nil {
		printCheck(checkWarn, "Disk space", fmt.Sprintf("could not check: %v", spaceErr))
		return
	}

	freeGB := float64(free) / (1024 * 1024 * 1024)

	if freeGB < 5 {
		printCheck(checkWarn, "Disk space", fmt.Sprintf("low (%.0f GB free on %s)", freeGB, dir))
	} else {
		printCheck(checkOK, "Disk space", fmt.Sprintf("OK  (%.0f GB free on %s)", freeGB, dir))
	}
}

func checkInstallDir(cfg *config.Config) {
	dir := cfg.Install.Dir
	if dir == "" {
		dir = config.DefaultInstallDir()
	}
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			printCheck(checkWarn, "Qt install dir", "does not exist yet: "+dir)
		} else {
			printCheck(checkFail, "Qt install dir", "error: "+err.Error())
		}
		return
	}
	if !info.IsDir() {
		printCheck(checkFail, "Qt install dir", "not a directory: "+dir)
		return
	}
	printCheck(checkOK, "Qt install dir", "OK  "+dir)
}

func checkQtInstallation(q storage.InstalledQt) {
	label := fmt.Sprintf("Qt %s  %s", q.Version, q.Arch)

	// Check directory exists.
	if _, err := os.Stat(q.InstallDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stdout, "  %s  %s\n", checkFail, label)
		fmt.Fprintf(os.Stdout, "       directory missing: %s\n", q.InstallDir)
		return
	}

	// Find qmake.
	qmakeExe := findQmakeInDir(q.InstallDir)
	if qmakeExe == "" {
		fmt.Fprintf(os.Stdout, "  %s  %s\n", checkWarn, label)
		fmt.Fprintf(os.Stdout, "       qmake not found in %s/bin\n", q.InstallDir)
		return
	}

	// Run qmake -version.
	out, err := exec.CommandContext(context.Background(), qmakeExe, "-version").Output()
	if err != nil {
		fmt.Fprintf(os.Stdout, "  %s  %s\n", checkWarn, label)
		fmt.Fprintf(os.Stdout, "       qmake: %s\n", qmakeExe)
		fmt.Fprintf(os.Stdout, "       qmake -version failed: %v\n", err)
		return
	}

	versionLine := strings.TrimSpace(string(out))
	// Extract just the last meaningful line.
	if lines := strings.Split(versionLine, "\n"); len(lines) > 0 {
		versionLine = strings.TrimSpace(lines[len(lines)-1])
	}

	fmt.Fprintf(os.Stdout, "  %s  %s\n", checkOK, label)
	fmt.Fprintf(os.Stdout, "       qmake: %s\n", qmakeExe)
	fmt.Fprintf(os.Stdout, "       qmake -version: %s\n", versionLine)

	// On Windows, check compiler presence for MSVC targets.
	if runtime.GOOS == "windows" && strings.Contains(q.Arch, "msvc") {
		plat := platform.Current()
		ok, msg := plat.CheckCompilerPresent(q.Arch)
		if !ok {
			fmt.Fprintf(os.Stdout, "       %s  %s\n", checkWarn, msg)
		} else if msg != "" {
			fmt.Fprintf(os.Stdout, "       %s  %s\n", checkOK, msg)
		}
	}
}

func checkToolInstallation(t storage.InstalledTool) {
	label := fmt.Sprintf("%s  %s", t.Name, t.Version)

	if _, err := os.Stat(t.InstallDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stdout, "  %s  %s\n", checkFail, label)
		fmt.Fprintf(os.Stdout, "       directory missing: %s\n", t.InstallDir)
		return
	}

	fmt.Fprintf(os.Stdout, "  %s  %s\n", checkOK, label)
}

func findQmakeInDir(installDir string) string {
	candidates := []string{
		filepath.Join(installDir, "bin", "qmake.exe"),
		filepath.Join(installDir, "bin", "qmake"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func printCheck(mark, label, status string) {
	// Pad label to ~26 chars with dots.
	padded := label
	targetLen := 26
	if len(padded) < targetLen {
		padded = padded + " " + strings.Repeat(".", targetLen-len(padded)-1)
	}
	fmt.Fprintf(os.Stdout, "%s  %s %s\n", mark, padded, status)
}

func formatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func osDescription() string {
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x64"
	case "arm64":
		arch = "arm64"
	}

	switch runtime.GOOS {
	case "windows":
		return fmt.Sprintf("Windows (%s)", arch)
	case "darwin":
		return fmt.Sprintf("macOS (%s)", arch)
	default:
		return fmt.Sprintf("Linux (%s)", arch)
	}
}
