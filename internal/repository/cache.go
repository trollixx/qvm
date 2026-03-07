package repository

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Cache manages on-disk gzip-compressed XML metadata files.
type Cache struct {
	dir string
}

// NewCache creates a Cache using the OS cache directory.
func NewCache() (*Cache, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("determining cache dir: %w", err)
	}
	dir := filepath.Join(base, "qvm", "metadata")
	err = os.MkdirAll(dir, 0o750)
	if err != nil {
		return nil, fmt.Errorf("creating cache dir %s: %w", dir, err)
	}
	return &Cache{dir: dir}, nil
}

// Dir returns the cache directory path.
func (c *Cache) Dir() string {
	return c.dir
}

// keyToFilename converts a URL key to a cache filename.
func (c *Cache) keyToFilename(key string) string {
	// Replace special chars with underscores.
	safe := strings.NewReplacer("://", "_", "/", "_", ".", "_", "?", "_").Replace(key)
	return filepath.Join(c.dir, safe+".xml.gz")
}

func (c *Cache) etagFilename(key string) string {
	return c.keyToFilename(key) + ".etag"
}

// ETag returns the stored ETag for key, or "" if none.
func (c *Cache) ETag(key string) string {
	data, err := os.ReadFile(c.etagFilename(key))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// Store saves body (XML) compressed with gzip and the etag.
func (c *Cache) Store(key string, body []byte, etag string) error {
	fn := c.keyToFilename(key)
	// Write gzip-compressed.
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, err := gz.Write(body)
	if err != nil {
		return err
	}
	err = gz.Close()
	if err != nil {
		return err
	}
	err = os.WriteFile(fn, buf.Bytes(), 0o600)
	if err != nil {
		return err
	}
	if etag != "" {
		_ = os.WriteFile(c.etagFilename(key), []byte(etag), 0o600)
	}
	return nil
}

// Load returns the cached XML body for key, decompressed.
// Returns nil, nil if no cache entry exists.
func (c *Cache) Load(key string) ([]byte, error) {
	fn := c.keyToFilename(key)
	data, err := os.ReadFile(fn)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return gunzip(data)
}

// IsStale reports whether the cached entry for key is older than maxAge.
func (c *Cache) IsStale(key string, maxAge time.Duration) bool {
	fn := c.keyToFilename(key)
	info, err := os.Stat(fn)
	if err != nil {
		return true
	}
	return time.Since(info.ModTime()) > maxAge
}

// LoadStale returns the cached XML body regardless of staleness.
func (c *Cache) LoadStale(key string) ([]byte, error) {
	return c.Load(key)
}

func gunzip(data []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("opening gzip reader: %w", err)
	}
	defer gz.Close()
	return io.ReadAll(gz)
}
