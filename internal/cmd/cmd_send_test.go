package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MailAfrica/MailAfrica-CLI/internal/api"
	"github.com/MailAfrica/MailAfrica-CLI/internal/config"
)

func mustOutboundEnv(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))
	t.Setenv(config.EnvAPIURL, "")
	t.Setenv(config.EnvAPIKey, testAPIKey)
}

// newOutboundTestServer fakes the outbound/domains/sandbox surfaces for Phase 3
// command e2e tests. It accepts the env API key only.
func newOutboundTestServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var msgSeq int64
	var domainID int64 = 9
	var tplSeq int64
	var credSeq int64
	var senderID int64

	// failureAttemptWhen > 0 means that numbered POST /emails call fails.
	var failureAttemptWhen atomic.Int32

	writeEnv := func(w http.ResponseWriter, status int, success bool, data any) {
		env := map[string]any{"success": success, "data": data, "request_id": "req-3", "timestamp": "2026-01-02T00:00:00Z"}
		writeJSON(w, status, mustJSON(t, env))
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != testAPIKey {
			writeJSON(w, http.StatusUnauthorized, mustJSON(t, map[string]any{"success": false}))
			return
		}

		path := r.URL.Path
		// ── outbound email ──────────────────────────────────────────────────
		if r.Method == http.MethodPost && path == "/api/outbound/emails" {
			n := msgSeq + 1
			msgSeq = n
			fails := failureAttemptWhen.Load()
			if fails > 0 && int32(n) == fails {
				writeJSON(w, http.StatusBadGateway, mustJSON(t, map[string]any{
					"success": false, "message": "downstream error",
					"errors": []map[string]string{{"code": "PROVIDER_ERROR", "message": "SMTP error"}},
				}))
				return
			}
			var req api.OutboundSendRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			recipients := len(req.To) + len(req.Cc) + len(req.Bcc)
			writeEnv(w, http.StatusCreated, true, map[string]any{
				"id": n, "user_id": 1, "from_address": "noreply@test",
				"to_addresses": req.To, "subject": req.Subject,
				"status": "sent", "amount_tzs": 5 * recipients,
				"provider_message_id": fmt.Sprintf("mta-%d", n),
				"created_at":          "2026-01-02T00:00:00Z",
			})
			return
		}
		if r.Method == http.MethodGet && path == "/api/outbound/emails" {
			writeEnv(w, http.StatusOK, true, []map[string]any{})
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(path, "/api/outbound/emails/") && len(strings.Split(path, "/")) == 5 {
			id := msgSeq
			writeEnv(w, http.StatusOK, true, map[string]any{
				"message":    map[string]any{"id": id, "from_address": "noreply@test", "subject": "hi", "status": "sent"},
				"recipients": []map[string]any{{"recipient": "a@x.com", "status": "sent"}},
			})
			return
		}

		// ── templates ───────────────────────────────────────────────────────
		if r.Method == http.MethodPost && path == "/api/outbound/templates" {
			tplSeq++
			var req api.TemplateRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			writeEnv(w, http.StatusCreated, true, map[string]any{
				"id": tplSeq, "user_id": 1, "name": req.Name, "subject": req.Subject,
				"created_at": "2026-01-02T00:00:00Z", "updated_at": "2026-01-02T00:00:00Z",
			})
			return
		}
		if r.Method == http.MethodGet && path == "/api/outbound/templates" {
			writeEnv(w, http.StatusOK, true, []map[string]any{})
			return
		}
		if r.Method == http.MethodDelete && strings.HasPrefix(path, "/api/outbound/templates/") {
			writeEnv(w, http.StatusOK, true, map[string]any{})
			return
		}

		// ── domains ─────────────────────────────────────────────────────────
		if r.Method == http.MethodPost && path == "/api/domains" {
			domainID++
			writeEnv(w, http.StatusCreated, true, map[string]any{
				"domain": map[string]any{
					"id": domainID, "user_id": 1, "domain": "send.test",
					"purpose": "sending", "status": "pending",
					"from_local_part": "noreply", "created_at": "2026-01-02T00:00:00Z",
				},
				"dns_records": []map[string]string{
					{"type": "TXT", "host": "mail._domainkey.send.test", "value": "v=DKIM1; k=rsa; p=abc123"},
					{"type": "TXT", "host": "send.test", "value": "v=spf1 ip4:1.2.3.4 -all"},
					{"type": "TXT", "host": "_dmarc.send.test", "value": "v=DMARC1; p=reject; rua=mailto:admin@test"},
				},
			})
			return
		}
		if r.Method == http.MethodGet && path == "/api/domains" {
			writeEnv(w, http.StatusOK, true, []map[string]any{
				{"id": domainID, "user_id": 1, "domain": "send.test", "purpose": "sending", "status": "pending",
					"from_local_part": "noreply", "created_at": "2026-01-02T00:00:00Z"},
			})
			return
		}
		if r.Method == http.MethodPost && strings.HasPrefix(path, "/api/domains/") && strings.HasSuffix(path, "/verify") {
			writeEnv(w, http.StatusOK, true, map[string]any{
				"id": domainID, "user_id": 1, "domain": "send.test", "purpose": "sending", "status": "verified",
				"from_local_part": "noreply", "verified_at": "2026-01-02T00:01:00Z", "created_at": "2026-01-02T00:00:00Z",
			})
			return
		}
		if r.Method == http.MethodDelete && strings.HasPrefix(path, "/api/domains/") {
			writeEnv(w, http.StatusOK, true, map[string]any{})
			return
		}

		// ── sender addresses ────────────────────────────────────────────────
		if r.Method == http.MethodPost && strings.HasPrefix(path, "/api/domains/") && strings.HasSuffix(path, "/senders") {
			senderID++
			writeEnv(w, http.StatusCreated, true, map[string]any{
				"id": senderID, "user_id": 1, "domain_id": domainID, "domain": "send.test",
				"local_part": "hello", "created_at": "2026-01-02T00:00:00Z",
			})
			return
		}
		if r.Method == http.MethodGet && path == "/api/domains/senders" {
			writeEnv(w, http.StatusOK, true, []map[string]any{
				{"id": senderID, "user_id": 1, "domain_id": domainID, "domain": "send.test",
					"local_part": "hello", "created_at": "2026-01-02T00:00:00Z"},
			})
			return
		}
		if r.Method == http.MethodDelete && strings.HasPrefix(path, "/api/domains/senders/") {
			writeEnv(w, http.StatusOK, true, nil)
			return
		}

		// ── sandbox ─────────────────────────────────────────────────────────
		if r.Method == http.MethodPost && path == "/api/sandbox/credentials" {
			credSeq++
			writeEnv(w, http.StatusCreated, true, map[string]any{
				"id": credSeq, "user_id": 1, "client_id": "sid-abc", "client_secret": "ssec-xyz",
				"revoked": false, "created_at": "2026-01-02T00:00:00Z",
			})
			return
		}
		if r.Method == http.MethodGet && path == "/api/sandbox/credentials" {
			writeEnv(w, http.StatusOK, true, []map[string]any{
				{"id": credSeq, "user_id": 1, "client_id": "sid-abc", "client_secret": "ssec-xyz",
					"revoked": false, "created_at": "2026-01-02T00:00:00Z"},
			})
			return
		}
		if r.Method == http.MethodGet && path == "/api/sandbox/credentials/smtp" {
			writeEnv(w, http.StatusOK, true, map[string]any{
				"host": "127.0.0.1", "port": 1025, "username": "user", "password": "secret123", "password_set": true,
			})
			return
		}
		if r.Method == http.MethodPost && path == "/api/sandbox/credentials/smtp/regenerate" {
			writeEnv(w, http.StatusOK, true, map[string]any{
				"host": "127.0.0.1", "port": 1025, "username": "user", "password": "newsecret456", "password_set": true,
			})
			return
		}
		if r.Method == http.MethodPost && strings.HasPrefix(path, "/api/sandbox/credentials/") {
			writeEnv(w, http.StatusOK, true, nil)
			return
		}
		if r.Method == http.MethodGet && path == "/api/sandbox/messages" {
			writeEnv(w, http.StatusOK, true, []map[string]any{})
			return
		}

		http.NotFound(w, r)
	}))
	return srv, &failureAttemptWhen
}

func TestCLI_SendEmailSingle(t *testing.T) {
	mustOutboundEnv(t)
	srv, _ := newOutboundTestServer(t)
	defer srv.Close()

	out, err := runCLI("send", "email", "--to", "alice@corp.com,bob@corp.com", "--subject", "hi", "--text-body", "hello", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("send email error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "sent (2 recipients)") || !strings.Contains(out, "10 TZS") {
		t.Fatalf("send email output: %s", out)
	}
}

func TestCLI_SendEmailRejectsOverLimitWithoutAutoChunk(t *testing.T) {
	mustOutboundEnv(t)
	srv, _ := newOutboundTestServer(t)
	defer srv.Close()

	var addrs []string
	for i := 0; i < 51; i++ {
		addrs = append(addrs, fmt.Sprintf("r%d@test.com", i))
	}
	_, err := runCLI("send", "email", "--to", strings.Join(addrs, ","), "--subject", "hi", "--text-body", "x", "--api-url", srv.URL)
	if err == nil {
		t.Fatal("send with 51 recipients must fail without --auto-chunk")
	}
	if !strings.Contains(err.Error(), "auto-chunk") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCLI_SendBatchContract_PartialFailure(t *testing.T) {
	mustOutboundEnv(t)
	srv, failureAt := newOutboundTestServer(t)
	defer srv.Close()
	failureAt.Store(2) // second single-send call fails → chunk 2

	// 120 recipients → 3 chunks of 50,50,20; chunk 2 fails.
	addrs := make([]string, 120)
	for i := range addrs {
		addrs[i] = fmt.Sprintf("r%d@test.com", i)
	}
	_, err := runCLI("send", "batch", "--to", strings.Join(addrs, ","), "--subject", "bulk", "--text-body", "x", "--auto-chunk", "--api-url", srv.URL)
	if err == nil {
		t.Fatal("partial failure must exit non-zero")
	}
	// The runner captures stderr, but the formatted message goes to cmd stdout
	// which runCLI returns; check the error message.
	if !strings.Contains(err.Error(), "failed chunk") {
		t.Fatalf("error should mention failed chunk, got: %v", err)
	}
}

func TestCLI_SendBatchContract_RenderedMessage(t *testing.T) {
	mustOutboundEnv(t)
	srv, failureAt := newOutboundTestServer(t)
	defer srv.Close()
	failureAt.Store(2)

	addrs := make([]string, 120)
	for i := range addrs {
		addrs[i] = fmt.Sprintf("r%d@test.com", i)
	}
	out, err := runCLI("send", "batch", "--to", strings.Join(addrs, ","), "--subject", "bulk", "--text-body", "x", "--auto-chunk", "--api-url", srv.URL)
	if err == nil {
		t.Fatal("partial failure must exit non-zero")
	}
	msg := out + err.Error()
	if !strings.Contains(msg, "sent 50/120 across 3 chunks") || !strings.Contains(msg, "chunk 2 failed") || !strings.Contains(msg, "chunks 3..3 not attempted") {
		t.Fatalf("contract message mismatch: %q", msg)
	}
}

func TestCLI_SendBatchContract_AllFail(t *testing.T) {
	mustOutboundEnv(t)
	srv, failureAt := newOutboundTestServer(t)
	defer srv.Close()
	failureAt.Store(1)

	addrs := make([]string, 10)
	for i := range addrs {
		addrs[i] = fmt.Sprintf("r%d@test.com", i)
	}
	out, err := runCLI("send", "batch", "--to", strings.Join(addrs, ","), "--subject", "fail", "--text-body", "x", "--auto-chunk", "--api-url", srv.URL)
	if err == nil {
		t.Fatal("first chunk failure must exit non-zero")
	}
	msg := out + err.Error()
	if !strings.Contains(msg, "sent 0/10 across 1 chunks") || !strings.Contains(msg, "chunk 1 failed") {
		t.Fatalf("first chunk failure message: %q", msg)
	}
}

func TestCLI_SendBatchContract_LastChunkFail(t *testing.T) {
	mustOutboundEnv(t)
	srv, failureAt := newOutboundTestServer(t)
	defer srv.Close()
	// 70 recipients → chunks 50/20; chunk 2 fails → "chunks 3..2" should NOT appear
	failureAt.Store(2)

	addrs := make([]string, 70)
	for i := range addrs {
		addrs[i] = fmt.Sprintf("r%d@test.com", i)
	}
	out, err := runCLI("send", "batch", "--to", strings.Join(addrs, ","), "--subject", "fail", "--text-body", "x", "--auto-chunk", "--api-url", srv.URL)
	if err == nil {
		t.Fatal("last chunk failure must exit non-zero")
	}
	msg := out + err.Error()
	if strings.Contains(msg, "not attempted") {
		t.Fatalf("no 'chunks not attempted' suffix when last chunk fails: %q", msg)
	}
	if !strings.Contains(msg, "sent 50/70 across 2 chunks") || !strings.Contains(msg, "chunk 2 failed") {
		t.Fatalf("last chunk failure message: %q", msg)
	}
}

func TestCLI_DomainEndToEnd(t *testing.T) {
	mustOutboundEnv(t)
	srv, _ := newOutboundTestServer(t)
	defer srv.Close()

	out, err := runCLI("domain", "add", "--domain", "send.test", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("domain add error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "domain added: send.test") || !strings.Contains(out, "DKIM") || !strings.Contains(out, "SPF") || !strings.Contains(out, "DMARC") {
		t.Fatalf("domain add should print named DNS records: %s", out)
	}
	if !strings.Contains(out, "mail._domainkey.send.test") {
		t.Fatalf("DKIM host missing: %s", out)
	}

	if out, err = runCLI("domain", "list", "--api-url", srv.URL); err != nil {
		t.Fatalf("domain list error = %v", err)
	}
	if !strings.Contains(out, "send.test") || !strings.Contains(out, "pending") {
		t.Fatalf("domain list output: %s", out)
	}

	if out, err = runCLI("domain", "verify", "9", "--api-url", srv.URL); err != nil {
		t.Fatalf("domain verify error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "verified") {
		t.Fatalf("domain verify output: %s", out)
	}

	if out, err = runCLI("domain", "sender", "create", "--domain-id", "9", "--local-part", "hello", "--api-url", srv.URL); err != nil {
		t.Fatalf("sender create error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "hello@send.test") {
		t.Fatalf("sender create output: %s", out)
	}

	if out, err = runCLI("domain", "sender", "list", "--api-url", srv.URL); err != nil {
		t.Fatalf("sender list error = %v", err)
	}
	if !strings.Contains(out, "hello@send.test") {
		t.Fatalf("sender list output: %s", out)
	}

	if _, err = runCLI("domain", "sender", "delete", "1", "--api-url", srv.URL); err != nil {
		t.Fatalf("sender delete error = %v", err)
	}
	if _, err = runCLI("domain", "delete", "9", "--api-url", srv.URL); err != nil {
		t.Fatalf("domain delete error = %v", err)
	}
}

func TestCLI_SandboxEndToEnd(t *testing.T) {
	mustOutboundEnv(t)
	srv, _ := newOutboundTestServer(t)
	defer srv.Close()

	out, err := runCLI("sandbox", "credential", "create", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("credential create error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "ssec-xyz") {
		t.Fatalf("credential create should reveal secret once: %s", out)
	}

	out, err = runCLI("sandbox", "credential", "list", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("credential list error = %v", err)
	}
	if strings.Contains(out, "ssec-xyz") {
		t.Fatalf("credential list must not leak full secret: %s", out)
	}

	out, err = runCLI("sandbox", "smtp", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("smtp error = %v", err)
	}
	if !strings.Contains(out, "secret123") {
		t.Fatalf("smtp output should show password: %s", out)
	}

	out, err = runCLI("sandbox", "smtp-regenerate", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("smtp-regenerate error = %v", err)
	}
	if !strings.Contains(out, "newsecret456") {
		t.Fatalf("smtp-regenerate should show new password: %s", out)
	}
}

func TestCLI_SendTemplates(t *testing.T) {
	mustOutboundEnv(t)
	srv, _ := newOutboundTestServer(t)
	defer srv.Close()

	out, err := runCLI("send", "template", "create", "--name", "Welcome", "--subject", "Hi {{name}}", "--html-body", "<p>Hello</p>", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("template create error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "template created") {
		t.Fatalf("template create output: %s", out)
	}

	out, err = runCLI("send", "template", "list", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("template list error = %v", err)
	}
	if !strings.Contains(out, "no templates") {
		t.Fatalf("expected no templates message, got: %s", out)
	}

	if _, err = runCLI("send", "template", "delete", "1", "--api-url", srv.URL); err != nil {
		t.Fatalf("template delete error = %v", err)
	}
}

func TestCLI_SendList(t *testing.T) {
	mustOutboundEnv(t)
	srv, _ := newOutboundTestServer(t)
	defer srv.Close()

	out, err := runCLI("send", "list", "--api-url", srv.URL)
	if err != nil {
		t.Fatalf("send list error = %v", err)
	}
	if !strings.Contains(out, "no sent emails") {
		t.Fatalf("send list output: %s", out)
	}
}

func TestCLI_SendJSONOutput(t *testing.T) {
	mustOutboundEnv(t)
	srv, _ := newOutboundTestServer(t)
	defer srv.Close()

	out, err := runCLI("send", "email", "--to", "a@b.com", "--subject", "x", "--text-body", "y", "--api-url", srv.URL, "--json")
	if err != nil {
		t.Fatalf("send email --json error = %v\n%s", err, out)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("not json: %v\n%s", err, out)
	}
	if parsed["status"] != "sent" {
		t.Fatalf("json output status: %v", parsed["status"])
	}
}
