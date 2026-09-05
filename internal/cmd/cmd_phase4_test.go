package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newPhase4TestServer fakes the billing/compliance/sms/agent surface. It
// records the last received agent/config PUT body so tests can assert that only
// changed fields were sent.
func newPhase4TestServer(t *testing.T) (*httptest.Server, *map[string]any) {
	t.Helper()

	lastAgentPut := map[string]any{}
	writeEnv := func(w http.ResponseWriter, status int, success bool, data any, message string) {
		env := map[string]any{"success": success, "data": data, "request_id": "req-4", "timestamp": "2026-01-02T00:00:00Z"}
		if message != "" {
			env["message"] = message
		}
		writeJSON(w, status, mustJSON(t, env))
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != testAPIKey {
			writeJSON(w, http.StatusUnauthorized, mustJSON(t, map[string]any{"success": false}))
			return
		}
		path := r.URL.Path

		// ── billing ─────────────────────────────────────────────────────────
		if r.Method == http.MethodGet && path == "/api/billing/balance" {
			writeEnv(w, http.StatusOK, true, map[string]any{"balance_tzs": 12345}, "")
			return
		}
		if r.Method == http.MethodPost && path == "/api/billing/topup" {
			writeEnv(w, http.StatusCreated, true, map[string]any{
				"topup": map[string]any{"id": 42, "user_id": 7, "amount_tzs": 10000, "status": "pending",
					"provider_reference": "snip_abc", "created_at": "2026-01-02T00:00:00Z"},
				"checkout_url":       "https://pay.snippe.sh/co/42",
				"payment_link_url":   "https://pay.snippe.sh/link/42",
				"provider_reference": "snip_abc",
			}, "checkout session created")
			return
		}
		if r.Method == http.MethodPost && path == "/api/billing/topup/phone" {
			writeEnv(w, http.StatusCreated, true, map[string]any{
				"topup": map[string]any{"id": 43, "user_id": 7, "amount_tzs": 10000, "status": "pending",
					"provider_reference": "snip_xyz", "created_at": "2026-01-02T00:00:00Z"},
				"provider_reference": "snip_xyz",
			}, "payment request sent to your phone")
			return
		}

		// ── compliance ──────────────────────────────────────────────────────
		if r.Method == http.MethodGet && path == "/api/compliance/profile" {
			writeEnv(w, http.StatusOK, true, map[string]any{
				"id": 1, "user_id": 7, "pdpc_registered": true,
				"pdpc_certificate_number": "PDPC/2026/001",
				"pdpc_registered_at":      "2026-01-15T00:00:00Z",
				"default_retention_days":  30,
				"data_consent_at":         "2026-01-01T00:00:00Z",
				"privacy_policy_version":  "v1.0",
				"updated_at":              "2026-01-02T00:00:00Z",
			}, "")
			return
		}
		if r.Method == http.MethodPatch && path == "/api/compliance/profile" {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if _, ok := body["pdpc_registered_at"]; ok {
				writeEnv(w, http.StatusOK, true, map[string]any{
					"id": 1, "user_id": 7, "pdpc_registered": true,
					"pdpc_certificate_number": "PDPC/2026/002",
					"default_retention_days":  60,
					"updated_at":              "2026-01-02T00:00:00Z",
				}, "")
				return
			}
			writeEnv(w, http.StatusOK, true, map[string]any{
				"id": 1, "user_id": 7, "pdpc_registered": body["pdpc_registered"],
				"default_retention_days": body["default_retention_days"],
				"updated_at":             "2026-01-02T00:00:00Z",
			}, "")
			return
		}
		if r.Method == http.MethodGet && path == "/api/compliance/audit-export" {
			writeEnv(w, http.StatusOK, true, map[string]any{
				"pdpc_registered": true, "pdpc_certificate_number": "PDPC/2026/002",
				"default_retention_days": 60, "address_count": 3, "message_count": 154,
				"generated_at": "2026-01-02T00:00:00Z",
			}, "")
			return
		}

		// ── sms ─────────────────────────────────────────────────────────────
		if r.Method == http.MethodPost && path == "/api/sms/notifications" {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			phone := body["phone_number"].(string)
			if !strings.HasPrefix(phone, "+255") {
				phone = "+255" + strings.TrimLeft(phone, "0")
			}
			writeEnv(w, http.StatusCreated, true, map[string]any{
				"id": 9, "address_id": 3, "phone_number": phone, "is_active": true,
				"created_at": "2026-01-02T00:00:00Z", "api_key": "SDK_secret_x",
			}, "sms notification created — save the api_key now, it won't be shown again")
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(path, "/api/sms/notifications/") && strings.HasSuffix(path, "/deliveries") {
			writeEnv(w, http.StatusOK, true, []map[string]any{
				{"id": 21, "sms_notification_id": 9, "message_id": 55, "status": "sent",
					"provider_message_id": "SDK-msg-abc", "attempt": 1, "created_at": "2026-01-02T00:00:00Z"},
			}, "")
			return
		}
		if r.Method == http.MethodPost && strings.HasPrefix(path, "/api/sms/notifications/") {
			writeEnv(w, http.StatusOK, true, nil, "sms notification revoked")
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(path, "/api/sms/notifications") {
			writeEnv(w, http.StatusOK, true, []map[string]any{
				{"id": 9, "address_id": 3, "phone_number": "+255712345678", "is_active": true, "created_at": "2026-01-02T00:00:00Z"},
			}, "")
			return
		}

		// ── agent ───────────────────────────────────────────────────────────
		if r.Method == http.MethodGet && path == "/api/agent/configs" {
			writeEnv(w, http.StatusOK, true, []map[string]any{
				{"address_id": 3, "user_id": 7, "mode": "auto", "persona": "Be terse", "enabled": true,
					"reply_from_domain_id": 11, "reply_from_address": "support@example.com",
					"updated_at": "2026-01-02T00:00:00Z"},
			}, "")
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(path, "/api/agent/configs/") {
			// default zero-config for an address with no saved config
			writeEnv(w, http.StatusOK, true, map[string]any{
				"address_id": 3, "user_id": 7, "mode": "off", "enabled": true,
				"updated_at": "0001-01-01T00:00:00Z",
			}, "")
			return
		}
		if r.Method == http.MethodPut && strings.HasPrefix(path, "/api/agent/configs/") {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			lastAgentPut = body
			cfg := map[string]any{
				"address_id": 3, "user_id": 7, "mode": body["mode"], "enabled": true,
				"updated_at": "2026-01-02T01:00:00Z",
			}
			if p, ok := body["persona"]; ok {
				cfg["persona"] = p
			}
			writeEnv(w, http.StatusOK, true, cfg, "")
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(path, "/draft") {
			writeEnv(w, http.StatusOK, true, map[string]any{
				"draft": "Thanks for your note! Our team will get back to you shortly.",
			}, "")
			return
		}

		http.NotFound(w, r)
	}))
	return srv, &lastAgentPut
}

func TestCLI_Wallet(t *testing.T) {
	mustOutboundEnv(t)
	srv, _ := newPhase4TestServer(t)
	defer srv.Close()

	out, err := runCLI("wallet", "balance", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("wallet balance error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "12345 TZS") {
		t.Fatalf("balance output: %s", out)
	}

	if _, err := runCLI("wallet", "topup", "--amount", "100", "--api-url", srv.URL); err == nil {
		t.Fatal("topup below 2000 TZS must be rejected")
	}

	out, err = runCLI("wallet", "topup", "--amount", "10000", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("topup error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "https://pay.snippe.sh/co/42") || !strings.Contains(out, "checkout") {
		t.Fatalf("topup should print the hosted checkout url: %s", out)
	}

	out, err = runCLI("wallet", "topup", "--amount", "10000", "--via", "phone", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("topup phone error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "snip_xyz") {
		t.Fatalf("phone topup should print provider reference: %s", out)
	}
}

func TestCLI_Compliance(t *testing.T) {
	mustOutboundEnv(t)
	srv, _ := newPhase4TestServer(t)
	defer srv.Close()

	out, err := runCLI("compliance", "profile", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("compliance profile error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "PDPC/2026/001") || !strings.Contains(out, "30") {
		t.Fatalf("compliance profile output: %s", out)
	}

	out, err = runCLI("compliance", "update", "--pdpc", "--registered-at", "2026-02-20", "--retention-days", "60", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("compliance update error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "PDPC/2026/002") {
		t.Fatalf("compliance update output: %s", out)
	}

	if _, err := runCLI("compliance", "update", "--registered-at", "20/02/2026", "--api-url", srv.URL); err == nil {
		t.Fatal("bad YYYY-MM-DD must be rejected")
	}

	out, err = runCLI("compliance", "audit", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("compliance audit error = %v", err)
	}
	if !strings.Contains(out, "154") || !strings.Contains(out, "3") {
		t.Fatalf("audit export output: %s", out)
	}
}

func TestCLI_SMSNotifications(t *testing.T) {
	mustOutboundEnv(t)
	srv, _ := newPhase4TestServer(t)
	defer srv.Close()

	out, err := runCLI("sms", "notification", "create", "--address-id", "3", "--phone", "0712345678",
		"--sendafrica-key", "SDK_input", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("sms create error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "SDK_secret_x") {
		t.Fatalf("sms create must reveal the provider key once: %s", out)
	}
	if !strings.Contains(out, "+255712345678") {
		t.Fatalf("sms create should normalize phone: %s", out)
	}

	out, err = runCLI("sms", "notification", "list", "--address-id", "3", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("sms list error = %v", err)
	}
	if strings.Contains(out, "SDK_secret_x") {
		t.Fatalf("sms list must not contain provider key: %s", out)
	}

	if _, err := runCLI("sms", "notification", "revoke", "9", "--api-url", srv.URL); err != nil {
		t.Fatalf("sms revoke error = %v", err)
	}

	out, err = runCLI("sms", "notification", "deliveries", "9", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("sms deliveries error = %v", err)
	}
	if !strings.Contains(out, "sent") {
		t.Fatalf("sms deliveries output: %s", out)
	}
}

func TestCLI_AgentConfigRead(t *testing.T) {
	mustOutboundEnv(t)
	srv, _ := newPhase4TestServer(t)
	defer srv.Close()

	out, err := runCLI("agent", "config", "3", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("agent config error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "mode") || !strings.Contains(out, "off") {
		t.Fatalf("agent config read output: %s", out)
	}

	out, err = runCLI("agent", "list", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("agent list error = %v", err)
	}
	if !strings.Contains(out, "auto") || !strings.Contains(out, "support@example.com") {
		t.Fatalf("agent list output: %s", out)
	}
}

func TestCLI_AgentConfigWriteOnlyChangedFields(t *testing.T) {
	mustOutboundEnv(t)
	srv, lastPut := newPhase4TestServer(t)
	defer srv.Close()

	_, err := runCLI("agent", "config", "3", "--mode", "draft", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("agent config write error = %v", err)
	}
	// Only mode given → only mode sent.
	if mode, _ := (*lastPut)["mode"].(string); mode != "draft" {
		t.Fatalf("agent put mode = %v", lastPut)
	}
	if _, ok := (*lastPut)["persona"]; ok {
		t.Fatalf("persona must not be sent when not passed: %v", lastPut)
	}

	_, err = runCLI("agent", "config", "3", "--mode", "auto", "--persona", "Be concise", "--reply-from-domain-id", "11", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("agent config write with persona error = %v", err)
	}
	if (*lastPut)["persona"] != "Be concise" {
		t.Fatalf("persona not persisted: %v", lastPut)
	}
	if (*lastPut)["reply_from_domain_id"].(float64) != 11 {
		t.Fatalf("reply_from_domain_id not persisted: %v", lastPut)
	}

	if _, err := runCLI("agent", "config", "3", "--mode", "wild", "--api-url", srv.URL); err == nil {
		t.Fatal("invalid mode must be rejected client-side")
	}
}

func TestCLI_AgentDraftNeverSends(t *testing.T) {
	mustOutboundEnv(t)
	srv, _ := newPhase4TestServer(t)
	defer srv.Close()

	out, err := runCLI("agent", "draft", "3", "--subject", "Pricing", "--text-body", "How much?", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("agent draft error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "Thanks for your note") {
		t.Fatalf("agent draft output: %s", out)
	}
	if !strings.Contains(out, "nothing was sent") {
		t.Fatalf("agent draft must make clear nothing was sent: %s", out)
	}

	if _, err := runCLI("agent", "draft", "3", "--api-url", srv.URL); err == nil {
		t.Fatal("agent draft without subject/text must be rejected")
	}
}
