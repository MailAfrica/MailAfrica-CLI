package api

import (
	"bytes"
	"strings"
	"testing"
)

func TestRedactBody_SensitiveFields(t *testing.T) {
	in := []byte(`{"email":"a@b.c","password":"hunter2","api_key":"MAIL_x","refresh_token":"abc",
                        "user":{"name":"x","password_hash":"x"},"items":[{"secret":"1"}]}`)
	out := redactBody(in)
	s := string(out)
	for _, forbidden := range []string{"hunter2", "MAIL_x", "abc", "\"password_hash\":\"x\"", "\"secret\":\"1\""} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("redacted output leaked %q: %s", forbidden, s)
		}
	}
	for _, allowed := range []string{"a@b.c", `"name":"x"`, "***REDACTED***"} {
		if !strings.Contains(s, allowed) {
			t.Fatalf("redacted output missing expected %q: %s", allowed, s)
		}
	}
}

func TestRedactBody_NonJSONPassthrough(t *testing.T) {
	in := []byte("not json at all")
	if !bytes.Equal(redactBody(in), in) {
		t.Fatal("non-JSON bodies should pass through untouched")
	}
}

func TestRedactBody_Empty(t *testing.T) {
	if got := redactBody(nil); got != nil {
		t.Fatalf("nil body should stay nil, got %q", got)
	}
}
