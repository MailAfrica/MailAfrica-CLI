package api

import (
	"bytes"
	"encoding/json"
	"strings"
)

// sensitiveKeySubstrings match JSON object keys whose values must never be
// emitted to debug output. Matches are substring-based (case-insensitive) so
// variants like "password_hash" or "client_secret" are covered.
var sensitiveKeySubstrings = []string{
	"password",
	"secret",
	"token",
	"api_key",
	"apikey",
	"credential",
	"id_token",
	"code_verifier",
	"code_challenge",
}

// redactBody returns a printable, redacted copy of a JSON request/response body
// for use in --debug output. Non-JSON bodies are returned verbatim.
func redactBody(b []byte) []byte {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return b
	}
	redactValue(v)
	out, err := json.Marshal(v)
	if err != nil {
		return b
	}
	return out
}

// redactValue walks a decoded JSON tree replacing sensitive values with a
// placeholder in place.
func redactValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if isSensitiveKey(k) {
				t[k] = "***REDACTED***"
				continue
			}
			t[k] = redactValue(val)
		}
		return t
	case []any:
		for i := range t {
			t[i] = redactValue(t[i])
		}
		return t
	default:
		return v
	}
}

func isSensitiveKey(k string) bool {
	lk := strings.ToLower(k)
	for _, s := range sensitiveKeySubstrings {
		if strings.Contains(lk, s) {
			return true
		}
	}
	return false
}

// prettyJSON indents a JSON blob for readable debug output.
func prettyJSON(b []byte) []byte {
	var buf bytes.Buffer
	if err := json.Indent(&buf, b, "", "  "); err != nil {
		return b
	}
	return buf.Bytes()
}
