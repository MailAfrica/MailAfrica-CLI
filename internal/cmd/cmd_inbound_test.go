package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MailAfrica/MailAfrica-CLI/internal/api"
	"github.com/MailAfrica/MailAfrica-CLI/internal/config"
)

// newInboundTestServer fakes the inbound + webhook surface. Only the env API
// key is accepted, so the tests exercise authentication exactly as scripts do.
// The returned settable bool flips the inbound domain's DNS verification state.
func newInboundTestServer(t *testing.T) (*httptest.Server, *bool) {
	t.Helper()

	addrID := int64(7)
	domainID := int64(43)
	webhookID := int64(5)
	deliveryID := int64(9)
	verified := false

	writeEnv := func(w http.ResponseWriter, status int, success bool, data any, pag *map[string]int) {
		env := map[string]any{"success": success, "data": data, "request_id": "req-2", "timestamp": "2026-01-01T00:00:00Z"}
		if pag != nil {
			env["pagination"] = *pag
		}
		writeJSON(w, status, mustJSON(t, env))
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != testAPIKey {
			writeJSON(w, http.StatusUnauthorized, mustJSON(t, map[string]any{"success": false, "errors": []map[string]string{{"code": "UNAUTHENTICATED", "message": "invalid or missing key"}}}))
			return
		}

		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/inbound/addresses":
			var body api.CreateInboundAddressRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			addrID++
			writeEnv(w, http.StatusCreated, true, map[string]any{
				"id": addrID, "user_id": 1, "local_part": body.LocalPart,
				"retention_days": 30, "created_at": "2026-01-01T00:00:00Z",
			}, nil)

		case r.Method == http.MethodGet && r.URL.Path == "/api/inbound/addresses":
			writeEnv(w, http.StatusOK, true, []map[string]any{{
				"id": addrID, "user_id": 1, "local_part": "invoices",
				"retention_days": 30, "created_at": "2026-01-01T00:00:00Z",
			}}, nil)

		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/inbound/addresses/"):
			writeEnv(w, http.StatusOK, true, nil, nil)

		case r.Method == http.MethodPost && r.URL.Path == "/api/inbound/domains":
			var body api.CreateInboundDomainRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			writeEnv(w, http.StatusCreated, true, map[string]any{
				"domain":              map[string]any{"id": domainID, "user_id": 1, "domain": body.Domain, "verification_token": "token-xyz"},
				"verification_record": map[string]any{"type": "TXT", "host": "_mailafrica." + body.Domain, "value": "token-xyz"},
			}, nil)

		case r.Method == http.MethodGet && r.URL.Path == "/api/inbound/domains":
			state := map[string]any{"id": domainID, "user_id": 1, "domain": "in.example.com", "created_at": "2026-01-01T00:00:00Z"}
			if verified {
				state["verified_at"] = "2026-01-02T00:00:00Z"
			}
			writeEnv(w, http.StatusOK, true, []map[string]any{state}, nil)

		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/inbound/domains/") && strings.HasSuffix(r.URL.Path, "/verify"):
			if !verified {
				writeJSON(w, http.StatusOK, mustJSON(t, map[string]any{"success": false, "errors": []map[string]string{{"code": "PENDING", "message": "txt record not found"}}}))
				return
			}
			writeEnv(w, http.StatusOK, true, map[string]any{"id": domainID, "domain": "in.example.com", "verified_at": "2026-01-02T00:00:00Z"}, nil)

		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/inbound/domains/"):
			writeEnv(w, http.StatusOK, true, nil, nil)

		case r.Method == http.MethodGet && r.URL.Path == "/api/inbound/messages":
			msgs := []map[string]any{
				{"id": 11, "address_id": 7, "from_addr": "alice@corp.com", "to_addr": "invoices@mailafrica.online", "subject": "Invoice", "is_read": false, "received_at": "2026-01-02T00:00:00Z"},
				{"id": 10, "address_id": 7, "from_addr": "bob@corp.com", "to_addr": "invoices@mailafrica.online", "subject": "Receipt", "is_read": true, "received_at": "2026-01-01T00:00:00Z"},
			}
			writeEnv(w, http.StatusOK, true, msgs, &map[string]int{"page": 1, "per_page": 2, "total": 2, "total_pages": 1})

		case strings.HasPrefix(r.URL.Path, "/api/inbound/messages/") && strings.HasSuffix(r.URL.Path, "/read"):
			writeEnv(w, http.StatusOK, true, nil, nil)

		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/api/inbound/messages/"):
			writeEnv(w, http.StatusOK, true, nil, nil)

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/inbound/messages/"):
			writeEnv(w, http.StatusOK, true, map[string]any{
				"id": 11, "address_id": 7, "from_addr": "alice@corp.com", "to_addr": "invoices@mailafrica.online",
				"subject": "Invoice", "text_body": "Dear customer, you owe us.", "is_read": false, "received_at": "2026-01-02T00:00:00Z",
			}, nil)

		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/webhook/webhooks/") && len(parts) == 3:
			webhookID++
			writeEnv(w, http.StatusCreated, true, map[string]any{
				"id": webhookID, "address_id": 7, "url": "https://app.example.com/mail", "secret": "whsec_abcdef1234567890", "is_active": true, "created_at": "2026-01-01T00:00:00Z",
			}, nil)

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/webhook/webhooks/") && len(parts) == 3:
			writeEnv(w, http.StatusOK, true, []map[string]any{{
				"id": webhookID, "address_id": 7, "url": "https://app.example.com/mail", "secret": "whsec_abcdef1234567890", "is_active": true, "created_at": "2026-01-01T00:00:00Z",
			}}, nil)

		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/webhook/webhooks/"):
			writeEnv(w, http.StatusOK, true, nil, nil)

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/webhook/webhooks/") && strings.HasSuffix(r.URL.Path, "/deliveries"):
			deliveryID++
			writeEnv(w, http.StatusOK, true, []map[string]any{{
				"id": deliveryID, "webhook_id": 5, "message_id": 11, "status_code": 200, "attempt": 1,
				"status": "delivered", "delivered_at": "2026-01-02T00:00:00Z", "created_at": "2026-01-02T00:00:00Z",
			}}, nil)

		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/webhook/webhooks/") && strings.HasSuffix(r.URL.Path, "/test"):
			writeEnv(w, http.StatusOK, true, map[string]any{"status_code": http.StatusOK}, nil)

		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/webhook/trigger/"):
			writeEnv(w, http.StatusOK, true, nil, nil)

		default:
			http.NotFound(w, r)
		}
	}))
	return srv, &verified
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func writeJSON(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func phase2Env(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))
	t.Setenv(config.EnvAPIURL, "")
	t.Setenv(config.EnvAPIKey, testAPIKey)
}

// TestCLI_InboundEndToEnd covers address/domain/message commands against the
// fake inbound surface, including the PENDING verify handshake.
func TestCLI_InboundEndToEnd(t *testing.T) {
	phase2Env(t)
	srv, dnsReady := newInboundTestServer(t)
	defer srv.Close()

	out, err := runCLI("inbound", "address", "create", "--local-part", "invoices", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("address create error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "address created: 8") || !strings.Contains(out, "@mailafrica.online") {
		t.Fatalf("address create output: %s", out)
	}

	if out, err = runCLI("inbound", "address", "list", "--api-url", srv.URL); err != nil {
		t.Fatalf("address list error = %v", err)
	}
	if !strings.Contains(out, "invoices@mailafrica.online") {
		t.Fatalf("address list output: %s", out)
	}

	// Reject invalid local parts client-side before any request.
	if _, err = runCLI("inbound", "address", "create", "--local-part", "Bad_Part!", "--api-url", srv.URL); err == nil {
		t.Fatal("invalid local-part should be rejected")
	}

	if out, err = runCLI("inbound", "domain", "add", "--domain", "in.example.com", "--api-url", srv.URL); err != nil {
		t.Fatalf("domain add error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "_mailafrica.in.example.com") || !strings.Contains(out, "token-xyz") {
		t.Fatalf("domain add should print DNS records, got: %s", out)
	}

	// Verify before DNS is ready → PENDING error; after → success.
	if _, err = runCLI("inbound", "domain", "verify", "43", "--api-url", srv.URL); err == nil {
		t.Fatal("verify should fail while DNS pending")
	}
	if !strings.Contains(err.Error(), "not verified yet") {
		t.Fatalf("pending verify error = %v", err)
	}
	*dnsReady = true
	if out, err = runCLI("inbound", "domain", "verify", "43", "--api-url", srv.URL); err != nil {
		t.Fatalf("verify after DNS ready error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "verified") {
		t.Fatalf("verify output: %s", out)
	}

	if out, err = runCLI("inbound", "message", "list", "--address-id", "7", "--api-url", srv.URL); err != nil {
		t.Fatalf("message list error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "Alice") && !strings.Contains(out, "alice@corp.com") {
		t.Fatalf("message list should show senders: %s", out)
	}
	if !strings.Contains(out, "page 1 of 1") {
		t.Fatalf("message list should print pagination footer: %s", out)
	}

	if out, err = runCLI("inbound", "message", "get", "11", "--api-url", srv.URL); err != nil {
		t.Fatalf("message get error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "you owe us") {
		t.Fatalf("message get should include body: %s", out)
	}

	if out, err = runCLI("inbound", "message", "read", "11", "--api-url", srv.URL); err != nil {
		t.Fatalf("message read error = %v", err)
	}
	if !strings.Contains(out, "11 marked read") {
		t.Fatalf("message read output: %s", out)
	}

	if _, err = runCLI("inbound", "address", "delete", "8", "--api-url", srv.URL); err != nil {
		t.Fatalf("address delete error = %v", err)
	}
}

// TestCLI_WebhookEndToEnd covers the webhook surface and verifies the one-time
// secret is shown at create but masked in listings.
func TestCLI_WebhookEndToEnd(t *testing.T) {
	phase2Env(t)
	srv, _ := newInboundTestServer(t)
	defer srv.Close()

	out, err := runCLI("webhook", "create", "--address-id", "7", "--url", "https://app.example.com/mail", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("webhook create error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "whsec_abcdef1234567890") {
		t.Fatalf("webhook create should reveal the full secret once: %s", out)
	}

	out, err = runCLI("webhook", "list", "--address-id", "7", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("webhook list error = %v\n%s", err, out)
	}
	if strings.Contains(out, "whsec_abcdef1234567890") {
		t.Fatalf("webhook list must not leak the full secret: %s", out)
	}
	if !strings.Contains(out, "whsec_ab") && !strings.Contains(out, "whsec_") {
		t.Fatalf("webhook list should still show a masked secret prefix: %s", out)
	}

	if out, err = runCLI("webhook", "test", "5", "--api-url", srv.URL); err != nil {
		t.Fatalf("webhook test error = %v", err)
	}
	if !strings.Contains(out, "HTTP 200") {
		t.Fatalf("webhook test output: %s", out)
	}

	if out, err = runCLI("webhook", "deliveries", "5", "--api-url", srv.URL); err != nil {
		t.Fatalf("webhook deliveries error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "delivered") {
		t.Fatalf("webhook deliveries output: %s", out)
	}

	if out, err = runCLI("webhook", "trigger", "11", "--api-url", srv.URL); err != nil {
		t.Fatalf("webhook trigger error = %v", err)
	}
	if !strings.Contains(out, "queued") {
		t.Fatalf("webhook trigger output: %s", out)
	}

	if _, err = runCLI("webhook", "delete", "5", "--api-url", srv.URL); err != nil {
		t.Fatalf("webhook delete error = %v", err)
	}

	// Required-flag guard rails.
	if _, err = runCLI("webhook", "list", "--api-url", srv.URL); err == nil {
		t.Fatal("webhook list without --address-id should fail")
	}
	if _, err = runCLI("webhook", "create", "--address-id", "7", "--api-url", srv.URL); err == nil {
		t.Fatal("webhook create without --url should fail")
	}
}

func TestCLI_InboundJSONOutput(t *testing.T) {
	phase2Env(t)
	srv, _ := newInboundTestServer(t)
	defer srv.Close()

	out, err := runCLI("inbound", "message", "list", "--address-id", "7", "--api-url", srv.URL, "--json")
	if err != nil {
		t.Fatalf("message list --json error = %v\n%s", err, out)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("message list --json not JSON: %v\n%s", err, out)
	}
	if _, ok := parsed["messages"]; !ok {
		t.Fatalf("JSON output should include a messages key: %v", parsed)
	}
	if parsed["pagination"] == nil {
		t.Fatalf("JSON output should include pagination: %v", parsed)
	}
}

func TestCLI_WebhookJSONOutput(t *testing.T) {
	phase2Env(t)
	srv, _ := newInboundTestServer(t)
	defer srv.Close()

	out, err := runCLI("webhook", "create", "--address-id", "7", "--url", "https://app.example.com/mail", "--api-url", srv.URL, "--json")
	if err != nil {
		t.Fatalf("webhook create --json error = %v\n%s", err, out)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("webhook create --json not JSON: %v\n%s", err, out)
	}
	if parsed["secret"] != "whsec_abcdef1234567890" {
		t.Fatalf("--json should include the webhook secret for the consumer: %v", parsed["secret"])
	}
}
