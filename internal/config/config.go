// Package config resolves CLI settings and stores credentials safely on disk.
//
// Precedence (highest first): explicit flags → environment variables → the
// config file at $XDG_CONFIG_HOME/mailafrica/config.json (default
// ~/.config/mailafrica/config.json), which is always written with mode 0600 so
// credentials never leak to other users.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	// EnvAPIURL overrides the API base URL for the current process.
	EnvAPIURL = "MAILAFRICA_API_URL"
	// EnvAPIKey overrides the stored API key for the current process.
	EnvAPIKey = "MAILAFRICA_API_KEY"

	// DefaultAPIURL is the production MailAfrica API.
	DefaultAPIURL = "https://api.mailafrica.online"
)

// Config is the persisted CLI state. Credential fields are never logged or
// printed by commands; this file itself is the only secret store.
//
// mu guards every field so a shared *Config can be used from concurrent
// request paths (the API client may retry/refresh from multiple goroutines).
type Config struct {
	mu           sync.Mutex
	APIURL       string `json:"api_url,omitempty"`
	APIKey       string `json:"api_key,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	LastEmail    string `json:"last_email,omitempty"`
}

// Path returns the config file location, honouring XDG_CONFIG_HOME when set.
func Path() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "mailafrica", "config.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "mailafrica", "config.json")
	}
	return filepath.Join(home, ".config", "mailafrica", "config.json")
}

// Load reads the config file. A missing file yields an empty config, not an
// error, so first run works out of the box.
func Load() (*Config, error) {
	data, err := os.ReadFile(Path())
	if errors.Is(err, os.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", Path(), err)
	}
	return &c, nil
}

// Save writes the config atomically (tmp + rename) with owner-only
// permissions, creating the directory tree as needed.
func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.saveLocked()
}

func (c *Config) saveLocked() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	dir := filepath.Dir(Path())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	tmp := Path() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, Path()); err != nil {
		return fmt.Errorf("commit config: %w", err)
	}
	if err := os.Chmod(Path(), 0o600); err != nil {
		return fmt.Errorf("secure config: %w", err)
	}
	return nil
}

// GetRefreshToken returns the stored refresh token.
func (c *Config) GetRefreshToken() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.RefreshToken
}

// EffectiveAPIURL resolves the base URL: env var → file → default.
func (c *Config) EffectiveAPIURL() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v := strings.TrimRight(os.Getenv(EnvAPIURL), "/"); v != "" {
		return v
	}
	if v := strings.TrimRight(c.APIURL, "/"); v != "" {
		return v
	}
	return DefaultAPIURL
}

// EffectiveAPIKey resolves the API key: env var → file.
func (c *Config) EffectiveAPIKey() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v := os.Getenv(EnvAPIKey); v != "" {
		return v
	}
	return c.APIKey
}

// SetAPIKey persists a new API key with secure permissions.
func (c *Config) SetAPIKey(k string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.APIKey = k
	return c.saveLocked()
}

// SetRefreshToken persists a rotated refresh token with secure permissions.
func (c *Config) SetRefreshToken(rt string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.RefreshToken = rt
	return c.saveLocked()
}

// SetLastEmail remembers the account identity that last logged in locally.
func (c *Config) SetLastEmail(email string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.LastEmail = email
	return c.saveLocked()
}

// ClearAuth removes every stored credential (API key, refresh token). The file
// is retained with empty credential fields.
func (c *Config) ClearAuth() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.APIKey = ""
	c.RefreshToken = ""
	return c.saveLocked()
}
