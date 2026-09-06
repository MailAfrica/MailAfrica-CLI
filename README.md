# MailAfrica CLI

A user-side command-line client for the [MailAfrica](https://mailafrica.online)
email infrastructure API: receive inbound mail, wire webhooks, send transactional
email from verified domains, watch your TZS wallet, and turn an address into an
AI auto-responder — all from your terminal.

**User-side only.** No admin/operator commands ship in this tool.

## Install

```bash
go install github.com/MailAfrica/MailAfrica-CLI/cmd/mailafrica@latest
```

or build locally:

```bash
make build   # -> bin/mailafrica
```

## Configure

Configuration is read with this precedence (highest first):

1. Flags: `--api-url`, `--api-key`
2. Environment: `MAILAFRICA_API_URL`, `MAILAFRICA_API_KEY`
3. Config file: `~/.config/mailafrica/config.json` (created `chmod 0600`)

Log in to get a session API key:

```bash
mailafrica auth login --identifier you@example.com
```

or set `MAILAFRICA_API_KEY` for scripted use.

## Quick start

```bash
# account
mailafrica apikeys list
mailafrica wallet balance

# receive: inbound address -> webhook
mailafrica inbound address create --local-part support
mailafrica webhook create --address-id 1 --url https://you.example/hooks/mail
mailafrica inbound message list --address-id 1

# send from your own signed domain
mailafrica domain add --domain mail.example.com          # publish DKIM/SPF/DMARC
mailafrica domain verify 1
mailafrica send email --to you@corp.com --subject "Hi" --text-body "hello"
mailafrica send batch --to-file recipients.txt --subject "Bulk"
mailafrica send template create --name Welcome --subject "Hi {{name}}" --html-file welcome.html

# sandbox: test SMTP flows without paying
mailafrica sandbox smtp
mailafrica sandbox message list

# AI auto-responder on an address
mailafrica agent config 1 --mode auto --persona "You are our sales rep…"
mailafrica agent draft 1 --subject "Pricing?" --text-body "How much?"
```

## Security

- Keys and tokens live only in `~/.config/mailafrica/config.json` with owner-only
  permissions; env vars override per-process.
- One-time secrets (API keys, SMTP passwords, SMS provider keys) are shown
  **exactly once** and masked everywhere else.
- `--debug` redacts passwords, tokens, and keys from request/response dumps.
- Payment happens via the provider's hosted checkout / USSD push — the CLI never
  sees card or money details.
- Never commit a config or env file; `.gitignore` covers them. A CI scan rejects
  real credential shapes across the entire git history.

## Commands

```
mailafrica
├── agent        Turn an inbound address into an AI auto-responder
│   ├── config   Read or update auto-reply config (off|draft|auto)
│   ├── list     Configured addresses and their modes
│   └── draft    Preview a reply the agent would send (never sends)
├── apikeys      Manage developer API keys (MAIL_…)
├── auth         Register, log in, manage the session
├── compliance   PDPC profile, retention, audit export
├── config       Inspect configuration
├── domain       Verified sending domains: DKIM/SPF/DMARC records + identities
├── inbound      Receiving addresses, domains, mail
├── sandbox      Test flows with a sandbox SMTP server
├── send         Send email, batch (server-side ≤50/call chunking), templates
├── sms          Short SMS when mail hits an inbound address
├── version      Print the version
├── wallet         Balance and top-ups (min 2000 TZS)
└── webhook      Delivery callbacks to your receiving addresses
```

## Development

```bash
make build   # -> bin/mailafrica
make fmt
make vet
make test
```

Tests spin up ephemeral fake servers (with `-race` clean) covering the e2e
command paths incl. the batch summary contract and one-time-secret handling.