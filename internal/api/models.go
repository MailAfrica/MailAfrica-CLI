package api

import "time"

// User mirrors the serialized user object from /api/auth/me and login/register
// responses.
type User struct {
	ID              int64      `json:"id"`
	Email           *string    `json:"email"`
	PhoneNumber     *string    `json:"phone_number"`
	Name            string     `json:"name"`
	CompanyName     *string    `json:"company_name"`
	IsAdmin         bool       `json:"is_admin"`
	DisabledAt      *time.Time `json:"disabled_at"`
	EmailVerifiedAt *time.Time `json:"email_verified_at"`
	PhoneVerifiedAt *time.Time `json:"phone_verified_at"`
	CreatedAt       time.Time  `json:"created_at"`
}

// AuthResponse is the data of /api/auth/register, /login, /google and /refresh.
type AuthResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
	User         User   `json:"user"`
}

// APIKey mirrors a developer API key row (created + listed).
type APIKey struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"user_id"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"key_prefix"`
	Scopes     string     `json:"scopes"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// CreateAPIKeyResponse is the data of POST /api/apikeys. Key is the plaintext
// MAIL_… secret and is returned exactly once by the server.
type CreateAPIKeyResponse struct {
	APIKey APIKey `json:"api_key"`
	Key    string `json:"key"`
}

// DNSRecord is one raw entry in the sending-domain dns_records array.
type DNSRecord struct {
	Type  string `json:"type"`
	Host  string `json:"host"`
	Value string `json:"value"`
}

// UnknownRecord is a dns_records entry that did not map to DKIM/SPF/DMARC. It
// is surfaced, never silently dropped, so a missing or novel record can't be
// overlooked.
type UnknownRecord struct {
	Type  string
	Host  string
	Value string
}

// DNSRecords is the named, position-independent view of a sending domain's
// dns_records array. Constructed by ParseDNSRecords; call sites never index the
// raw array positionally.
type DNSRecords struct {
	DKIM  DNSRecord
	SPF   DNSRecord
	DMARC DNSRecord
	// Unrecognized carries any record that did not map to the three known types.
	Unrecognized []UnknownRecord
}
