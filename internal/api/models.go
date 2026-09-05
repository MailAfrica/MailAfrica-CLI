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

// InboundAddress is a receiving address mailbox (inbound_addresses rows).
type InboundAddress struct {
	ID            int64     `json:"id"`
	UserID        int64     `json:"user_id"`
	LocalPart     string    `json:"local_part"`
	Label         *string   `json:"label"`
	DomainID      *int64    `json:"domain_id"`
	RetentionDays int       `json:"retention_days"`
	CreatedAt     time.Time `json:"created_at"`
}

// CreateInboundAddressRequest is the body of POST /api/inbound/addresses.
type CreateInboundAddressRequest struct {
	LocalPart string  `json:"local_part"`
	Label     *string `json:"label,omitempty"`
	DomainID  *int64  `json:"domain_id,omitempty"`
}

// InboundDomain is a custom inbound domain (TXT verify token + DNS check).
type InboundDomain struct {
	ID                int64      `json:"id"`
	UserID            int64      `json:"user_id"`
	Domain            string     `json:"domain"`
	VerificationToken string     `json:"verification_token,omitempty"`
	VerifiedAt        *time.Time `json:"verified_at"`
	LastCheckAt       *time.Time `json:"last_check_at"`
	CreatedAt         time.Time  `json:"created_at"`
}

// CreateInboundDomainRequest is the body of POST /api/inbound/domains.
type CreateInboundDomainRequest struct {
	Domain string `json:"domain"`
}

// VerificationRecord is a DNS record to publish when setting up an inbound
// domain (or sending domain in later phases).
type VerificationRecord struct {
	Type  string `json:"type"`
	Host  string `json:"host"`
	Value string `json:"value"`
}

// CreateInboundDomainResponse is the data of POST /api/inbound/domains: the
// domain plus the exact DNS records the operator must publish.
type CreateInboundDomainResponse struct {
	Domain              InboundDomain        `json:"domain"`
	VerificationRecord  VerificationRecord   `json:"verification_record"`
	VerificationRecords []VerificationRecord `json:"verification_records"`
}

// InboundMessage is a received email at an inbound address.
type InboundMessage struct {
	ID        int64     `json:"id"`
	AddressID int64     `json:"address_id"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Subject   string    `json:"subject"`
	TextBody  string    `json:"text_body"`
	HTMLBody  string    `json:"html_body"`
	IsRead    bool      `json:"is_read"`
	CreatedAt time.Time `json:"created_at"`
}

// Webhook is a receive callback wired to an inbound address. Secret signs the
// delivery notification (X-Signature header) and is shown once at creation.
type Webhook struct {
	ID        int64     `json:"id"`
	AddressID int64     `json:"address_id"`
	URL       string    `json:"url"`
	Secret    string    `json:"secret"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateWebhookRequest is the body of POST /api/webhook/webhooks.
// Secret may be left empty — the server generates one.
type CreateWebhookRequest struct {
	AddressID int64  `json:"address_id"`
	URL       string `json:"url"`
	Secret    string `json:"secret,omitempty"`
}

// WebhookDelivery is one attempt to POST a mail delivery notification.
type WebhookDelivery struct {
	ID          int64      `json:"id"`
	WebhookID   int64      `json:"webhook_id"`
	MessageID   int64      `json:"message_id"`
	StatusCode  int        `json:"status_code"`
	Attempt     int        `json:"attempt"`
	DeliveredAt *time.Time `json:"delivered_at"`
	Status      string     `json:"status"`
	NextRetryAt *time.Time `json:"next_retry_at"`
	LastError   string     `json:"last_error"`
	CreatedAt   time.Time  `json:"created_at"`
}
