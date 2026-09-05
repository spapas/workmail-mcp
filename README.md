# workmail-mcp

`workmail-mcp` is a small, local, security-focused **read-only** bridge between an IMAP mailbox (initially Zimbra) and MCP-capable AI clients.

The primary target is Windows. One binary supports:

- **Hermes** through MCP over `stdio`
- **ChatGPT-compatible remote MCP access** through authenticated streamable HTTP on loopback, intended to be paired with OpenAI Secure MCP Tunnel when the ChatGPT account/workspace supports custom MCP apps

## Security boundary

The project deliberately does **not** implement:

- SMTP / sending mail
- delete, move, expunge, or flag mutation
- folder creation
- arbitrary/raw IMAP commands
- a public network listener

IMAP is always implicit TLS (`IMAPS`, normally port 993) with normal certificate and hostname verification. HTTP mode refuses non-loopback bind addresses and requires a bearer token. Email bodies and attachments are treated as untrusted data.

See [SECURITY.md](SECURITY.md) for the threat model.

## MCP tools

| Tool | Purpose |
| --- | --- |
| `mail_list_folders` | List visible IMAP folders |
| `mail_search` | Structured full-text/header/date search returning metadata |
| `mail_recent` | Recent message metadata |
| `mail_get` | One bounded message body plus attachment metadata |
| `mail_get_attachment` | One bounded attachment as base64 |
| `mail_get_thread` | Bounded thread reconstruction using mail identifiers |

All tools are read-only.

## Install

GitHub Actions builds `windows/amd64` and `linux/amd64` binaries on every push and pull request. Tagged releases are configured to publish versioned binaries and SHA-256 checksums.

For Windows, place `workmail-mcp-windows-amd64.exe` somewhere such as:

```text
C:\Tools\workmail-mcp\workmail-mcp.exe
```

Renaming the downloaded binary is fine.

## Configuration

Configuration is environment-only; the program intentionally does **not** auto-load `.env` files. Secrets can be supplied directly or, preferably, from local files.

Required for IMAP operations:

```text
WORKMAIL_IMAP_HOST
WORKMAIL_IMAP_PORT=993
WORKMAIL_IMAP_USERNAME
WORKMAIL_IMAP_PASSWORD       OR WORKMAIL_IMAP_PASSWORD_FILE
```

HTTP mode additionally requires:

```text
WORKMAIL_HTTP_ADDR=127.0.0.1:8787
WORKMAIL_API_TOKEN           OR WORKMAIL_API_TOKEN_FILE
```

Optional bounded defaults:

```text
WORKMAIL_DEFAULT_FOLDER=INBOX
WORKMAIL_MAX_RESULTS=50
WORKMAIL_MAX_QUERY_LENGTH=512
WORKMAIL_MAX_MESSAGE_BYTES=26214400
WORKMAIL_MAX_BODY_BYTES=524288
WORKMAIL_MAX_ATTACHMENT_BYTES=10485760
WORKMAIL_MAX_THREAD_MESSAGES=50
WORKMAIL_OPERATION_TIMEOUT=30s
```

`*_FILE` values point to a local text file containing only the secret. If both the direct value and its `_FILE` variant are set, startup fails rather than choosing one silently.

### Windows CMD example

```cmd
set WORKMAIL_IMAP_HOST=mail.example.org
set WORKMAIL_IMAP_PORT=993
set WORKMAIL_IMAP_USERNAME=my-user@example.org
set WORKMAIL_IMAP_PASSWORD_FILE=C:\Users\me\.workmail-mcp\imap-password.txt
set WORKMAIL_DEFAULT_FOLDER=INBOX
```

Generate an HTTP bearer token without using an external utility:

```cmd
workmail-mcp.exe token > C:\Users\me\.workmail-mcp\api-token.txt
set WORKMAIL_API_TOKEN_FILE=C:\Users\me\.workmail-mcp\api-token.txt
```

Protect secret files with Windows ACLs appropriate to your account and do not place them in the repository.

## Commands

```text
workmail-mcp stdio                     MCP over stdin/stdout
workmail-mcp serve                     authenticated streamable HTTP MCP on loopback
workmail-mcp doctor                    test TLS, IMAP login, and folder listing
workmail-mcp doctor --latest-subject   also test read-only message metadata access
workmail-mcp token                     generate a 256-bit bearer token
workmail-mcp version                   print build version
workmail-mcp help                      usage
```

Run the privacy-safe connectivity check before configuring a client:

```cmd
workmail-mcp.exe doctor
```

A successful basic check prints only a success status; credentials and mailbox content are not printed.

For a stronger end-to-end IMAP diagnostic, explicitly opt in to a message metadata probe:

```cmd
workmail-mcp.exe doctor --latest-subject
```

This performs the normal TLS/login/folder checks and then selects `WORKMAIL_DEFAULT_FOLDER` read-only, searches for the newest message, fetches envelope metadata only, and prints only its subject. It does **not** fetch the message body or attachments. The subject is untrusted mailbox data, is normalized to one line, and is truncated to 200 Unicode characters before display.

Example:

```text
OK: IMAPS TLS, authentication, and folder listing succeeded
OK: latest subject in INBOX [untrusted mailbox data]: Example message subject
```

If the configured default folder is empty, the command reports that the metadata probe succeeded and that the folder contains no messages.

## Manual MCP stdio smoke tests on Windows

You can test the MCP `stdio` transport directly, without Hermes or ChatGPT. Run these commands from the same Command Prompt where the required `WORKMAIL_*` variables are available.

> **Windows note:** avoid piping multiple MCP JSON messages with `echo (...) | workmail-mcp.exe stdio`. `cmd.exe` writes CRLF line endings and this can cause framing errors such as `invalid trailing data at the end of stream`. The examples below use Python to write newline-delimited JSON (`\n`) and keep the bidirectional stdio session open while responses are read.

The examples assume the release binary is named `workmail-mcp-windows-amd64.exe` and that the Python launcher is available as `py -3`.

### 1. Verify MCP initialize and `tools/list`

```cmd
py -3 -c "import subprocess,json; p=subprocess.Popen(['workmail-mcp-windows-amd64.exe','stdio'],stdin=subprocess.PIPE,stdout=subprocess.PIPE); send=lambda x:(p.stdin.write(json.dumps(x,separators=(',',':')).encode()+b'\n'),p.stdin.flush()); send({'jsonrpc':'2.0','id':1,'method':'initialize','params':{'protocolVersion':'2025-11-25','capabilities':{},'clientInfo':{'name':'manual-test','version':'1.0'}}}); print('INIT:',p.stdout.readline().decode().strip()); send({'jsonrpc':'2.0','method':'notifications/initialized','params':{}}); send({'jsonrpc':'2.0','id':2,'method':'tools/list','params':{}}); print('TOOLS:',p.stdout.readline().decode().strip()); p.stdin.close(); p.terminate()"
```

A successful response should identify `workmail-mcp` and list these six tools:

```text
mail_list_folders
mail_search
mail_recent
mail_get
mail_get_attachment
mail_get_thread
```

### 2. Call `mail_list_folders` through MCP

This verifies the full path from a manual MCP client through `stdio` to the live IMAP mailbox:

```cmd
py -3 -c "import subprocess,json; p=subprocess.Popen(['workmail-mcp-windows-amd64.exe','stdio'],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE); send=lambda x:(p.stdin.write(json.dumps(x,separators=(',',':')).encode()+b'\n'),p.stdin.flush()); send({'jsonrpc':'2.0','id':1,'method':'initialize','params':{'protocolVersion':'2025-11-25','capabilities':{},'clientInfo':{'name':'manual-test','version':'1.0'}}}); p.stdout.readline(); send({'jsonrpc':'2.0','method':'notifications/initialized','params':{}}); send({'jsonrpc':'2.0','id':2,'method':'tools/call','params':{'name':'mail_list_folders','arguments':{}}}); print(p.stdout.readline().decode().strip()); p.terminate()"
```

The result should contain mailbox folders such as `INBOX` and any other folders visible to the configured IMAP account.

### 3. Call `mail_recent` through MCP

This asks for up to five messages from the last seven days and returns metadata only:

```cmd
py -3 -c "import subprocess,json; p=subprocess.Popen(['workmail-mcp-windows-amd64.exe','stdio'],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE); send=lambda x:(p.stdin.write(json.dumps(x,separators=(',',':')).encode()+b'\n'),p.stdin.flush()); send({'jsonrpc':'2.0','id':1,'method':'initialize','params':{'protocolVersion':'2025-11-25','capabilities':{},'clientInfo':{'name':'manual-test','version':'1.0'}}}); p.stdout.readline(); send({'jsonrpc':'2.0','method':'notifications/initialized','params':{}}); send({'jsonrpc':'2.0','id':2,'method':'tools/call','params':{'name':'mail_recent','arguments':{'folder':'INBOX','days':7,'limit':5}}}); print(p.stdout.readline().decode().strip()); p.terminate()"
```

Together, the diagnostic paths separate failures cleanly:

```text
doctor
  -> TLS / IMAP / authentication / folder listing

doctor --latest-subject
  -> read-only folder SELECT / SEARCH / envelope FETCH

manual stdio initialize + tools/list
  -> MCP stdio transport and tool schemas

manual tools/call
  -> MCP -> mail service -> live IMAP end-to-end
```

## Hermes

Hermes can launch the local binary directly through MCP stdio. Ensure the `WORKMAIL_*` environment variables are visible to the Hermes process, then configure a local MCP server similar to:

```cmd
hermes mcp add workmail --command C:\Tools\workmail-mcp\workmail-mcp.exe --args stdio
hermes mcp test workmail
```

No localhost port or API token is required for `stdio` mode.

See [docs/CLIENTS.md](docs/CLIENTS.md) for notes and troubleshooting.

## ChatGPT / Secure MCP Tunnel

ChatGPT does not call `127.0.0.1` directly. Run:

```cmd
workmail-mcp.exe serve
```

which exposes only:

```text
http://127.0.0.1:8787/mcp
```

and requires `Authorization: Bearer <token>`.

For a private developer-machine MCP server, OpenAI's supported architecture is **Secure MCP Tunnel**. The tunnel client makes an outbound HTTPS connection and forwards MCP requests to the private server without opening an inbound firewall port. Current ChatGPT custom-MCP availability is plan/workspace dependent; check current OpenAI documentation before setup. Details are in [docs/CLIENTS.md](docs/CLIENTS.md).

## Development

The module targets Go 1.27.

```text
gofmt -w .
go vet ./...
go test ./...
go build ./cmd/workmail
```

On Windows CMD:

```cmd
scripts\check.cmd
scripts\build.cmd
```

### Automated IMAPS integration test

CI also runs a deterministic integration suite against a disposable, pinned GreenMail server. The suite does not use production credentials or mailbox data.

It:

- generates an ephemeral test CA and a `localhost` server certificate;
- starts GreenMail with SMTP for fixture injection and IMAPS for the application under test;
- keeps TLS certificate-chain and hostname verification enabled;
- seeds four known messages, including an attachment and a two-message thread;
- builds the real `workmail-mcp` binary;
- verifies `doctor --latest-subject`;
- drives the real binary over MCP `stdio` and exercises all six read-only MCP tools.

On Linux, or in a Linux environment with Docker, Python 3, OpenSSL, and Go installed, run the same suite with:

```text
bash scripts/integration-test.sh
```

The generated CA, certificate, test mailbox, password, messages, container, and temporary files exist only for the test run and are removed afterward. The test never enables `InsecureSkipVerify` or another TLS-verification bypass.

The code is intentionally split so MCP handlers cannot issue raw IMAP commands directly:

```text
cmd/workmail/       CLI and process lifecycle
internal/config/    validated environment configuration
internal/mail/      domain types and MIME parser
internal/imap/      TLS/read-only IMAP implementation
internal/auth/      bearer-token HTTP middleware
internal/mcp/       bounded MCP tools and transports
```

## Operational model

There is no mailbox mirror, database, Redis, vector store, or background index. Requests perform live, bounded IMAP operations. Whole-message download is allowed only after checking the server-reported RFC822 size against the configured maximum.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) and [docs/ROADMAP.md](docs/ROADMAP.md).
