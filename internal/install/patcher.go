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
		lines := strings.Split(string(data), "\n")
		found := false
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "Prefix") {
				lines[i] = "Prefix=" + prefix
				found = true
				break
			}
		}
		if !found {
			// Existing file has no Prefix line - insert one after [Paths].
			inserted := false
			for i, line := range lines {
				if strings.TrimSpace(line) == "[Paths]" {
					tail := make([]string, len(lines)-i-1)
					copy(tail, lines[i+1:])
					lines = append(lines[:i+1], append([]string{"Prefix=" + prefix}, tail...)...)
					inserted = true
					break
				}
			}
			if !inserted {
				lines = append([]string{"[Paths]", "Prefix=" + prefix, ""}, lines...)
			}
		}
		if err := os.WriteFile(qtConfPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			return fmt.Errorf("writing qt.conf: %w", err)
		}
		return nil
	}

	// No existing file - create a minimal one.
	if err := os.MkdirAll(filepath.Dir(qtConfPath), 0o755); err != nil {
		return fmt.Errorf("creating bin dir for qt.conf: %w", err)
	}
	content := "[Paths]\nPrefix=" + prefix + "\n"
	if err := os.WriteFile(qtConfPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing qt.conf: %w", err)
	}
	return nil
}
