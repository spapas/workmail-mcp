# Roadmap

## Phase 0 — Repository foundation
- [x] Go module and application entry point
- [x] Repository guidance in `AGENTS.md`
- [x] Secret-safe `.gitignore` and example environment file
- [x] CI for formatting, vetting, and tests
- [x] Automatic Windows/Linux binary artifacts

## Phase 1 — Configuration and domain contracts
- [ ] Typed configuration loading and validation
- [ ] Mail domain models
- [ ] Read-only mail service interface
- [ ] Explicit limits and timeout policy

## Phase 2 — Zimbra/IMAP read-only backend
- [ ] TLS IMAP connection with certificate verification
- [ ] Folder listing
- [ ] Message search
- [ ] Message retrieval and MIME parsing
- [ ] Attachment retrieval with size limits
- [ ] Thread reconstruction
- [ ] Unit/integration tests using non-production fixtures

## Phase 3 — MCP over stdio for Hermes
- [ ] MCP server lifecycle
- [ ] Bounded read-only mail tools
- [ ] Hermes configuration documentation
- [ ] End-to-end local test

## Phase 4 — localhost MCP HTTP mode
- [ ] Bind to loopback only
- [ ] Bearer-token authentication
- [ ] Request limits and timeouts
- [ ] Audit logging without sensitive content
- [ ] ChatGPT connection/tunnel documentation

## Phase 5 — Hardening and releases
- [ ] Security review of tool boundaries
- [ ] Dependency review and pinning policy
- [ ] Version command and build metadata
- [ ] Tagged GitHub Releases with downloadable binaries
- [ ] Windows installation/update documentation

## Explicitly out of scope for the initial release
- Sending email
- Deleting or moving messages
- Changing message flags
- Creating folders
- Raw/arbitrary IMAP commands
- Full-mailbox synchronization
- Database/vector-store indexing
