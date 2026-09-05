// Package api is a typed HTTP client for the MailAfrica API. Every request runs
// through a single path (Client.Do / Client.DoPublic) that owns the auth header,
// response envelope parsing, and the 401→refresh→retry interceptor.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Envelope is the standard MailAfrica response wrapper returned by every
// endpoint:
//
//	{"success":true,"message":"...","data":{...},"errors":[...],
//	 "pagination":{...},"request_id":"...","timestamp":"..."}
type Envelope struct {
	Success    bool            `json:"success"`
	Message    string          `json:"message"`
	Data       json.RawMessage `json:"data"`
	Errors     []ErrorEntry    `json:"errors"`
	Pagination *Pagination     `json:"pagination"`
	RequestID  string          `json:"request_id"`
	Timestamp  string          `json:"timestamp"`
}

// ErrorEntry is one item of the errors array on failed envelopes.
type ErrorEntry struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Pagination describes a paged result set.
type Pagination struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// sentinel errors surfaced to command layers.
var (
	// ErrNotAuthenticated means no usable credential exists to make the call.
	ErrNotAuthenticated = errors.New("not authenticated: run `mailafrica auth login` or create/set an API key")
	// ErrRefreshExpired means the stored refresh token was rejected (expired or
	// revoked); stored credentials are cleared so the user must log in again.
	ErrRefreshExpired = errors.New("session expired: run `mailafrica auth login`")
)

// APIError is a structured failure returned by the API for non-2xx responses.
type APIError struct {
	Status    int
	Code      string
	Message   string
	RequestID string
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("request failed: %s", e.Message)
	if e.Code != "" {
		msg += fmt.Sprintf(" (%s)", e.Code)
	}
	if e.Status != 0 {
		msg += fmt.Sprintf(" [http %d]", e.Status)
	}
	return msg
}

// Is reports whether err is an *APIError. Useful for callers that want to
// branch on the error code without unpacking the type themselves.
func (e *APIError) Is(target error) bool {
	_, ok := target.(*APIError)
	return ok
}

// apiErrorFrom builds a typed error from an envelope + status code.
func apiErrorFrom(env *Envelope, status int) *APIError {
	ae := &APIError{Status: status, RequestID: env.RequestID}
	if len(env.Errors) > 0 {
		ae.Code = env.Errors[0].Code
		ae.Message = env.Errors[0].Message
	}
	if ae.Message == "" {
		ae.Message = env.Message
	}
	if ae.Message == "" {
		ae.Message = "unexpected API error"
	}
	return ae
}
