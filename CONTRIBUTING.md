# Contributing to workmail-mcp

Thanks for helping. This project is security-sensitive (real mailboxes), so small, reviewable changes are preferred.

## Branching

- Primary branch is `master`. Branch from `master`, target `master` in PRs.
- Keep PRs focused; separate refactors from behavior changes.

## Before you push

Windows CMD (primary):

```cmd
scripts\check.cmd
scripts\build.cmd
```

Portable:

```text
gofmt -l .
go vet ./...
go test ./...
go build ./cmd/workmail
```

Full integration (needs Docker + Go + Python + OpenSSL):

```text
bash scripts/integration-test.sh
```

CI runs `gofmt` check, `go vet`, `go test`, and the GreenMail IMAPS integration suite on PRs and pushes to `master`.

## Rules that block a PR

- Read-only stays read-only: no SMTP send, delete/move/expunge, flag mutation, folder creation, or raw IMAP commands.
- No `InsecureSkipVerify`, no non-loopback HTTP default, no unauthenticated HTTP.
- No real credentials, tokens, hostnames, mailbox content, certificates, or body-bearing logs in code, docs, tests, or CI config.
- Treat email content as untrusted data; never log bodies, attachments, passwords, or tokens.
- Validate and bound all tool inputs; keep explicit timeouts.
- New exported identifiers need Go doc comments; wrap errors with `%w`.
- Update `README.md` + `docs/` when behavior, config, or tool surface changes.

See `AGENTS.md`, `SECURITY.md`, `docs/ARCHITECTURE.md`, and `docs/ROADMAP.md` before changing transports, backends, or the tool surface.
