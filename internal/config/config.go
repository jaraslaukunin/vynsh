// Package config loads and stores optional user configuration for vynsh.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/jaraslaukunin/vynsh/internal/audit"
	"github.com/jaraslaukunin/vynsh/internal/fsutil"
)

// Config holds user overrides. Every field is optional.
type Config struct {
	// Excludes are extra paths or glob patterns skipped by scan, dupes and rm.
	Excludes []string `json:"excludes,omitempty"`
	// Protected lists extra paths vynsh must never delete.
	Protected []string `json:"protected,omitempty"`
	// ScanDepth is the default tree depth used by `vynsh scan`.
	ScanDepth int `json:"scan_depth,omitempty"`
	// Top is the default number of rows in tables.
	Top int `json:"top,omitempty"`
	// TrashByDefault makes `vynsh rm` move files to the trash instead of
	// deleting them outright.
	TrashByDefault bool `json:"trash_by_default,omitempty"`
	// NoColor disables ANSI colours.
	NoColor bool `json:"no_color,omitempty"`
}

// Default returns the built-in configuration.
func Default() *Config { return &Config{ScanDepth: 2, Top: 15} }

// Path returns the configuration file path.
func Path() string {
	dir, err := audit.Dir()
	if err != nil {
		return filepath.Join(fsutil.HomeDir(), ".config", "vynsh", "config.json")
	}
	return filepath.Join(dir, "config.json")
}

// Load reads the configuration file, returning defaults when it is absent.
func Load() (*Config, error) {
	cfg := Default()
	data, err := os.ReadFile(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return Default(), err
	}
	if cfg.ScanDepth <= 0 {
		cfg.ScanDepth = 2
	}
	if cfg.Top <= 0 {
		cfg.Top = 15
	}
	return cfg, nil
}

// Save writes the configuration file.
func Save(cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(Path()), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(Path(), data, 0o600)
}

// WriteDefault creates the configuration file with default values if it does
// not exist yet, and returns its path.
func WriteDefault() (string, error) {
	p := Path()
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	cfg := Default()
	cfg.Protected = []string{}
	cfg.Excludes = []string{}
	if err := Save(cfg); err != nil {
		return p, err
	}
	return p, nil
}

// Apply registers the extra protected paths with the filesystem layer.
func (c *Config) Apply() {
	if c == nil {
		return
	}
	if len(c.Protected) > 0 {
		fsutil.AddProtected(c.Protected...)
	}
}
