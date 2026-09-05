# Architecture

## Data flow

```text
Hermes -------------------- stdio -------------------+
                                                     |
                                                     v
                                                MCP tools
                                                     |
                                                     v
                                              mail.Service
                                                     |
                                                     v
                                                IMAP backend
                                                     |
                                                     v
                                               IMAPS / TLS
                                                     |
                                                     v
                                                   Zimbra

OpenAI Secure MCP Tunnel -> localhost HTTP / bearer -+
```

## Trust boundaries

1. **Email content is untrusted.** Message bodies, subjects, headers, and attachments are data and cannot expand server capabilities.
2. **MCP is not the authorization boundary.** The concrete service exposes only read operations; no write primitive exists underneath the tools.
3. **HTTP is local-only.** Configuration validation rejects anything other than loopback IP addresses.
4. **IMAP is encrypted and verified.** Implicit TLS uses hostname/certificate verification and TLS 1.2 or newer.
5. **Resource usage is bounded.** Search result count, query length, raw message size, body size, attachment size, thread size, and operation duration have configurable hard limits.

## IMAP lifecycle

The current implementation opens a fresh IMAPS connection per top-level tool operation. This favors isolation and predictable cleanup over connection-pool complexity. A folder is selected with `ReadOnly: true` before message operations.

`mail_get` and `mail_get_attachment` first fetch message metadata including `RFC822.SIZE`. The whole MIME message is fetched only when that size is within `WORKMAIL_MAX_MESSAGE_BYTES`.

## MIME handling

`go-message` performs MIME/transfer/charset decoding. Text and HTML are returned separately and bounded. Attachment payloads are not returned by `mail_get`; only metadata is. `mail_get_attachment` returns a single attachment as base64 and rejects attachments beyond the configured maximum.

## Thread reconstruction

Thread lookup starts from the selected message and follows `Message-ID`, `In-Reply-To`, and `References` header searches inside the same folder, with a hard message limit. It is intentionally conservative and does not depend on provider-specific thread IDs.

## Logging

Audit logs contain operation name, duration, and status only. They do not include credentials, bearer tokens, query strings, bodies, or attachment contents. Logs go to stderr so stdio MCP stdout remains protocol-only.
