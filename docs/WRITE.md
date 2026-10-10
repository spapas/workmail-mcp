# Write operations (two-phase confirmation)

Owner decision (October 2026): the bridge is no longer read-only. All four
write capabilities — send, move, delete, flag mutation — are available through
MCP, but **every write requires a confirmation step, always**. There is no
direct (single-call) write path.

## Flow

```text
1. client calls mail_*_request  -> server validates + freezes exact payload
                                -> returns {pending_id, expires_at, summary}
2. user reviews summary
3. client calls mail_confirm {pending_id} -> server executes frozen payload once
```

Rules:

- `pending_id` values are random 128-bit IDs (base64url), single-use, and
  expire after `WORKMAIL_CONFIRM_TTL` (default 5m, range 1m–30m).
- Confirm executes **exactly the frozen payload**: clients cannot change
  recipients, folders, UIDs, or flags at confirm time.
- Unknown, reused, or expired IDs fail; the client must re-request.
- Bodies are never echoed in request summaries or logs.
- BCC recipients travel in the SMTP envelope only, never in message headers.

## Tools

| Tool | Effect |
| --- | --- |
| `mail_send_request` | Stage an email send (To/Cc/Bcc, subject, text/HTML body) |
| `mail_move_request` | Stage moving one UID to an existing folder (MOVE, or COPY + expunge fallback) |
| `mail_delete_request` | Stage permanent delete of one UID (STORE `\Deleted` + expunge, UIDPLUS when offered) |
| `mail_set_flags_request` | Stage flag update; allowlist: `\Seen`, `\Flagged`, `\Answered`, `\Draft` (`\Deleted` rejected — use the delete flow) |
| `mail_confirm` | Execute one staged operation by `pending_id` |

## Transports

- Move/delete/flags use IMAPS with a `ReadOnly: false` SELECT, only inside the
  confirm step. Read paths still select `ReadOnly: true`.
- Send uses SMTP with verified TLS: implicit TLS on port 465, otherwise
  STARTTLS when offered. TLS 1.2+, hostname verification, no bypass.
- SMTP identity defaults to the IMAP identity (`WORKMAIL_SMTP_HOST` defaults
  to `WORKMAIL_IMAP_HOST`, credentials and From default likewise) so a typical
  Zimbra setup needs no extra config; explicit `WORKMAIL_SMTP_*` overrides win.

## Limits

- Recipients: `WORKMAIL_MAX_RECIPIENTS` (default 50, max 100).
- Bodies: bounded by `WORKMAIL_MAX_BODY_BYTES`; subject capped at 998 chars,
  control characters rejected (header-injection safe).
- Flags: allowlisted, Add/Remove conflicts rejected.
- Folders/destination: trimmed, max 1024 chars, control characters rejected.
- Every operation runs under `WORKMAIL_OPERATION_TIMEOUT`.

## Audit

Request and confirm steps log operation name, duration, and status only — no
recipients, subjects, bodies, folders, passwords, or tokens.
