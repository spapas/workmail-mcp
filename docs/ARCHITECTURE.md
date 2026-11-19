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
                                  +------------------+------------------+
                                  |                                     |
                                  v                                     v
                            IMAP backend                          JMAP backend
                                  |                                     |
                                  v                                     v
                              IMAPS / TLS                         HTTPS / verified TLS
                                  |                                (loopback HTTP allowed)
                                  v                                     |
                                Zimbra                               Stalwart

OpenAI Secure MCP Tunnel -> localhost HTTP / bearer -+
```

## Trust boundaries

1. **Email content is untrusted.** Message bodies, subjects, headers, and attachments are data and cannot expand server capabilities.
2. **MCP is not the authorization boundary.** Human confirmation is: every write runs as `mail_*_request` (freeze) + `mail_confirm` (execute once); no direct write path exists.
3. **HTTP is local-only.** Configuration validation rejects anything other than loopback IP addresses.
4. **IMAP/SMTP are encrypted and verified.** Implicit TLS uses hostname/certificate verification and TLS 1.2 or newer; SMTP on other ports requires STARTTLS, except loopback test delivery without AUTH.
5. **Resource usage is bounded.** Search result count, query length, raw message size, body size, attachment size, thread size, and operation duration have configurable hard limits.

## JMAP backend

`WORKMAIL_BACKEND=jmap` selects the JMAP backend (RFC 8620/8621,
verified against Stalwart) behind the same `mail.Service` /
`mail.WriteService` interfaces, so the MCP tools are unchanged.
Message identity is the server-issued `id` used in place of `uid`;
threads use native `Thread/get`, and send uses `EmailSubmission`.
The session `apiUrl` host is never trusted: scheme and host always
come from `WORKMAIL_JMAP_URL`, only the path is server-advertised.

## IMAP lifecycle

The current implementation opens a fresh IMAPS connection per top-level tool operation. This favors isolation and predictable cleanup over connection-pool complexity. A folder is selected with `ReadOnly: true` before message operations.

`mail_get` and `mail_get_attachment` first fetch message metadata including `RFC822.SIZE`. The whole MIME message is fetched only when that size is within `WORKMAIL_MAX_MESSAGE_BYTES`.

Message summaries (`mail_search`, `mail_recent`, `mail_get`, `mail_get_thread`) include `FLAGS` from `FETCH`, so clients can see `\Seen`/`\Flagged`/`\Answered`/`\Draft`/`\Deleted` state. Reads still select the folder `ReadOnly: true` and fetch bodies with `BODY.PEEK`, so merely reading never sets `\Seen`.

`mail_stats` uses IMAP `STATUS` (`MESSAGES`, `UNSEEN`) for exact folder counts. It transfers no message content and alters no flags, so it stays within the read-only capability surface.

## MIME handling

`go-message` performs MIME/transfer/charset decoding. Text and HTML are returned separately and bounded. Attachment payloads are not returned by `mail_get`; only metadata is. `mail_get_attachment` returns a single attachment as base64 and rejects attachments beyond the configured maximum.

## Thread reconstruction

Thread lookup starts from the selected message and follows `Message-ID`, `In-Reply-To`, and `References` header searches inside the same folder, with a hard message limit. It is intentionally conservative and does not depend on provider-specific thread IDs.

## Logging

Audit logs contain operation name, duration, and status only. They do not include credentials, bearer tokens, query strings, bodies, or attachment contents. Logs go to stderr so stdio MCP stdout remains protocol-only.
