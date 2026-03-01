package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

)

// MirrorListURL is the Qt project page that lists community download mirrors.
const MirrorListURL = "https://download.qt.io/static/mirrorlist/"

// mirrorListData is the on-disk JSON structure for the cached mirror list.
type mirrorListData struct {
	Mirrors   []string  `json:"mirrors"`
	FetchedAt time.Time `json:"fetched_at"`
}

// MirrorListCache manages the on-disk cache of Qt mirrors fetched from MirrorListURL.
// The cache is only refreshed when the caller explicitly calls Save.
type MirrorListCache struct {
	path string
}

// NewMirrorListCache creates a MirrorListCache stored in the OS cache directory.
func NewMirrorListCache() (*MirrorListCache, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("determining mirror cache path: %w", err)
	}
	dir := filepath.Join(base, "qvm")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating cache dir: %w", err)
	}
	return &MirrorListCache{path: filepath.Join(dir, "mirrors.json")}, nil
}

// NewMirrorListCacheAt creates a MirrorListCache at a specific path.
// Intended for testing.
func NewMirrorListCacheAt(path string) *MirrorListCache {
	return &MirrorListCache{path: path}
}

// Path returns the path to the cache file.
func (c *MirrorListCache) Path() string { return c.path }

// Exists reports whether a cached mirror list is present on disk.
func (c *MirrorListCache) Exists() bool {
	_, err := os.Stat(c.path)
	return err == nil
}

// Load returns the cached mirror list. Returns nil, nil if no cache exists yet.
func (c *MirrorListCache) Load() ([]string, error) {
	data, err := os.ReadFile(c.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var d mirrorListData
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("parsing mirror list cache: %w", err)
	}
	return d.Mirrors, nil
}

// Save writes mirrors to the cache file, overwriting any previous contents.
func (c *MirrorListCache) Save(mirrors []string) error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	d := mirrorListData{Mirrors: mirrors, FetchedAt: time.Now().UTC()}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, data, 0o644)
}

// FetchMirrorList downloads Qt's mirror list page and returns the mirror base URLs.
// Each returned URL ends with "/" and is suitable as a mirror base (no online/qtsdkrepository/ suffix).
func FetchMirrorList(ctx context.Context, timeoutSeconds int) ([]string, error) {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, MirrorListURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", MirrorListURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching mirror list: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	mirrors := ParseMirrorListHTML(body)
	if len(mirrors) == 0 {
		return nil, fmt.Errorf("no mirrors found in response from %s", MirrorListURL)
	}
	return mirrors, nil
}

// ParseMirrorListHTML extracts HTTPS mirror base URLs from Qt's mirrorlist HTML page.
// It skips Qt's own infrastructure domains and normalizes URLs to end with "/".
func ParseMirrorListHTML(body []byte) []string {
	html := string(body)
	var mirrors []string
	seen := map[string]bool{}

	rest := html
	for {
		idx := strings.Index(rest, `href="https://`)
		if idx < 0 {
			break
		}
		rest = rest[idx+6:] // skip past 'href="'
		end := strings.IndexByte(rest, '"')
		if end < 0 {
			break
		}
		rawURL := rest[:end]
		rest = rest[end:]

		if isQtOwnDomain(rawURL) {
			continue
		}
		// Normalize: ensure trailing slash.
		if !strings.HasSuffix(rawURL, "/") {
			rawURL += "/"
		}
		if !seen[rawURL] {
			seen[rawURL] = true
			mirrors = append(mirrors, rawURL)
		}
	}
	return mirrors
}

// isQtOwnDomain reports whether url belongs to Qt's own infrastructure
// rather than a community mirror.
func isQtOwnDomain(url string) bool {
	for _, prefix := range []string{
		"https://download.qt.io/",
		"https://master.qt.io/",
		"https://www.qt.io/",
		"https://qt.io/",
	} {
		if strings.HasPrefix(url, prefix) {
			return true
		}
	}
	return false
}
