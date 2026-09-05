# Roadmap

## Implemented MVP

- [x] Go module and repository conventions
- [x] secret-safe configuration with `*_FILE` support
- [x] read-only domain interface
- [x] TLS IMAP backend with read-only folder selection
- [x] folder listing and structured search
- [x] bounded message/MIME retrieval
- [x] bounded attachment retrieval
- [x] bounded provider-independent thread reconstruction
- [x] MCP stdio transport for Hermes
- [x] streamable HTTP MCP transport on loopback
- [x] bearer-token HTTP authentication
- [x] non-sensitive audit logging
- [x] `doctor`, `doctor --latest-subject`, `token`, `version`, and help commands
- [x] unit tests, formatting checks, and `go vet` in CI
- [x] deterministic GreenMail IMAPS integration tests with trusted ephemeral TLS
- [x] end-to-end MCP stdio regression coverage for all six read-only tools
- [x] automatic Windows/Linux binary artifacts
- [x] tag-driven GitHub Release workflow with checksums
- [x] client/security/architecture documentation
- [x] basic smoke test against the owner's Zimbra server: TLS, authentication, folder listing, MCP initialize/tools list, and `mail_list_folders`

## Requires the owner's real environment

- [ ] full Zimbra validation for `mail_recent`, `mail_search`, `mail_get`, `mail_get_attachment`, and `mail_get_thread`
- [ ] Hermes end-to-end tool invocation on the owner's Windows machine
- [ ] ChatGPT end-to-end test when the account/workspace supports custom MCP and Secure MCP Tunnel

## Possible later work (not required for the MVP)

- connection reuse/pooling if measured latency justifies it
- optional message-header-only fetch modes
- provider-specific optimizations behind the same `mail.Service` interface
- signed release binaries

## Explicitly out of scope

- sending mail
- deleting/moving messages
- changing flags
- creating folders
- raw/arbitrary IMAP commands
- full mailbox synchronization
- database or vector-store indexing
