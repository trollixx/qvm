package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/adrg/xdg"
)

const registryVersion = 1

// InstalledExtras holds extra content installed alongside a Qt version.
type InstalledExtras struct {
	Docs      bool `json:"docs,omitempty"`
	Examples  bool `json:"examples,omitempty"`
	Sources   bool `json:"sources,omitempty"`
	DebugInfo bool `json:"debug_info,omitempty"`
}

// InstalledQt describes a single installed Qt SDK target.
type InstalledQt struct {
	Version     string          `json:"version"`
	Arch        string          `json:"arch"`
	InstallDir  string          `json:"install_dir"`
	Modules     []string        `json:"modules,omitempty"`
	Extras      InstalledExtras `json:"extras,omitempty"`
	InstalledAt time.Time       `json:"installed_at"`
	SizeBytes   int64           `json:"size_bytes,omitempty"`
}

// UnmarshalJSON migrates the legacy "target" key (renamed to "arch") transparently.
func (q *InstalledQt) UnmarshalJSON(data []byte) error {
	type Alias InstalledQt
	var v struct {
		Alias

		Target string `json:"target"` // legacy field name
	}
	err := json.Unmarshal(data, &v)
	if err != nil {
		return err
	}
	*q = InstalledQt(v.Alias)
	if q.Arch == "" && v.Target != "" {
		q.Arch = v.Target
	}
	return nil
}

// InstalledTool describes an installed tool.
type InstalledTool struct {
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	InstallDir  string    `json:"install_dir"`
	InstalledAt time.Time `json:"installed_at"`
	SizeBytes   int64     `json:"size_bytes,omitempty"`
}

// Registry is the root of the registry file.
type Registry struct {
	Version int             `json:"version"`
	Qt      []InstalledQt   `json:"qt,omitempty"`
	Tools   []InstalledTool `json:"tools,omitempty"`
}

// RegistryManager manages loading and saving the qvm registry.
type RegistryManager struct {
	path string
}

// NewRegistryManager creates a RegistryManager using the XDG state directory.
// On Linux this is ~/.local/state/qvm/; on Windows %LOCALAPPDATA%\qvm\.
func NewRegistryManager() (*RegistryManager, error) {
	path, err := xdg.StateFile("qvm/registry.json")
	if err != nil {
		return nil, fmt.Errorf("determining registry path: %w", err)
	}
	return &RegistryManager{path: path}, nil
}

// NewRegistryManagerAt creates a RegistryManager at a specific path (for testing).
func NewRegistryManagerAt(path string) *RegistryManager {
	return &RegistryManager{path: path}
}

// Path returns the registry file path.
func (m *RegistryManager) Path() string {
	return m.path
}

// Load reads the registry from disk. Returns an empty registry if the file doesn't exist.
// If the file contains legacy data (old "target" key), it is rewritten in place so
// subsequent reads see the clean form.
func (m *RegistryManager) Load() (*Registry, error) {
	data, err := os.ReadFile(m.path)
	if os.IsNotExist(err) {
		return &Registry{Version: registryVersion}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading registry: %w", err)
	}
	var r Registry
	err = json.Unmarshal(data, &r)
	if err != nil {
		return nil, fmt.Errorf("parsing registry: %w", err)
	}
	if r.Version > registryVersion {
		return nil, fmt.Errorf(
			"registry was written by a newer qvm version (version %d > %d); please upgrade qvm",
			r.Version,
			registryVersion,
		)
	}
	if needsMigration(data) {
		_ = m.Save(&r) // best-effort; a read failure here is non-fatal
	}
	return &r, nil
}

// needsMigration reports whether the raw registry JSON contains any legacy "target"
// entries that need to be rewritten as "arch".
func needsMigration(data []byte) bool {
	var raw struct {
		Qt []json.RawMessage `json:"qt"`
	}
	if json.Unmarshal(data, &raw) != nil {
		return false
	}
	for _, entry := range raw.Qt {
		var fields struct {
			Target string `json:"target"`
			Arch   string `json:"arch"`
		}
		if json.Unmarshal(entry, &fields) == nil && fields.Target != "" && fields.Arch == "" {
			return true
		}
	}
	return false
}

// Save atomically writes the registry to disk.
func (m *RegistryManager) Save(r *Registry) error {
	r.Version = registryVersion
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding registry: %w", err)
	}
	err = os.MkdirAll(filepath.Dir(m.path), 0o750)
	if err != nil {
		return fmt.Errorf("creating registry directory: %w", err)
	}
	// Atomic write via temp file.
	tmp := m.path + ".tmp"
	err = os.WriteFile(tmp, data, 0o600)
	if err != nil {
		return fmt.Errorf("writing registry tmp: %w", err)
	}
	err = os.Rename(tmp, m.path)
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("committing registry: %w", err)
	}
	return nil
}

// AddQt adds or replaces a Qt installation record.
func (m *RegistryManager) AddQt(entry InstalledQt) error {
	unlock, err := lockFile(m.path)
	if err != nil {
		return fmt.Errorf("acquiring registry lock: %w", err)
	}
	defer unlock()

	r, err := m.Load()
	if err != nil {
		return err
	}
	// Replace if same version+arch.
	for i, q := range r.Qt {
		if q.Version == entry.Version && q.Arch == entry.Arch {
			r.Qt[i] = entry
			return m.Save(r)
		}
	}
	r.Qt = append(r.Qt, entry)
	return m.Save(r)
}

// RemoveQt removes a Qt installation record.
func (m *RegistryManager) RemoveQt(version, arch string) error {
	unlock, err := lockFile(m.path)
	if err != nil {
		return fmt.Errorf("acquiring registry lock: %w", err)
	}
	defer unlock()

	r, err := m.Load()
	if err != nil {
		return err
	}
	filtered := r.Qt[:0]
	for _, q := range r.Qt {
		if q.Version == version && (arch == "" || q.Arch == arch) {
			continue
		}
		filtered = append(filtered, q)
	}
	r.Qt = filtered
	return m.Save(r)
}

// AddTool adds or replaces a tool installation record.
func (m *RegistryManager) AddTool(entry InstalledTool) error {
	unlock, err := lockFile(m.path)
	if err != nil {
		return fmt.Errorf("acquiring registry lock: %w", err)
	}
	defer unlock()

	r, err := m.Load()
	if err != nil {
		return err
	}
	for i, t := range r.Tools {
		if t.Name == entry.Name && t.Version == entry.Version {
			r.Tools[i] = entry
			return m.Save(r)
		}
	}
	r.Tools = append(r.Tools, entry)
	return m.Save(r)
}

// RemoveTool removes a tool installation record.
func (m *RegistryManager) RemoveTool(name, version string) error {
	unlock, err := lockFile(m.path)
	if err != nil {
		return fmt.Errorf("acquiring registry lock: %w", err)
	}
	defer unlock()

	r, err := m.Load()
	if err != nil {
		return err
	}
	filtered := r.Tools[:0]
	for _, t := range r.Tools {
		if t.Name == name && (version == "" || t.Version == version) {
			continue
		}
		filtered = append(filtered, t)
	}
	r.Tools = filtered
	return m.Save(r)
}
