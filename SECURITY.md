# Security policy

`workmail-mcp` is intended to sit between AI clients and a real mailbox, so security boundaries are part of the product design.

## Supported security model

The bridge exposes read tools plus four write capabilities (send, move,
delete, flag mutation), all gated by mandatory two-phase confirmation:

- no direct write path: every write is `mail_*_request` (freeze) + `mail_confirm` (execute once)
- single-use, expiring confirmation IDs; frozen payloads cannot be altered at confirm time
- no raw/arbitrary IMAP command execution
- TLS with certificate verification for IMAP and SMTP (implicit TLS or STARTTLS, TLS 1.2+)
- loopback-only HTTP binding when HTTP transport is enabled
- bearer-token authentication for HTTP mode
- bounded inputs and explicit timeouts
- mailbox content treated as untrusted data: email bodies are never
  interpreted as instructions, and a malicious message cannot bypass the
  human confirmation step on its own
- no secrets, recipients, bodies, or mailbox content in logs

Write-specific risks (prompt-injection-triggered send/delete, irreversible
expunge) are mitigated by the confirmation gate, the flag allowlist
(`\Deleted` is only reachable via the delete flow), BCC envelope-only
handling, and per-operation timeouts. Review each `mail_*_request` summary
before confirming.

## Reporting a vulnerability

Do not include real mailbox credentials, API tokens, private message content, or sensitive logs in a public GitHub issue.

If a report can be demonstrated safely with synthetic data, a GitHub issue is acceptable. For vulnerabilities requiring disclosure of sensitive operational information, use a private contact channel with the repository owner.

## Secrets

Never commit:

- IMAP passwords or app passwords
- bearer tokens
- `.env` files containing live values
- private keys or certificates
- real mailbox exports
- logs containing message bodies or attachments

If a secret is committed accidentally, assume it is compromised and rotate it; deleting it from the latest commit is not sufficient.
