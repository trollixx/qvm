package config

import (
	"os"
	"path/filepath"

	"github.com/adrg/xdg"

	toml "github.com/pelletier/go-toml/v2"
)

// Config holds all qvm configuration.
type Config struct {
	Qt struct {
		Default string `toml:"default"` // default version for commands that take an optional <version>
	} `toml:"qt"`

	Install struct {
		Dir string `toml:"dir"`
	} `toml:"install"`

	Repository struct {
		URL       string   `toml:"url"`
		Mirrors   []string `toml:"mirrors"`
		Blacklist []string `toml:"blacklist"`
	} `toml:"repository"`

	Download struct {
		Concurrency    int `toml:"concurrency"`
		TimeoutSeconds int `toml:"timeout_seconds"`
	} `toml:"download"`
}

// Load reads the config from the default config file, creating defaults if absent.
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	cfg := defaults()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}

	err = toml.Unmarshal(data, cfg)
	if err != nil {
		return nil, err
	}

	// Fill in blanks with defaults.
	if cfg.Install.Dir == "" {
		cfg.Install.Dir = DefaultInstallDir()
	}
	if cfg.Repository.URL == "" {
		cfg.Repository.URL = DefaultRepositoryURL
	}
	if cfg.Download.Concurrency == 0 {
		cfg.Download.Concurrency = DefaultConcurrency()
	}
	if cfg.Download.TimeoutSeconds == 0 {
		cfg.Download.TimeoutSeconds = DefaultTimeoutSeconds()
	}

	return cfg, nil
}

// Save writes cfg to the default config file.
func Save(cfg *Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	err = os.MkdirAll(filepath.Dir(path), 0o750)
	if err != nil {
		return err
	}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func configPath() (string, error) {
	return xdg.ConfigFile("qvm/config.toml")
}

// Path returns the path to the qvm config file.
func Path() (string, error) {
	return configPath()
}

func defaults() *Config {
	cfg := &Config{}
	cfg.Install.Dir = DefaultInstallDir()
	cfg.Repository.URL = DefaultRepositoryURL
	cfg.Download.Concurrency = DefaultConcurrency()
	cfg.Download.TimeoutSeconds = DefaultTimeoutSeconds()
	return cfg
}
