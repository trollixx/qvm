package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// qtConfMode is the file mode used for qt.conf (world-readable).
const qtConfMode os.FileMode = 0o644

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
	err = atomicWriteFile(qtConfPath, []byte(content))
	if err != nil {
		return fmt.Errorf("writing qt.conf: %w", err)
	}
	return nil
}

// isPrefixLine reports whether line is a Qt-conf "Prefix=..." entry.
// It deliberately rejects look-alikes such as "PrefixOptions=" by requiring
// the trimmed line to start with "Prefix" followed by either '=' or whitespace.
func isPrefixLine(line string) bool {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "Prefix") {
		return false
	}
	rest := t[len("Prefix"):]
	if rest == "" {
		return true
	}
	switch rest[0] {
	case '=', ' ', '\t':
		return true
	}
	return false
}

// updateQtConf rewrites the Prefix line in an existing qt.conf, preserving all other settings.
func updateQtConf(data []byte, prefix, path string) error {
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if isPrefixLine(line) {
			lines[i] = "Prefix=" + prefix
			return atomicWriteFile(path, []byte(strings.Join(lines, "\n")))
		}
	}
	// No Prefix line — insert one after [Paths].
	for i, line := range lines {
		if strings.TrimSpace(line) == "[Paths]" {
			tail := make([]string, len(lines)-i-1)
			copy(tail, lines[i+1:])
			lines = append(lines[:i+1], append([]string{"Prefix=" + prefix}, tail...)...)
			return atomicWriteFile(path, []byte(strings.Join(lines, "\n")))
		}
	}
	// No [Paths] section — prepend one.
	lines = append([]string{"[Paths]", "Prefix=" + prefix, ""}, lines...)
	return atomicWriteFile(path, []byte(strings.Join(lines, "\n")))
}

// atomicWriteFile writes data to path via a temp-file + rename so a crash mid-write
// cannot corrupt the destination. Files are written with qtConfMode (0o644).
func atomicWriteFile(path string, data []byte) error {
	tmp := path + ".tmp"
	err := os.WriteFile(tmp, data, qtConfMode) //nolint:gosec // qtConfMode is 0o644 by design
	if err != nil {
		return err
	}
	err = os.Rename(tmp, path)
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
