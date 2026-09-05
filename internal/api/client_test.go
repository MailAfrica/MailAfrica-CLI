package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/MailAfrica/MailAfrica-CLI/internal/config"
)

// newTestConfig returns a real config backed by a throwaway XDG dir so token
// rotation exercises the persistence path end to end.
func newTestConfig(t *testing.T) *config.Config {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))
	t.Setenv(config.EnvAPIURL, "")
	t.Setenv(config.EnvAPIKey, "")
	c, err := config.Load()
	if err != nil {
		t.Fatalf("load test config: %v", err)
	}
	return c
}

func envelope(t *testing.T, status int, success bool, data any) []byte {
	t.Helper()
	body := map[string]any{"success": success, "data": data}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return b
}

func writeJSON(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func authResponse(token, refresh string) map[string]any {
	user := map[string]any{
		"id": 1, "name": "Test User", "email": "user@example.com",
		"is_admin": false, "created_at": "2026-01-01T00:00:00Z",
	}
	return map[string]any{"token": token, "refresh_token": refresh, "user": user}
}

// TestDo_RefreshesAndRetriesOn401 is the happy-refresh path: the first
// authenticated call returns 401, the client rotates the stored refresh token,
// and retries the original request exactly once with the new JWT.
func TestDo_RefreshesAndRetriesOn401(t *testing.T) {
	var (
		refreshCalls   atomic.Int32
		protectedCalls atomic.Int32
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/refresh":
			refreshCalls.Add(1)
			writeJSON(w, http.StatusOK, envelope(t, 200, true, authResponse("jwt-new", "rt-2")))
		case "/api/protected":
			n := protectedCalls.Add(1)
			if n == 1 {
				writeJSON(w, http.StatusUnauthorized, envelope(t, 401, false, nil))
				return
			}
			if got := r.Header.Get("Authorization"); got != "Bearer jwt-new" {
				writeJSON(w, http.StatusForbidden, envelope(t, 403, false, nil))
				return
			}
			writeJSON(w, http.StatusOK, envelope(t, 200, true, map[string]any{"ok": true}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := newTestConfig(t)
	_ = cfg.SetRefreshToken("rt-1")
	cl := New(srv.URL, cfg, nil)

	var out struct {
		OK bool `json:"ok"`
	}
	if err := cl.Do(context.Background(), http.MethodGet, "/api/protected", nil, &out); err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if !out.OK {
		t.Fatal("expected parsed data ok=true")
	}
	if refreshCalls.Load() != 1 {
		t.Fatalf("refresh calls = %d, want 1", refreshCalls.Load())
	}
	if protectedCalls.Load() != 2 {
		t.Fatalf("protected calls = %d, want 2 (401 then retry)", protectedCalls.Load())
	}
	// Rotated refresh token must have been persisted.
	if cfg.RefreshToken != "rt-2" {
		t.Fatalf("persisted refresh token = %q, want rt-2", cfg.RefreshToken)
	}
}

// TestDo_RefreshFails_ClearsAuth covers the dead-refresh-token path: a 401 on
// the refresh call must clear stored credentials and surface ErrRefreshExpired,
// never retry into an unwinnable loop.
func TestDo_RefreshFails_ClearsAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/refresh":
			writeJSON(w, http.StatusUnauthorized, envelope(t, 401, false, nil))
		case "/api/protected":
			writeJSON(w, http.StatusUnauthorized, envelope(t, 401, false, nil))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := newTestConfig(t)
	_ = cfg.SetRefreshToken("rt-dead")
	cl := New(srv.URL, cfg, nil)

	var out map[string]any
	err := cl.Do(context.Background(), http.MethodGet, "/api/protected", nil, &out)
	if err == nil {
		t.Fatal("expected error from expired refresh token")
	}
	if !strings.Contains(err.Error(), "session expired") {
		t.Fatalf("error = %q, want session-expired hint", err)
	}
	if cfg.RefreshToken != "" {
		t.Fatal("stored refresh token should have been cleared")
	}
}

// TestDo_Concurrent401sTriggerSingleRefresh verifies the concurrency guard: N
// goroutines racing the same 401 must collectively perform exactly one refresh,
// with the rest reusing the rotated credential.
func TestDo_Concurrent401sTriggerSingleRefresh(t *testing.T) {
	var (
		refreshCalls   atomic.Int32
		protectedCalls atomic.Int32
		okReq          atomic.Int32
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/refresh":
			refreshCalls.Add(1)
			writeJSON(w, http.StatusOK, envelope(t, 200, true, authResponse("jwt-c", "rt-2")))
		case "/api/protected":
			protective := protectedCalls.Add(1)
			// Pre-refresh requests see 401; after the first successful refresh
			// the token "jwt-c" is valid. There is a small race where a request
			// sent before any refresh completes also 401s — fine, the gate
			// collapses those into a single refresh.
			if r.Header.Get("Authorization") != "Bearer jwt-c" && protective <= 8 {
				writeJSON(w, http.StatusUnauthorized, envelope(t, 401, false, nil))
				return
			}
			if r.Header.Get("Authorization") != "Bearer jwt-c" {
				writeJSON(w, http.StatusForbidden, envelope(t, 403, false, nil))
				return
			}
			okReq.Add(1)
			writeJSON(w, http.StatusOK, envelope(t, 200, true, map[string]any{"ok": true}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := newTestConfig(t)
	_ = cfg.SetRefreshToken("rt-1")
	cl := New(srv.URL, cfg, nil)

	const n = 8
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out struct {
				OK bool `json:"ok"`
			}
			errs <- cl.Do(context.Background(), http.MethodGet, "/api/protected", nil, &out)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Do() error = %v", err)
		}
	}
	if refreshCalls.Load() != 1 {
		t.Fatalf("refresh calls = %d, want exactly 1 (single rotation under mutex)", refreshCalls.Load())
	}
	if okReq.Load() != n {
		t.Fatalf("successful protected calls = %d, want %d", okReq.Load(), n)
	}
}

// TestDo_NotAuthenticated surfaces the sentinel before hitting the network when
// no credential exists at all.
func TestDo_NotAuthenticated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("no request should reach the server")
	}))
	defer srv.Close()

	cfg := newTestConfig(t)
	cl := New(srv.URL, cfg, nil)
	err := cl.Do(context.Background(), http.MethodGet, "/api/protected", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "not authenticated") {
		t.Fatalf("err = %v, want not-authenticated sentinel", err)
	}
}

// TestDo_UsesAPIKeyHeader verifies API-key auth bypasses the refresh path and
// sends X-API-Key.
func TestDo_UsesAPIKeyHeader(t *testing.T) {
	var refreshCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/refresh":
			refreshCalls.Add(1)
			writeJSON(w, http.StatusOK, envelope(t, 200, true, authResponse("t", "rt")))
		case "/api/protected":
			if got := r.Header.Get("X-API-Key"); got != "MAIL_abcdef1234" {
				writeJSON(w, http.StatusForbidden, envelope(t, 403, false, nil))
				return
			}
			// Deliberately 401 the API-key call: with an API key set, the
			// client must NOT try to refresh.
			writeJSON(w, http.StatusUnauthorized, envelope(t, 401, false, nil))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := newTestConfig(t)
	_ = cfg.SetRefreshToken("rt-stale") // ignored while API key is set
	cl := New(srv.URL, cfg, nil)
	cl.SetAPIKey("MAIL_abcdef1234")

	err := cl.Do(context.Background(), http.MethodGet, "/api/protected", nil, nil)
	if err == nil {
		t.Fatal("expected 401 to surface")
	}
	if refreshCalls.Load() != 0 {
		t.Fatalf("refresh calls = %d, want 0 (API keys never refresh)", refreshCalls.Load())
	}
}

// TestDo_SurfacesTypedAPIError checks error-code mapping for non-2xx responses.
func TestDo_SurfacesTypedAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := []byte(`{"success":false,"errors":[{"code":"VALIDATION_ERROR","message":"bad local_part"}],"request_id":"req-1"}`)
		writeJSON(w, http.StatusBadRequest, body)
	}))
	defer srv.Close()

	cfg := newTestConfig(t)
	_ = cfg.SetAPIKey("MAIL_k")
	cl := New(srv.URL, cfg, nil)
	var out map[string]any
	err := cl.Do(context.Background(), http.MethodPost, "/api/inbound/addresses", nil, &out)
	if err == nil {
		t.Fatal("expected error")
	}
	ae, ok := err.(*APIError)
	if !ok {
		t.Fatalf("err type = %T, want *APIError", err)
	}
	if ae.Code != "VALIDATION_ERROR" || ae.Message != "bad local_part" || ae.Status != 400 {
		t.Fatalf("unexpected APIError: %+v", ae)
	}
}

// TestDoPublic_NoAuthHeader verifies public calls never attach credentials.
func TestDoPublic_NoAuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, h := range []string{"Authorization", "X-API-Key"} {
			if r.Header.Get(h) != "" {
				writeJSON(w, http.StatusForbidden, envelope(t, 403, false, nil))
				return
			}
		}
		writeJSON(w, http.StatusOK, envelope(t, 200, true, authResponse("t", "rt")))
	}))
	defer srv.Close()

	cfg := newTestConfig(t)
	_ = cfg.SetAPIKey("MAIL_k")
	cl := New(srv.URL, cfg, nil)
	var out AuthResponse
	if err := cl.DoPublic(context.Background(), http.MethodPost, "/api/auth/login", map[string]string{"identifier": "a@b.c"}, &out); err != nil {
		t.Fatalf("DoPublic error = %v", err)
	}
	if out.Token != "t" || out.RefreshToken != "rt" {
		t.Fatalf("unexpected auth response: %+v", out)
	}
}

// TestRefresh_ExpiredClearsAuth directly exercises Client.Refresh when the
// stored refresh token is dead.
func TestRefresh_ExpiredClearsAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusUnauthorized, envelope(t, 401, false, nil))
	}))
	defer srv.Close()

	cfg := newTestConfig(t)
	_ = cfg.SetRefreshToken("rt-dead")
	cl := New(srv.URL, cfg, nil)
	if err := cl.Refresh(context.Background()); err != ErrRefreshExpired {
		t.Fatalf("Refresh() error = %v, want ErrRefreshExpired", err)
	}
	if cfg.RefreshToken != "" {
		t.Fatal("refresh token should have been cleared")
	}
}

// TestApplyAuth_DebugRedaction ensures --debug output never contains secrets.
func TestApplyAuth_DebugRedaction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := []byte(`{"success":true,"data":{"token":"JWT_SUPER_SECRET","refresh_token":"RT_SECRET","user":{"name":"x"}}}`)
		writeJSON(w, http.StatusOK, body)
	}))
	defer srv.Close()

	cfg := newTestConfig(t)
	_ = cfg.SetRefreshToken("rt")
	cl := New(srv.URL, cfg, nil)
	cl.Debug = true
	var buf strings.Builder
	cl.DebugOut = &buf
	_ = cl.Refresh(context.Background())
	out := buf.String()
	for _, secret := range []string{"RT_SECRET", "JWT_SUPER_SECRET"} {
		if strings.Contains(out, secret) {
			t.Fatalf("debug output leaked secret %q:\n%s", secret, out)
		}
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
