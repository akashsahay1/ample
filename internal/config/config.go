// Package config persists AMPLS user configuration in <Home>/config.json.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"ampls/internal/paths"
)

type Ports struct {
	HTTP  int `json:"http"`
	HTTPS int `json:"https"`
	MySQL int `json:"mysql"`
}

// Link is a single folder served as <Name>.<tld>, outside any parked directory.
type Link struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// SiteSettings are per-site overrides, keyed by site name (e.g. "blog").
type SiteSettings struct {
	PHP     string `json:"php,omitempty"`     // isolated PHP minor version, "" = default
	Secure  bool   `json:"secure,omitempty"`  // serve over HTTPS
	DocRoot string `json:"docRoot,omitempty"` // override detected document root (relative to site path)
}

type MySQLConfig struct {
	RootPassword string `json:"rootPassword"`
}

type AppSettings struct {
	StartServicesOnLaunch bool `json:"startServicesOnLaunch"`
	StopServicesOnQuit    bool `json:"stopServicesOnQuit"`
	LaunchAtLogin         bool `json:"launchAtLogin"`
}

type Config struct {
	Version    int                     `json:"version"`
	DefaultPHP string                  `json:"defaultPhp"` // minor version, e.g. "8.5"
	TLD        string                  `json:"tld"`
	Ports      Ports                   `json:"ports"`
	Parked     []string                `json:"parked"`
	Links      []Link                  `json:"links"`
	Sites      map[string]SiteSettings `json:"sites"`
	MySQL      MySQLConfig             `json:"mysql"`
	App        AppSettings             `json:"app"`
}

func Default() *Config {
	return &Config{
		Version: 1,
		TLD:     "test",
		Ports:   Ports{HTTP: 80, HTTPS: 443, MySQL: 3306},
		Parked:  []string{paths.DefaultSitesDir()},
		Links:   []Link{},
		Sites:   map[string]SiteSettings{},
		App:     AppSettings{StartServicesOnLaunch: true, StopServicesOnQuit: true},
	}
}

var mu sync.Mutex

// Load reads config.json, returning defaults when it does not exist yet.
func Load() (*Config, error) {
	mu.Lock()
	defer mu.Unlock()
	return load()
}

func load() (*Config, error) {
	c := Default()
	b, err := os.ReadFile(paths.ConfigFile())
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, c); err != nil {
		return nil, err
	}
	if c.Sites == nil {
		c.Sites = map[string]SiteSettings{}
	}
	if c.TLD == "" {
		c.TLD = "test"
	}
	return c, nil
}

// Save writes config.json atomically.
func Save(c *Config) error {
	mu.Lock()
	defer mu.Unlock()
	return save(c)
}

func save(c *Config) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(paths.ConfigFile()), 0o755); err != nil {
		return err
	}
	tmp := paths.ConfigFile() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, paths.ConfigFile())
}

// Update loads, mutates and saves the config under a single lock.
func Update(fn func(c *Config) error) (*Config, error) {
	mu.Lock()
	defer mu.Unlock()
	c, err := load()
	if err != nil {
		return nil, err
	}
	if err := fn(c); err != nil {
		return nil, err
	}
	return c, save(c)
}
