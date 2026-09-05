package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/MailAfrica/MailAfrica-CLI/internal/config"
)

// ClientVersion identifies the CLI to the API via the User-Agent header.
const ClientVersion = "mailafrica-cli"

// Client is a MailAfrica API client. One command invocation runs a single
// in-flight request at a time; see the invariant note below.
//
// Concurrency invariant: the CLI assumes one in-flight request per process.
// The refresh path is nevertheless serialized by refreshMu and guarded by a
// generation counter, so if a future phase adds parallelism (e.g. chunked
// batch sends) two concurrent 401s can never rotate the same refresh token
// twice: the second caller waits for the first refresh and reuses the already
// rotated credential instead of refreshing again. Do not remove the mutex
// without also restricting the caller to single-threaded request issuance.
type Client struct {
	BaseURL string
	HTTP    *http.Client

	cfg *config.Config

	mu        sync.Mutex
	refreshMu sync.Mutex
	// accessToken is the in-process JWT. It is never persisted; each process
	// obtains it either from auth login or by refreshing the stored refresh
	// token on demand.
	accessToken string
	// refreshGeneration increments each time a refresh actually rotates the
	// token, letting waiters behind refreshMu detect an already-finished
	// refresh and skip their own. Atomic so the read at request-build time
	// never races with the write under refreshMu.
	refreshGeneration atomic.Int32

	// overrideAPIKey wins over env/config when set (via --api-key).
	overrideAPIKey string

	// Debug writes redacted request/response exchanges to DebugOut.
	Debug    bool
	DebugOut io.Writer
}

// New builds a client. cfg is shared with the caller so token rotation
// persists back through the same config file the commands read.
func New(baseURL string, cfg *config.Config, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &Client{
		BaseURL:  strings.TrimRight(baseURL, "/"),
		HTTP:     httpClient,
		cfg:      cfg,
		DebugOut: io.Discard,
	}
}

// SetAPIKey overrides the effective API key for this process (--api-key flag).
func (c *Client) SetAPIKey(k string) {
	c.overrideAPIKey = k
}

// Do issues an authenticated request, parsing the envelope data into out.
// It transparently handles a 401 by refreshing the stored refresh token and
// retrying exactly once before giving up.
func (c *Client) Do(ctx context.Context, method, path string, body any, out any) error {
	return c.do(ctx, method, path, body, out, true, true)
}

// DoPublic issues an unauthenticated request (login, refresh, email-verify).
func (c *Client) DoPublic(ctx context.Context, method, path string, body any, out any) error {
	return c.do(ctx, method, path, body, out, false, false)
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any, authed, canRefresh bool) error {
	if authed && !c.hasCredentials() {
		return ErrNotAuthenticated
	}

	attempts := 0
	for {
		// Capture the refresh generation the moment this attempt's credentials
		// are chosen. If a concurrent caller refreshes while this request is in
		// flight, that generation differs and we reuse the rotated credential
		// instead of refreshing a stale token a second time.
		gen := int(c.refreshGeneration.Load())
		env, status, err := c.roundTrip(ctx, method, path, body, authed)
		if err != nil {
			return err
		}

		if status == http.StatusUnauthorized && authed && canRefresh && attempts == 0 && !c.hasAPIKey() {
			willRefresh := c.storedRefreshToken() != ""
			if !willRefresh {
				// We have credentials but nothing refreshable (bare JWT). Don't
				// clear anything; surface the 401 as-is.
				return apiErrorFrom(env, status)
			}
			if err := c.refreshIfNeeded(ctx, gen); err != nil {
				return err
			}
			attempts++
			continue
		}

		if status >= http.StatusBadRequest {
			return apiErrorFrom(env, status)
		}
		if out != nil && len(bytes.TrimSpace(env.Data)) > 0 && !bytes.Equal(bytes.TrimSpace(env.Data), []byte("null")) {
			if err := json.Unmarshal(env.Data, out); err != nil {
				return fmt.Errorf("parse response for %s: %w", path, err)
			}
		}
		return nil
	}
}

// refreshIfNeeded rotates the refresh token unless another goroutine already
// did so while this caller waited for refreshMu.
func (c *Client) refreshIfNeeded(ctx context.Context, gen int) error {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	if gen != int(c.refreshGeneration.Load()) && c.accessToken != "" {
		// Refreshed by a concurrent caller; reuse the rotated credential.
		return nil
	}
	if err := c.refresh(ctx); err != nil {
		return err
	}
	c.refreshGeneration.Add(1)
	return nil
}

// Refresh rotates the stored refresh token now (the `mailafrica auth refresh`
// path). On success the in-process access token is set and the rotated refresh
// token persisted.
func (c *Client) Refresh(ctx context.Context) error {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()
	return c.refresh(ctx)
}

func (c *Client) refresh(ctx context.Context) error {
	rt := c.cfg.GetRefreshToken()
	if rt == "" {
		return ErrNotAuthenticated
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(map[string]string{"refresh_token": rt}); err != nil {
		return fmt.Errorf("encode refresh request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/auth/refresh", &buf)
	if err != nil {
		return fmt.Errorf("build refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", ClientVersion)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("refresh session: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized {
		// Expired or revoked refresh token: stop pretending we have a session.
		_ = c.cfg.SetRefreshToken("")
		c.mu.Lock()
		c.accessToken = ""
		c.mu.Unlock()
		return ErrRefreshExpired
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("refresh session: server returned %s", http.StatusText(resp.StatusCode))
	}

	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("parse refresh response: %w", err)
	}
	var ar AuthResponse
	if err := json.Unmarshal(env.Data, &ar); err != nil {
		return fmt.Errorf("parse refresh token: %w", err)
	}

	c.mu.Lock()
	c.accessToken = ar.Token
	c.mu.Unlock()
	if err := c.cfg.SetRefreshToken(ar.RefreshToken); err != nil {
		return fmt.Errorf("persist refresh token: %w", err)
	}
	return nil
}

// SetAccessToken stores a JWT obtained from login/register for this process.
func (c *Client) SetAccessToken(tok string) {
	c.mu.Lock()
	c.accessToken = tok
	c.mu.Unlock()
}

// roundTrip builds, sends, and parses one request. It returns the envelope,
// the HTTP status, and a transport-level error (connection, marshaling).
func (c *Client) roundTrip(ctx context.Context, method, path string, body any, authed bool) (*Envelope, int, error) {
	var bodyReader io.Reader
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("encode request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", ClientVersion)

	if authed {
		c.applyAuth(req)
	}

	if c.Debug {
		c.debugf("→ %s %s", method, req.URL.String())
		if bodyBytes != nil {
			c.debugf("  body: %s", prettyJSON(redactBody(bodyBytes)))
		}
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if c.Debug {
		body := raw
		if len(body) > 0 {
			body = prettyJSON(redactBody(body))
		}
		c.debugf("← %d %s", resp.StatusCode, body)
	}

	env := &Envelope{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, env)
	}
	if env.RequestID == "" {
		env.RequestID = resp.Header.Get("X-Request-Id")
	}
	return env, resp.StatusCode, nil
}

// applyAuth selects the credential: explicit --api-key, stored/env API key, or
// the in-process JWT.
func (c *Client) applyAuth(req *http.Request) {
	c.mu.Lock()
	at := c.accessToken
	c.mu.Unlock()

	switch {
	case c.overrideAPIKey != "":
		req.Header.Set("X-API-Key", c.overrideAPIKey)
	case c.cfg.EffectiveAPIKey() != "":
		req.Header.Set("X-API-Key", c.cfg.EffectiveAPIKey())
	case at != "":
		req.Header.Set("Authorization", "Bearer "+at)
	default:
		// Only a stored refresh token exists — leave the request unauthenticated
		// so the server 401s it and do() triggers the refresh path.
	}
}

func (c *Client) hasCredentials() bool {
	c.mu.Lock()
	at := c.accessToken
	c.mu.Unlock()
	return c.overrideAPIKey != "" || c.cfg.EffectiveAPIKey() != "" || at != "" || c.cfg.GetRefreshToken() != ""
}

func (c *Client) hasAPIKey() bool {
	return c.overrideAPIKey != "" || c.cfg.EffectiveAPIKey() != ""
}

func (c *Client) storedRefreshToken() string {
	return c.cfg.GetRefreshToken()
}

func (c *Client) debugf(format string, a ...any) {
	if c.Debug && c.DebugOut != nil {
		fmt.Fprintf(c.DebugOut, format+"\n", a...)
	}
}
