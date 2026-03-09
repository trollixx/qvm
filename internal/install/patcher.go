package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PatchQtConf writes (or rewrites) the qt.conf file in the installed Qt
// directory so that the Prefix points to the actual installation path.
// Qt archives do not ship a qt.conf; this step creates it.
func PatchQtConf(installDir string) error {
	qtConfPath := filepath.Join(installDir, "bin", "qt.conf")

	// Read existing content if present; ignore not-found.
	data, err := os.ReadFile(qtConfPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading qt.conf: %w", err)
	}

	prefix := filepath.ToSlash(installDir)

	// If file already exists, replace the Prefix line in-place so any other
	// settings (Translations, Plugins, ...) are preserved.
	if len(data) > 0 {
		err = updateQtConf(data, prefix, qtConfPath)
		if err != nil {
			return fmt.Errorf("writing qt.conf: %w", err)
		}
		return nil
	}

	// No existing file - create a minimal one.
	err = os.MkdirAll(filepath.Dir(qtConfPath), 0o755) //nolint:gosec // 0755 for Qt SDK
	if err != nil {
		return fmt.Errorf("creating bin dir for qt.conf: %w", err)
	}
	content := "[Paths]\nPrefix=" + prefix + "\n"
	err = os.WriteFile(qtConfPath, []byte(content), 0o644) //nolint:gosec // 0644 ok
	if err != nil {
		return fmt.Errorf("writing qt.conf: %w", err)
	}
	return nil
}

// updateQtConf rewrites the Prefix line in an existing qt.conf, preserving all other settings.
func updateQtConf(data []byte, prefix, path string) error {
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "Prefix") {
			lines[i] = "Prefix=" + prefix
			return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644) //nolint:gosec // 0644 ok for Qt SDK
		}
	}
	// No Prefix line — insert one after [Paths].
	for i, line := range lines {
		if strings.TrimSpace(line) == "[Paths]" {
			tail := make([]string, len(lines)-i-1)
			copy(tail, lines[i+1:])
			lines = append(lines[:i+1], append([]string{"Prefix=" + prefix}, tail...)...)
			return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644) //nolint:gosec // 0644 ok for Qt SDK
		}
	}
	// No [Paths] section — prepend one.
	lines = append([]string{"[Paths]", "Prefix=" + prefix, ""}, lines...)
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644) //nolint:gosec // 0644 ok for Qt SDK
}
