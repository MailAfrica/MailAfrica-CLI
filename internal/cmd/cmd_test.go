package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MailAfrica/MailAfrica-CLI/internal/config"
)

const testAPIKey = "MAIL_testkey1234567890abcdef"

// newAuthTestServer simulates the MailAfrica auth + apikeys surface: login
// issues rt-1/jwt-1, any protected call 401s until refreshed to jwt-2, and the
// refresh endpoint rotates rt-1 → rt-2. Once the CLI stores the API key, the
// server accepts X-API-Key too.
func newAuthTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	user := map[string]any{
		"id": 7, "name": "Test User", "email": "user@example.com",
		"is_admin": false, "created_at": "2026-01-01T00:00:00Z",
	}

	writeAuth := func(w http.ResponseWriter, token, rt string) {
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    map[string]any{"token": token, "refresh_token": rt, "user": user},
		})
	}

	validAuth := func(r *http.Request) bool {
		return r.Header.Get("Authorization") == "Bearer jwt-2" ||
			r.Header.Get("X-API-Key") == testAPIKey
	}

	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		writeAuth(w, "jwt-1", "rt-1")
	})
	mux.HandleFunc("/api/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
		writeAuth(w, "jwt-2", "rt-2")
	})
	mux.HandleFunc("/api/auth/me", func(w http.ResponseWriter, r *http.Request) {
		if !validAuth(r) {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"success":false,"errors":[{"code":"UNAUTHORIZED"}]}`)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"success": true, "data": user})
	})
	mux.HandleFunc("/api/apikeys/", func(w http.ResponseWriter, r *http.Request) {
		if !validAuth(r) {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"success":false,"errors":[{"code":"UNAUTHORIZED"}]}`)
			return
		}
		keyRow := map[string]any{
			"id": 1, "user_id": 7, "name": "ci", "key_prefix": "MAIL_testkey",
			"scopes": "full", "created_at": "2026-01-01T00:00:00Z",
		}
		switch r.Method {
		case http.MethodPost:
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data":    map[string]any{"api_key": keyRow, "key": testAPIKey},
			})
		case http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{
				"success":    true,
				"data":       []any{keyRow},
				"pagination": nil,
				"message":    "api keys retrieved",
				"request_id": "req",
				"timestamp":  "2026-01-01T00:00:00Z",
			})
		case http.MethodDelete:
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": nil})
		default:
			http.Error(w, "unsupported", http.StatusMethodNotAllowed)
		}
	})
	return httptest.NewServer(mux)
}

func runCLI(args ...string) (string, error) {
	root := NewRootCmd(args)
	var buf bytes.Buffer
	root.SetOut(&buf)
	err := root.Execute()
	return buf.String(), err
}

func TestCLI_AuthAndAPIKeysEndToEnd(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))
	t.Setenv(config.EnvAPIURL, "")
	t.Setenv(config.EnvAPIKey, "")
	srv := newAuthTestServer(t)
	defer srv.Close()

	// 1. login: no prompt because --password is supplied.
	out, err := runCLI("auth", "login", "--identifier", "user@example.com", "--password", "x", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("login error = %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "Test User") {
		t.Fatalf("login output missing user: %s", out)
	}
	// 2. me in a fresh process: must 401 → refresh rt-1→rt-2 → retry.
	out, err = runCLI("auth", "me", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("me error = %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "user@example.com") {
		t.Fatalf("me output missing identity: %s", out)
	}

	// 3. create an API key and save it.
	out, err = runCLI("apikeys", "create", "--name", "ci", "--save", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("apikeys create error = %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, testAPIKey) {
		t.Fatalf("create output must show the key once: %s", out)
	}

	// 4. list + revoke now authenticate via the stored API key.
	out, err = runCLI("apikeys", "list", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("apikeys list error = %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "ci") || !strings.Contains(out, "MAIL_testkey") {
		t.Fatalf("list output missing key row: %s", out)
	}
	out, err = runCLI("apikeys", "revoke", "1", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("revoke error = %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "revoked") {
		t.Fatalf("revoke output: %s", out)
	}

	// 5. me via API key.
	out, err = runCLI("auth", "me", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("me via key error = %v", err)
	}
	if !strings.Contains(out, "Test User") {
		t.Fatalf("me via key output: %s", out)
	}
}

func TestCLI_NotAuthenticated(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))
	t.Setenv(config.EnvAPIKey, "")
	out, err := runCLI("apikeys", "list", "--api-url", "http://127.0.0.1:1")
	if err == nil {
		t.Fatalf("expected not-authenticated error, got output %q", out)
	}
	if !strings.Contains(err.Error(), "not authenticated") {
		t.Fatalf("err = %v", err)
	}
}

// TestCLI_EnvAPIKeyAuthenticates verifies MAILAFRICA_API_KEY works end to end
// with no stored config — the script-driven auth path.
func TestCLI_EnvAPIKeyAuthenticates(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))
	t.Setenv(config.EnvAPIKey, testAPIKey)

	srv := newAuthTestServer(t)
	defer srv.Close()

	out, err := runCLI("auth", "me", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("me via env key error = %v", err)
	}
	if !strings.Contains(out, "Test User") {
		t.Fatalf("env-key me output: %s", out)
	}
}

// TestCLI_JSONOutput verifies --json renders machine-readable output and that
// session persistence still happens for non-table output.
func TestCLI_JSONOutput(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))
	t.Setenv(config.EnvAPIKey, "")

	srv := newAuthTestServer(t)
	defer srv.Close()

	out, err := runCLI("auth", "login", "--identifier", "user@example.com", "--password", "x", "--api-url", srv.URL, "--json")
	if err != nil {
		t.Fatalf("login --json error = %v\n%s", err, out)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("login --json output not JSON: %v\n%s", err, out)
	}
	if parsed["name"] != "Test User" {
		t.Fatalf("login --json parsed name = %v", parsed["name"])
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.GetRefreshToken() != "rt-1" {
		t.Fatalf("stored refresh token = %q, want rt-1", cfg.GetRefreshToken())
	}
}

func TestCLI_ConfigGetSet(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))
	t.Setenv(config.EnvAPIURL, "")

	// Default URL.
	out, err := runCLI("config", "get", "api-url")
	if err != nil {
		t.Fatalf("config get error = %v", err)
	}
	if !strings.Contains(out, config.DefaultAPIURL) {
		t.Fatalf("default api-url = %q, want %q", out, config.DefaultAPIURL)
	}

	// Set + get.
	if _, err := runCLI("config", "set", "api-url", "https://staging.example.test"); err != nil {
		t.Fatalf("config set error = %v", err)
	}
	out, err = runCLI("config", "get", "api-url")
	if err != nil {
		t.Fatalf("config get error = %v", err)
	}
	if !strings.Contains(out, "https://staging.example.test") {
		t.Fatalf("api-url after set = %q", out)
	}

	// Env overrides file.
	t.Setenv(config.EnvAPIURL, "https://env.example.test")
	out, err = runCLI("config", "get", "api-url")
	if err != nil {
		t.Fatalf("config get error = %v", err)
	}
	if !strings.Contains(out, "https://env.example.test") || strings.Contains(out, "staging") {
		t.Fatalf("env override api-url = %q", out)
	}
}

func TestCLI_ConfigSecretsNeverRevealed(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))
	t.Setenv(config.EnvAPIKey, "")
	t.Setenv(config.EnvAPIURL, "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := cfg.SetAPIKey("MAIL_SUPER_SECRET_VALUE"); err != nil {
		t.Fatalf("set key: %v", err)
	}

	out, err := runCLI("config", "get", "api-key")
	if err != nil {
		t.Fatalf("config get api-key error = %v", err)
	}
	if strings.Contains(out, "MAIL_SUPER_SECRET_VALUE") {
		t.Fatalf("config get api-key must never reveal the value: %q", out)
	}
	if !strings.Contains(out, "set") {
		t.Fatalf("config get api-key should report 'set': %q", out)
	}

	// Setting secrets through `config set` must be refused.
	if _, err := runCLI("config", "set", "api-key", "MAIL_NEW"); err == nil {
		t.Fatal("config set api-key should be refused")
	}
	var buf bytes.Buffer
	root := NewRootCmd([]string{"version"})
	root.SetOut(&buf)
	if err := root.Execute(); err != nil {
		t.Fatalf("version error = %v", err)
	}
	if !strings.Contains(buf.String(), "mailafrica") {
		t.Fatalf("version output: %q", buf.String())
	}
}
