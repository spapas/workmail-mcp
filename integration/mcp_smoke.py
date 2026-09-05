#!/usr/bin/env python3
from __future__ import annotations

import base64
import json
import os
import subprocess
import sys
from typing import Any

EXPECTED_TOOLS = {
    "mail_list_folders",
    "mail_search",
    "mail_recent",
    "mail_get",
    "mail_get_attachment",
    "mail_get_thread",
}


class MCPClient:
    def __init__(self, binary: str) -> None:
        self._next_id = 1
        self._proc = subprocess.Popen(
            [binary, "stdio"],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            env=os.environ.copy(),
        )
        if self._proc.stdin is None or self._proc.stdout is None or self._proc.stderr is None:
            raise RuntimeError("failed to open MCP stdio pipes")

    def close(self) -> None:
        if self._proc.poll() is None:
            self._proc.terminate()
            try:
                self._proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                self._proc.kill()
                self._proc.wait(timeout=5)

    def notify(self, method: str, params: dict[str, Any] | None = None) -> None:
        self._send({"jsonrpc": "2.0", "method": method, "params": params or {}})

    def request(self, method: str, params: dict[str, Any] | None = None) -> dict[str, Any]:
        request_id = self._next_id
        self._next_id += 1
        self._send({"jsonrpc": "2.0", "id": request_id, "method": method, "params": params or {}})

        line = self._proc.stdout.readline()
        if not line:
            stderr = self._proc.stderr.read().decode("utf-8", errors="replace")
            raise RuntimeError(f"MCP server closed before response to {method}: {stderr}")
        response = json.loads(line.decode("utf-8"))
        if response.get("id") != request_id:
            raise AssertionError(f"response id mismatch for {method}: {response}")
        if "error" in response:
            raise AssertionError(f"MCP error for {method}: {response['error']}")
        return response["result"]

    def call_tool(self, name: str, arguments: dict[str, Any]) -> dict[str, Any]:
        result = self.request("tools/call", {"name": name, "arguments": arguments})
        structured = result.get("structuredContent")
        if isinstance(structured, dict):
            return structured
        for item in result.get("content", []):
            if item.get("type") == "text":
                try:
                    decoded = json.loads(item.get("text", ""))
                except json.JSONDecodeError:
                    continue
                if isinstance(decoded, dict):
                    return decoded
        raise AssertionError(f"tool {name} returned no structured JSON content: {result}")

    def _send(self, message: dict[str, Any]) -> None:
        payload = json.dumps(message, separators=(",", ":")).encode("utf-8") + b"\n"
        self._proc.stdin.write(payload)
        self._proc.stdin.flush()


def exact_subject(messages: list[dict[str, Any]], subject: str) -> dict[str, Any]:
    for item in messages:
        if item.get("subject") == subject:
            return item
    raise AssertionError(f"subject {subject!r} not found in {messages!r}")


def main() -> int:
    if len(sys.argv) != 2:
        print(f"usage: {sys.argv[0]} /path/to/workmail-mcp", file=sys.stderr)
        return 2

    client = MCPClient(sys.argv[1])
    try:
        init = client.request(
            "initialize",
            {
                "protocolVersion": "2025-11-25",
                "capabilities": {},
                "clientInfo": {"name": "greenmail-integration", "version": "1.0"},
            },
        )
        assert init["serverInfo"]["name"] == "workmail-mcp", init
        client.notify("notifications/initialized")

        tools = client.request("tools/list")
        names = {tool["name"] for tool in tools["tools"]}
        assert names == EXPECTED_TOOLS, names

        folders = client.call_tool("mail_list_folders", {})["folders"]
        assert any(folder.get("name") == "INBOX" for folder in folders), folders

        recent = client.call_tool(
            "mail_recent", {"folder": "INBOX", "days": 1, "limit": 10}
        )["messages"]
        recent_subjects = {item.get("subject") for item in recent}
        assert {
            "Integration plain message",
            "Integration attachment",
            "Integration thread",
            "Re: Integration thread",
        }.issubset(recent_subjects), recent_subjects

        attachment_hits = client.call_tool(
            "mail_search",
            {"folder": "INBOX", "subject": "Integration attachment", "limit": 10},
        )["messages"]
        attachment_summary = exact_subject(attachment_hits, "Integration attachment")
        attachment_uid = int(attachment_summary["uid"])

        fetched = client.call_tool(
            "mail_get", {"folder": "INBOX", "uid": attachment_uid}
        )["message"]
        assert fetched["subject"] == "Integration attachment", fetched
        assert "attachment integration body marker" in fetched.get("body_text", ""), fetched
        attachments = fetched.get("attachments") or []
        assert len(attachments) == 1, attachments
        assert attachments[0].get("filename") == "hello.txt", attachments

        attachment = client.call_tool(
            "mail_get_attachment",
            {"folder": "INBOX", "uid": attachment_uid, "attachment_index": 0},
        )["attachment"]
        assert attachment.get("filename") == "hello.txt", attachment
        decoded = base64.b64decode(attachment["data_base64"])
        assert decoded == b"hello from workmail-mcp integration\n", decoded

        thread_hits = client.call_tool(
            "mail_search",
            {"folder": "INBOX", "subject": "Integration thread", "limit": 10},
        )["messages"]
        thread_root = exact_subject(thread_hits, "Integration thread")
        thread = client.call_tool(
            "mail_get_thread",
            {"folder": "INBOX", "uid": int(thread_root["uid"]), "limit": 10},
        )["messages"]
        thread_subjects = {item.get("subject") for item in thread}
        assert {"Integration thread", "Re: Integration thread"}.issubset(thread_subjects), thread

        print("OK: MCP stdio integration suite passed")
        print("OK: folders, recent, search, get, attachment, and thread tools passed")
        return 0
    finally:
        client.close()


if __name__ == "__main__":
    sys.exit(main())
