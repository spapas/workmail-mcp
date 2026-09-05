# Client setup

## Hermes (recommended local path)

Hermes can run `workmail-mcp` directly as a local stdio MCP server. This is the simplest path because there is no network listener and no bearer token.

1. Configure the `WORKMAIL_IMAP_*` environment variables in the environment that launches Hermes.
2. Verify the mailbox first with `workmail-mcp.exe doctor`.
3. Add the server:

```cmd
hermes mcp add workmail --command C:\Tools\workmail-mcp\workmail-mcp.exe --args stdio
hermes mcp test workmail
```

If your Hermes version uses a configuration file instead, configure `workmail-mcp.exe stdio` as a local MCP stdio command. Hermes configuration syntax can change; use its current MCP documentation as the source of truth.

## ChatGPT and OpenAI Secure MCP Tunnel

The workmail HTTP mode is intentionally private:

```text
http://127.0.0.1:8787/mcp
```

ChatGPT cannot connect directly to a local MCP listener. OpenAI documents Secure MCP Tunnel for private/on-premises/developer-machine MCP servers. The tunnel is outbound-only from the machine running the local server.

At the time this document was written (September 2026), OpenAI's ChatGPT developer-mode/custom-MCP availability is workspace/plan dependent. The Help Center lists full MCP for Business and Enterprise/Edu, with Pro supporting read/fetch MCP in developer mode; Plus is not listed as supporting custom MCP. Treat this as a product limitation, not a server limitation, and re-check the current documentation because rollout policy can change.

### HTTP local side

Generate a random token:

```cmd
workmail-mcp.exe token > C:\Users\me\.workmail-mcp\api-token.txt
set WORKMAIL_API_TOKEN_FILE=C:\Users\me\.workmail-mcp\api-token.txt
workmail-mcp.exe serve
```

The server rejects `0.0.0.0`, LAN addresses, hostnames, and public addresses. Only numeric loopback addresses such as `127.0.0.1` and `::1` are accepted.

### Tunnel side

Use the current OpenAI Secure MCP Tunnel documentation and `tunnel-client` release. The OpenAI tunnel client needs a tunnel ID/runtime API key and can be pointed at the local MCP HTTP server. Because `workmail-mcp` requires a bearer token, configure the tunnel/client-side MCP authentication mechanism according to the current tunnel tooling rather than removing authentication from this service.

Do not expose port 8787 through router/NAT/firewall rules as a workaround.

## Troubleshooting

- Start with `workmail-mcp.exe doctor`; it isolates TLS/login/folder issues from MCP issues.
- For stdio mode, never redirect server stdout to logging. MCP owns stdout; this program logs only to stderr.
- For HTTP mode, a missing or incorrect bearer token returns HTTP 401.
- If TLS validation fails, fix the server certificate/trust chain. There is intentionally no `InsecureSkipVerify` setting.
- If a message or attachment exceeds configured limits, increase a limit deliberately rather than disabling size checks.
