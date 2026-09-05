# MailAfrica CLI

A user-side CLI for the [MailAfrica](https://mailafrica.online) email
infrastructure API — receiving inbound email, webhooks, sending, wallet billing,
and AI auto-reply configuration, all from your terminal.

**User-side only.** No admin/operator commands ship in this tool.

> Phase 1 of 5 is under construction: scaffold, auth, API keys, and the
> credential-safe config layer. See [ROADMAP.md](ROADMAP.md) for sequencing and
> priorities.

## Install

```bash
go install github.com/MailAfrica/MailAfrica-CLI/cmd/mailafrica@latest
```

## Configure

The CLI reads configuration from the following sources, highest precedence
first:

1. Flags: `--api-url`, `--api-key`
2. Environment: `MAILAFRICA_API_URL`, `MAILAFRICA_API_KEY`
3. Config file: `~/.config/mailafrica/config.json` (created `chmod 0600`)

## Example

```bash
mailafrica auth login --identifier you@example.com
mailafrica apikeys create --name ci --save
mailafrica apikeys list
```

## Security

Credentials are stored only in `~/.config/mailafrica/config.json` with owner-only
permissions. One-time secrets (API keys, SMTP passwords) are shown exactly once.
`--debug` output redacts passwords, tokens, and keys. Never commit a config file
or environment file; `.gitignore` covers them.

## Development

```bash
make build   # -> bin/mailafrica
make vet
make test
```