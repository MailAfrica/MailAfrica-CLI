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

// Attachment is a file embedded (base64) in an outbound email. Same shape as
// the inbound parser's attachment.
type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
	DataBase64  string `json:"data_base64"`
}

// OutboundMessage is a single outbound send attempt, successful or not. Status
// is "sent" (accepted by the local MTA) or "failed".
type OutboundMessage struct {
	ID                int64     `json:"id"`
	UserID            int64     `json:"user_id"`
	FromAddress       string    `json:"from_address"`
	ToAddresses       []string  `json:"to_addresses"`
	CcAddresses       []string  `json:"cc_addresses,omitempty"`
	BccAddresses      []string  `json:"bcc_addresses,omitempty"`
	Subject           string    `json:"subject"`
	HTMLBody          *string   `json:"html_body,omitempty"`
	TextBody          *string   `json:"text_body,omitempty"`
	Status            string    `json:"status"`
	ErrorCode         *string   `json:"error_code,omitempty"`
	ProviderMessageID *string   `json:"provider_message_id,omitempty"`
	AmountTZS         int64     `json:"amount_tzs"`
	CreatedAt         time.Time `json:"created_at"`
}

// OutboundSendRequest is the POST /api/outbound/emails body.
type OutboundSendRequest struct {
	To           []string          `json:"to"`
	Cc           []string          `json:"cc,omitempty"`
	Bcc          []string          `json:"bcc,omitempty"`
	Subject      string            `json:"subject"`
	HTMLBody     string            `json:"html_body,omitempty"`
	TextBody     string            `json:"text_body,omitempty"`
	Attachments  []Attachment      `json:"attachments,omitempty"`
	TemplateID   *int64            `json:"template_id,omitempty"`
	Variables    map[string]string `json:"variables,omitempty"`
	FromDomainID *int64            `json:"from_domain_id,omitempty"`
	FromAddress  *string           `json:"from_address,omitempty"`
}

// RecipientStatus is the per-recipient delivery state of an outbound message.
type RecipientStatus struct {
	ID           int64     `json:"id"`
	MessageID    int64     `json:"message_id"`
	Recipient    string    `json:"recipient"`
	Status       string    `json:"status"`
	ProviderCode *string   `json:"provider_code,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// OutboundMessageDetail is an OutboundMessage plus its per-recipient statuses.
type OutboundMessageDetail struct {
	Message    OutboundMessage   `json:"message"`
	Recipients []RecipientStatus `json:"recipients"`
}

// Template is a reusable subject/HTML/text body with {{variable}} placeholders.
type Template struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Name      string    `json:"name"`
	Subject   string    `json:"subject"`
	HTMLBody  string    `json:"html_body,omitempty"`
	TextBody  string    `json:"text_body,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TemplateRequest is the POST/PATCH /api/outbound/templates body (full replace
// on update).
type TemplateRequest struct {
	Name     string `json:"name"`
	Subject  string `json:"subject"`
	HTMLBody string `json:"html_body,omitempty"`
	TextBody string `json:"text_body,omitempty"`
}

// SendingDomain is a verified sending domain row from /api/domains. It must
// publish DKIM/SPF/DMARC and be status "verified" before mail can be sent from
// it; the self-hosted MTA signs with the stored per-domain DKIM key.
type SendingDomain struct {
	ID                int64      `json:"id"`
	UserID            int64      `json:"user_id"`
	Domain            string     `json:"domain"`
	VerificationToken string     `json:"verification_token,omitempty"`
	Purpose           string     `json:"purpose"`
	Status            string     `json:"status"`
	BouncePrefix      string     `json:"bounce_prefix"`
	FromLocalPart     string     `json:"from_local_part"`
	DkimHost          *string    `json:"dkim_host,omitempty"`
	DkimValue         *string    `json:"dkim_value,omitempty"`
	VerifiedAt        *time.Time `json:"verified_at,omitempty"`
	LastCheckAt       *time.Time `json:"last_check_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

// AddSendingDomainResponse is the data of POST /api/domains: the domain plus
// the complete DNS record set (DKIM/SPF/DMARC) to publish. dns_records is a
// positional array at the wire boundary — ParseDNSRecords maps it by content.
type AddSendingDomainResponse struct {
	Domain     SendingDomain `json:"domain"`
	DNSRecords []DNSRecord   `json:"dns_records"`
}

// AddSendingDomainRequest is the body of POST /api/domains.
type AddSendingDomainRequest struct {
	Domain        string `json:"domain"`
	FromLocalPart string `json:"from_local_part,omitempty"`
}

// SenderAddress is a From identity the user may send from on one of their
// verified sending domains.
type SenderAddress struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	DomainID  int64     `json:"domain_id"`
	Domain    string    `json:"domain"`
	LocalPart string    `json:"local_part"`
	CreatedAt time.Time `json:"created_at"`
}

// SenderAddressRequest is the body of POST /api/domains/{id}/senders.
type SenderAddressRequest struct {
	LocalPart string `json:"local_part"`
}

// SandboxCredential is an API credential for the sandbox. ClientSecret is
// returned in full only at creation.
type SandboxCredential struct {
	ID           int64     `json:"id"`
	UserID       int64     `json:"user_id"`
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret"`
	Scopes       *string   `json:"scopes,omitempty"`
	Revoked      bool      `json:"revoked"`
	CreatedAt    time.Time `json:"created_at"`
}

// SandboxSMTPCredentials are the SMTP AUTH settings for the sandbox server.
// Password is present only right after (re)generation.
type SandboxSMTPCredentials struct {
	Host        string     `json:"host"`
	Port        int        `json:"port"`
	Username    string     `json:"username"`
	Password    *string    `json:"password,omitempty"`
	PasswordSet bool       `json:"password_set"`
	GeneratedAt *time.Time `json:"credentials_generated_at,omitempty"`
}

// SandboxMessage is a message captured by the sandbox catch-all server after
// SMTP AUTH delivery with sandbox credentials.
type SandboxMessage struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"user_id"`
	From        string    `json:"from_addr"`
	To          string    `json:"to_addr"`
	Subject     string    `json:"subject"`
	TextBody    *string   `json:"text_body,omitempty"`
	HTMLBody    *string   `json:"html_body,omitempty"`
	Headers     any       `json:"headers"`
	Attachments any       `json:"attachments"`
	ReceivedAt  time.Time `json:"received_at"`
}

// BillingTopup is a wallet top-up request row created when a checkout session
// or USSD push is initiated. Status is pending|completed|failed.
type BillingTopup struct {
	ID                int64     `json:"id"`
	UserID            int64     `json:"user_id"`
	AmountTZS         int64     `json:"amount_tzs"`
	Status            string    `json:"status"`
	ProviderReference *string   `json:"provider_reference,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

// BillingTopupResponse carries a hosted-checkout top-up result. The payment
// links are the only payment surface the API exposes to users.
type BillingTopupResponse struct {
	Topup             *BillingTopup `json:"topup"`
	CheckoutURL       string        `json:"checkout_url,omitempty"`
	PaymentLinkURL    string        `json:"payment_link_url,omitempty"`
	ProviderReference string        `json:"provider_reference,omitempty"`
}

// ComplianceProfile captures a user's pseudonymisation/PDPC posture defaults.
type ComplianceProfile struct {
	ID                    int64      `json:"id"`
	UserID                int64      `json:"user_id"`
	PDPCRegistered        bool       `json:"pdpc_registered"`
	PDPCCertificateNumber *string    `json:"pdpc_certificate_number,omitempty"`
	PDPCRegisteredAt      *time.Time `json:"pdpc_registered_at,omitempty"`
	DefaultRetentionDays  int        `json:"default_retention_days"`
	DataConsentAt         *time.Time `json:"data_consent_at,omitempty"`
	PrivacyPolicyVersion  string     `json:"privacy_policy_version,omitempty"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// ComplianceProfileUpdate is the partial PATCH body. nil fields are untouched.
type ComplianceProfileUpdate struct {
	PDPCRegistered        *bool   `json:"pdpc_registered,omitempty"`
	PDPCCertificateNumber *string `json:"pdpc_certificate_number,omitempty"`
	PDPCRegisteredAt      *string `json:"pdpc_registered_at,omitempty"`
	DefaultRetentionDays  *int    `json:"default_retention_days,omitempty"`
}

// ComplianceAuditExport is a point-in-time data-handling snapshot for audits.
type ComplianceAuditExport struct {
	PDPCRegistered        bool      `json:"pdpc_registered"`
	PDPCCertificateNumber string    `json:"pdpc_certificate_number,omitempty"`
	DefaultRetentionDays  int       `json:"default_retention_days"`
	AddressCount          int64     `json:"address_count"`
	MessageCount          int64     `json:"message_count"`
	GeneratedAt           time.Time `json:"generated_at"`
}

// SMSNotification is a phone number that gets a short SMS when mail arrives at
// an inbound address. The SendAfrica API key is stored server-side encrypted
// and is only returned in full at creation.
type SMSNotification struct {
	ID          int64     `json:"id"`
	AddressID   int64     `json:"address_id"`
	PhoneNumber string    `json:"phone_number"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
}

// SMSCreateRequest body for creating an SMS notification.
type SMSCreateRequest struct {
	AddressID   int64  `json:"address_id"`
	PhoneNumber string `json:"phone_number"`
	APIKey      string `json:"api_key"`
}

// SMSCreateResponse extends the notification with the plaintext provider key,
// which the API returns exactly once.
type SMSCreateResponse struct {
	SMSNotification
	APIKey string `json:"api_key"`
}

// SMSDelivery is one captured delivery attempt for an SMS notification.
type SMSDelivery struct {
	ID                int64     `json:"id"`
	NotificationID    int64     `json:"sms_notification_id"`
	MessageID         int64     `json:"message_id"`
	Status            string    `json:"status"`
	ErrorCode         *string   `json:"error_code,omitempty"`
	ProviderMessageID *string   `json:"provider_message_id,omitempty"`
	Attempt           int       `json:"attempt"`
	CreatedAt         time.Time `json:"created_at"`
}

// AgentConfig is the auto-reply configuration for one inbound address. Mode is
// off|draft|auto. The actual replies are produced and sent by the deployed
// agent service, which reads this same config.
type AgentConfig struct {
	AddressID         int64     `json:"address_id"`
	UserID            int64     `json:"user_id"`
	Mode              string    `json:"mode"`
	Persona           *string   `json:"persona,omitempty"`
	Enabled           bool      `json:"enabled"`
	ReplyFromDomainID *int64    `json:"reply_from_domain_id,omitempty"`
	ReplyFromAddress  *string   `json:"reply_from_address,omitempty"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// AgentConfigUpdate is the upsert body. Only mode is required; everything else
// is optional and only set when non-nil.
type AgentConfigUpdate struct {
	Mode              string  `json:"mode"`
	Persona           *string `json:"persona,omitempty"`
	Enabled           *bool   `json:"enabled,omitempty"`
	ReplyFromDomainID *int64  `json:"reply_from_domain_id,omitempty"`
	ReplyFromAddress  *string `json:"reply_from_address,omitempty"`
}

// AgentDraftRequest is the input for a one-off draft preview. Never sends.
type AgentDraftRequest struct {
	Subject  string `json:"subject,omitempty"`
	TextBody string `json:"text_body,omitempty"`
}

// AgentDraftResponse is the generated preview text.
type AgentDraftResponse struct {
	Draft string `json:"draft"`
}
