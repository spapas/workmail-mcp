# Security policy

`workmail-mcp` is intended to sit between AI clients and a real mailbox, so security boundaries are part of the product design.

## Supported security model

The initial implementation is read-only and follows these principles:

- no SMTP sending
- no message deletion or movement
- no flag mutation or expunge
- no raw/arbitrary IMAP command execution
- TLS with certificate verification for IMAP
- loopback-only HTTP binding when HTTP transport is enabled
- bearer-token authentication for HTTP mode
- bounded inputs and explicit timeouts
- mailbox content treated as untrusted data
- no secrets or mailbox content in logs

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
