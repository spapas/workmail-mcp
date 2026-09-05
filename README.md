# workmail-mcp

A local, security-focused, read-only bridge between an IMAP mailbox (initially Zimbra) and MCP-capable AI clients.

## Initial goals

- Windows-first local deployment
- Hermes support through MCP over stdio
- ChatGPT support through localhost MCP HTTP plus the supported ChatGPT connection path
- strict read-only mailbox access
- small Go codebase with minimal dependencies
- automatic CI, tests, vetting, formatting checks, and binary builds

## Security model

The initial implementation intentionally does **not** support sending, deleting, moving, expunging, flag mutation, folder creation, or arbitrary/raw IMAP commands.

Secrets stay outside the repository. HTTP mode will bind to loopback only and use bearer-token authentication. IMAP connections will use TLS with certificate and hostname verification.

See [SECURITY.md](SECURITY.md) and [AGENTS.md](AGENTS.md) for the project invariants.

## Repository status

The repository is currently in the foundation phase. The implementation sequence is documented in [docs/ROADMAP.md](docs/ROADMAP.md).

Current skeleton:

```text
cmd/workmail/       application entry point
internal/config/    configuration (planned)
internal/imap/      IMAP backend (planned)
internal/mail/      domain/service layer (planned)
internal/mcp/       MCP tools/transports (planned)
internal/auth/      localhost HTTP auth (planned)
```

## Development

Requires Go 1.27 or later in the 1.27 release line.

```text
go test ./...
go vet ./...
go build ./cmd/workmail
```

Local secrets must never be committed. `.env.example` contains placeholders only; `.env` and common secret/config files are ignored.

## CI and binaries

GitHub Actions currently:

- checks Go formatting
- runs `go vet ./...`
- runs `go test ./...`
- builds `windows/amd64` and `linux/amd64` binaries
- uploads those binaries as workflow artifacts

Tagged GitHub Releases with persistent downloadable binaries are planned as part of the hardening/release phase.
