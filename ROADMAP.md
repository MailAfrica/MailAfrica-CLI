# MailAfrica CLI — Roadmap & Priorities

Tracked separately from the phase plan so build sequencing never silently
becomes priority ranking. **Sequencing is about dependencies, not value.**

## Strategic priority (do not deprioritize)

- **Agent commands (`agent config`, `agent draft`) are the product
  differentiator** — the "Twilio-for-AI-agent-email" story is built on turning an
  inbound address into an AI auto-responder. The CLI must ship these.
- They are sequenced last (Phase 4) only because agent `reply_from` config needs
  verified sending domains (Phase 3) working first.
- If the project ever slows down mid-stream (across the other ~30 repos), the
  Phase 4 agent piece must still land. It is **mandatory**, not optional polish.

## Phase map

| Phase | Scope | Status |
|-------|-------|--------|
| 1 | Foundation, config, API client + auth interceptor, auth + apikeys commands | shipped |
| 2 | Inbound (addresses/domains/messages) + webhooks | shipped |
| 3 | Sending domains + sender IDs, outbound send/batch, templates, sandbox | shipped |
| 4 | Wallet billing, compliance, SMS — **and Agent (strategic)** | shipped |
| 5 | Publication polish, CI, README, git-history secrets audit | shipped |

## Standing design invariants (locked in review)

1. **Single request path.** All auth flows through `internal/api` `Do()`; no
   command layer handles 401s. One in-flight request per process is assumed; the
   refresh path is still mutex-serialized so future concurrency can't rotate the
   same refresh token twice.
2. **`dns_records` parsed at the boundary** into a named `DNSRecords{DKIM, SPF,
   DMARC}` struct — never indexed positionally. Unrecognized records are
   surfaced (warn + expose), never silently dropped.
3. **Batch sends delegated to the server batch endpoint.** `send batch` posts
   to `/api/outbound/emails/batch`; the server splits into ≤50-recipient chunks,
   filters suppressed addresses, and continues on failure — reporting a
   `sent N / failed M` summary via `BatchResult`. The single-send cap (50) is
   still validated client-side with a pointer to `send batch`.
- **Sending-domain authoritative records surfaced.** `domain list`/`verify`
  now render the API-returned `spf_*`/`dmarc_*` fields alongside DKIM, so the
  CLI never guesses what DNS to publish.
- **Inbound message fields aligned to the wire.** `InboundMessage` now maps
  `from_addr`/`to_addr`/`received_at` (and pointer bodies) exactly as the API
  returns them.