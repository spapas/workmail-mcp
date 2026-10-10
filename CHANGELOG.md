# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

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

## [0.1.0] - 2026-10-10

### Added

- Read-only IMAPS bridge with six MCP tools: `mail_list_folders`, `mail_search`, `mail_recent`, `mail_get`, `mail_get_attachment`, `mail_get_thread`.
- MCP stdio transport (Hermes) and loopback-only authenticated HTTP transport (ChatGPT Secure MCP Tunnel path).
- Env-only config with `*_FILE` secrets, `doctor`, `token`, `version` commands.
- Deterministic GreenMail IMAPS integration suite covering all six tools.
- Docs: `README.md`, `docs/ARCHITECTURE.md`, `docs/CLIENTS.md`, `docs/ROADMAP.md`, `SECURITY.md`.
