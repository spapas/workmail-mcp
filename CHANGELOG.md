# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

## [0.4.0] - 2026-10-13

### Added

- Read-only `mail_stats` tool: total/unseen counts per folder via IMAP
  STATUS (counts only, no message content, no flag changes).

## [0.3.0] - 2026-10-12

### Added

- IMAP flags (`\Seen`, `\Flagged`, `\Answered`, `\Draft`, `\Deleted`) in
  message summaries returned by `mail_search`, `mail_recent`, `mail_get`,
  and `mail_get_thread` (via `FETCH FLAGS`, read-only; reading still never
  sets `\Seen`).

## [0.2.0] - 2026-10-10

### Added

- Starter governance: `LICENSE` (MIT), `CONTRIBUTING.md`, this changelog, `.editorconfig`, `.gitattributes`.
- Agent-oriented `AGENTS.md` sections: quick commands, repo map, definition of done.
- Two-phase write operations with mandatory confirmation (`docs/WRITE.md`):
  `mail_send_request`, `mail_move_request`, `mail_delete_request`,
  `mail_set_flags_request` plus `mail_confirm` (single-use expiring IDs, frozen
  payloads, no direct write path). Verified-TLS SMTP send with IMAP-identity
  defaults; IMAP move (MOVE or COPY fallback), delete (STORE `\Deleted` +
  expunge), and allowlisted flag mutation; new SMTP config, `WORKMAIL_MAX_RECIPIENTS`,
   and `WORKMAIL_CONFIRM_TTL` bounds.
- GreenMail IMAPS integration coverage for the write flow
  (`integration/mcp_write.py`): request + confirm for flags, move, delete,
  and send, plus guardrail assertions (validation rejections, single-use and
  unknown confirmation IDs).

## [0.1.1] - 2026-10-10

### Added

- GreenMail IMAPS integration suite (read tools), `doctor --latest-subject`
  probe, and manual MCP stdio smoke tests.
- MCP Go SDK 1.7.0.

## [0.1.0] - 2026-10-10

### Added

- Read-only IMAPS bridge with six MCP tools: `mail_list_folders`, `mail_search`, `mail_recent`, `mail_get`, `mail_get_attachment`, `mail_get_thread`.
- MCP stdio transport (Hermes) and loopback-only authenticated HTTP transport (ChatGPT Secure MCP Tunnel path).
- Env-only config with `*_FILE` secrets, `doctor`, `token`, `version` commands.
- Deterministic GreenMail IMAPS integration suite covering all six tools.
- Docs: `README.md`, `docs/ARCHITECTURE.md`, `docs/CLIENTS.md`, `docs/ROADMAP.md`, `SECURITY.md`.
