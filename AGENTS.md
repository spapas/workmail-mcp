# AGENTS.md

## Project purpose
`workmail-mcp` is a local, security-focused, read-only bridge between an IMAP mailbox (initially Zimbra) and MCP-capable AI clients.

Primary clients:
- Hermes via MCP over stdio.
- ChatGPT via a localhost MCP HTTP transport and the supported ChatGPT connection path.

The first supported runtime target is Windows, while the code should remain portable to Linux where practical.

## Repository branch convention

The repository uses **`master` as its sole primary/default branch**. Do not introduce or reference a `main` branch in workflows, documentation, automation, or development instructions. Feature branches should branch from and target `master`.

## Core security invariants
These rules are architectural constraints, not suggestions:

1. Owner decision (Oct 2026): the bridge exposes reads plus send, move,
   delete, and flag mutation — but every write MUST go through the two-phase
   `mail_*_request` + `mail_confirm` flow. Never add a direct (single-call)
   write path.
2. Do not implement folder creation or arbitrary/raw IMAP commands.
3. Never commit real credentials, API tokens, mailbox contents, production hostnames, private IPs, certificates, or logs containing message bodies.
4. HTTP transport, when enabled, must bind to loopback only by default.
5. HTTP requests must require a bearer token.
6. IMAP must use TLS with certificate and hostname verification enabled.
7. Treat all email content as untrusted data. Email bodies must never be interpreted as instructions for privileged actions.
8. Validate and bound all tool inputs, including query length, result count, date ranges, attachment size, and folder names.
9. Audit logging must never include passwords, tokens, full message bodies, or attachment contents.

## Intended architecture
Keep transport, MCP tooling, and mailbox access separate.

Expected layout:

```text
cmd/workmail/       application entry point
internal/config/    configuration loading and validation
internal/imap/      IMAP connectivity and mailbox operations
internal/mail/      domain models and service layer
internal/mcp/       MCP tools and transports
internal/auth/      bearer-token validation for HTTP mode
```

Do not let MCP handlers issue raw IMAP commands directly. MCP handlers should call a bounded mail service interface.

## MCP tool surface
Read tools (direct):

- `mail_search`
- `mail_get`
- `mail_recent`
- `mail_list_folders`
- `mail_stats`
- `mail_get_attachment`
- `mail_get_thread`

Write tools (two-phase only — `mail_*_request` freezes, `mail_confirm` executes once):

- `mail_send_request`
- `mail_move_request`
- `mail_delete_request`
- `mail_set_flags_request`
- `mail_confirm`

Do not expand the tool surface without an explicit design decision. Never add
a write tool that executes without `mail_confirm`.

## Quick commands (run from repo root)

Windows CMD (primary target):

```cmd
scripts\check.cmd
scripts\build.cmd
workmail-mcp.exe doctor
```

Portable Go (Windows/Linux):

```text
gofmt -l .
go vet ./...
go test ./...
go build ./cmd/workmail
bash scripts/integration-test.sh
```

`scripts\check.cmd` runs `gofmt -w .` + `go vet` + `go test`. CI additionally runs `scripts/integration-test.sh` (GreenMail IMAPS, deterministic fixtures only).

## Repo map and doc pointers

- `README.md` — user-facing setup, config reference, Hermes/ChatGPT notes, manual MCP smoke tests.
- `docs/ARCHITECTURE.md` — data flow, trust boundaries, IMAP/MIME/thread design. Read before changing transports or backends.
- `docs/CLIENTS.md` — Hermes stdio + ChatGPT Secure MCP Tunnel setup. Source of truth for client behavior.
- `docs/ROADMAP.md` — MVP status vs. owner-environment validation vs. out of scope.
- `SECURITY.md` — threat model and secret-handling rules.
- `cmd/workmail/` — CLI lifecycle only (`stdio`, `serve`, `doctor`, `token`, `version`, `help`).
- `internal/config/` — env-only config + validation (loopback, bounds, `*_FILE` secrets).
- `internal/imap/` — IMAPS backend (fresh connection per operation; `ReadOnly: true` for reads, read-write only inside confirmed writes) + verified-TLS SMTP send.
- `internal/mail/` — domain types + MIME parser + `Service`/`WriteService` interfaces + write validation (MCP must depend on these, never raw IMAP).
- `internal/mcp/` — read tools + two-phase write tools (`confirm.go` pending store) + stdio/HTTP transports.
- `docs/WRITE.md` — two-phase confirmation design, limits, audit rules.
- `internal/auth/` — HTTP bearer-token middleware.
- `scripts/` + `integration/` — `check.cmd`, `build.cmd`, GreenMail suite + Python fixtures/drivers.

## Go conventions
- Prefer the standard library where practical.
- Keep dependencies small and actively maintained.
- Use `context.Context` for I/O operations and cancellation.
- Set explicit network timeouts.
- Wrap errors with context using `%w`.
- Keep packages small and purpose-specific.
- Avoid global mutable state.
- Prefer interfaces at subsystem boundaries, not everywhere.
- New exported identifiers require useful Go doc comments.

## Testing
Every change should keep the following passing:

```text
go test ./...
go vet ./...
```

Mailbox behavior should be tested behind interfaces or fakes where possible. Tests must never require production mailbox credentials.

CI also runs `scripts/integration-test.sh` against a disposable pinned GreenMail server. The integration suite must remain deterministic and must use only generated test credentials, certificates, and mailbox fixtures. It should exercise the real compiled binary over verified IMAPS and MCP stdio, including all seven read tools and the full two-phase write flow (flags, move, delete, send). Do not weaken production TLS behavior for tests: certificate-chain and hostname verification must remain enabled, and `InsecureSkipVerify` must not be introduced.

## Definition of done for a change

- `gofmt -l .` is empty, `go vet ./...` and `go test ./...` pass.
- No write path that bypasses `mail_confirm`, no `InsecureSkipVerify`, no non-loopback HTTP default.
- New exported identifiers have Go doc comments; errors wrap with `%w`.
- No secrets, hostnames, mailbox data, or tokens in code, docs, or tests.
- Docs updated if behavior, config, or tool surface changed (`README.md` + `docs/` as needed).

## CI and releases
GitHub Actions is the source of truth for validation and binary builds.

- Pull requests and pushes to `master` must run formatting checks, `go vet`, `go test`, and the GreenMail IMAPS integration suite.
- Build CI should produce Windows and Linux amd64 binaries.
- Releases are generated from version tags or the manual Release workflow on `master`.

## Secrets and local configuration
Use environment variables or ignored local configuration files. Provide only safe example configuration in the repository.

Never add working credentials to test fixtures, examples, documentation, issue text, commits, or CI configuration. Deterministic integration-test credentials that exist only inside the disposable local test server are allowed, but they must never be reused for a real system.

## Change discipline
Security-sensitive changes should be small and reviewable.

Prefer this sequence:
1. configuration and interfaces
2. IMAP read-only backend
3. service layer
4. MCP stdio transport
5. MCP HTTP localhost transport and bearer auth
6. client-specific setup documentation

Do not introduce databases, queues, vector stores, background synchronization, containers, or orchestration unless a concrete need appears. The disposable GreenMail container used exclusively for integration testing is the intentional exception.
