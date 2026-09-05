package config

import (
	"os"
	"path/filepath"
	"testing"
)

func setXdg(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))
}

func TestPathUsesXdgConfigHome(t *testing.T) {
	base := t.TempDir()
	setXdg(t, base)
	got := Path()
	want := filepath.Join(base, "mailafrica", "config.json")
	if got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}

func TestLoadMissingFileReturnsEmpty(t *testing.T) {
	setXdg(t, t.TempDir())
	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if c.APIKey != "" || c.APIURL != "" || c.RefreshToken != "" {
		t.Fatalf("expected empty config, got %+v", c)
	}
}

func TestSaveLoadRoundtripAndMode(t *testing.T) {
	setXdg(t, t.TempDir())
	c := &Config{APIURL: "https://example.test", APIKey: "MAIL_secret", RefreshToken: "abc"}
	if err := c.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	fi, err := os.Stat(Path())
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config mode = %o, want 600", perm)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.APIKey != "MAIL_secret" || got.APIURL != "https://example.test" || got.RefreshToken != "abc" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
}

func TestAPIURLPrecedenceEnvOverFileOverDefault(t *testing.T) {
	setXdg(t, t.TempDir())
	// default
	c := &Config{}
	if got := c.EffectiveAPIURL(); got != DefaultAPIURL {
		t.Fatalf("default APIURL = %q", got)
	}
	// file
	c.APIURL = "https://file.test"
	if got := c.EffectiveAPIURL(); got != "https://file.test" {
		t.Fatalf("file APIURL = %q", got)
	}
	// env
	t.Setenv(EnvAPIURL, "https://env.test/")
	if got := c.EffectiveAPIURL(); got != "https://env.test" {
		t.Fatalf("env APIURL = %q", got)
	}
}

func TestAPIKeyPrecedenceEnvOverFile(t *testing.T) {
	setXdg(t, t.TempDir())
	c := &Config{APIKey: "MAIL_file"}
	if got := c.EffectiveAPIKey(); got != "MAIL_file" {
		t.Fatalf("file APIKey = %q", got)
	}
	t.Setenv(EnvAPIKey, "MAIL_env}")
	if got := c.EffectiveAPIKey(); got != "MAIL_env}" {
		t.Fatalf("env APIKey = %q", got)
	}
}

func TestClearAuth(t *testing.T) {
	setXdg(t, t.TempDir())
	c := &Config{APIKey: "MAIL_k", RefreshToken: "rt", APIURL: "https://x.test"}
	if err := c.SetRefreshToken("rt"); err != nil {
		t.Fatalf("SetRefreshToken() error = %v", err)
	}
	if err := c.ClearAuth(); err != nil {
		t.Fatalf("ClearAuth() error = %v", err)
	}
	if c.APIKey != "" || c.RefreshToken != "" || c.APIURL != "https://x.test" {
		t.Fatalf("ClearAuth left residue: %+v", c)
	}
}
