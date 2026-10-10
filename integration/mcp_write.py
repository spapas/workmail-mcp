#!/usr/bin/env python3
"""Two-phase write flow over MCP stdio against GreenMail.

Runs after mcp_smoke.py (read suite). Exercises all five write tools:
mail_*_request to freeze an operation, mail_confirm to execute it once.
Also asserts the guardrails: input validation rejects bad payloads before
anything is frozen, confirmation IDs are single-use, and unknown IDs fail.

IMAP cross-checks use imaplib directly (test harness only, never the product).
"""

from __future__ import annotations

import imaplib
import os
import ssl
import sys
import time

from mcp_smoke import MCPClient, exact_subject

SEND_SUBJECT = "Integration send via confirm"
SEND_MARKER = "send-via-confirm body marker"


def raw_call(client: MCPClient, name: str, arguments: dict) -> dict:
    return client.request("tools/call", {"name": name, "arguments": arguments})


def expect_error(result: dict, what: str) -> None:
    if not isinstance(result, dict) or result.get("isError") is not True:
        raise AssertionError(f"{what}: expected isError tool result, got {result!r}")


def search(client: MCPClient, folder: str, subject: str) -> list:
    return client.call_tool(
        "mail_search", {"folder": folder, "subject": subject, "limit": 10}
    )["messages"]


def imap_connection() -> imaplib.IMAP4_SSL:
    ca_file = os.environ["SSL_CERT_FILE"]
    context = ssl.create_default_context(cafile=ca_file)
    imap = imaplib.IMAP4_SSL("127.0.0.1", 3993, ssl_context=context)
    imap.login("integration", "integration-secret")
    return imap


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
                "clientInfo": {"name": "greenmail-write-integration", "version": "1.0"},
            },
        )
        assert init["serverInfo"]["name"] == "workmail-mcp", init
        client.notify("notifications/initialized")

        # --- validation guardrails: nothing is frozen, nothing executes ---
        expect_error(
            raw_call(
                client,
                "mail_set_flags_request",
                {"folder": "INBOX", "uid": 1, "add": ["\\Deleted"]},
            ),
            "flag \\Deleted",
        )
        expect_error(
            raw_call(
                client,
                "mail_send_request",
                {"subject": "no recipients", "body_text": "x"},
            ),
            "send without recipients",
        )
        expect_error(
            raw_call(
                client,
                "mail_send_request",
                {"to": ["not-an-email"], "body_text": "x"},
            ),
            "send with bad address",
        )
        expect_error(
            raw_call(
                client,
                "mail_move_request",
                {"folder": "INBOX", "uid": 1, "dest": "INBOX"},
            ),
            "move onto itself",
        )
        expect_error(
            raw_call(client, "mail_confirm", {"pending_id": "does-not-exist"}),
            "confirm unknown id",
        )

        # --- flags: freeze then confirm once ---
        plain = exact_subject(search(client, "INBOX", "Integration plain message"), "Integration plain message")
        plain_uid = int(plain["uid"])
        pending = client.call_tool(
            "mail_set_flags_request",
            {"folder": "INBOX", "uid": plain_uid, "add": ["Seen"]},
        )
        assert pending["kind"] == "set_flags", pending
        assert pending["pending_id"], pending
        confirmed = client.call_tool("mail_confirm", {"pending_id": pending["pending_id"]})
        assert confirmed["kind"] == "set_flags", confirmed
        assert "\\Seen" in (confirmed.get("flags") or {}).get("added", []), confirmed
        expect_error(
            raw_call(client, "mail_confirm", {"pending_id": pending["pending_id"]}),
            "reused flags confirmation",
        )

        # --- move: freeze then confirm once, verify in both folders ---
        imap = imap_connection()
        try:
            typ, _ = imap.create("Archive")
            assert typ == "OK", typ
        finally:
            imap.logout()
        reply = exact_subject(search(client, "INBOX", "Re: Integration thread"), "Re: Integration thread")
        reply_uid = int(reply["uid"])
        pending = client.call_tool(
            "mail_move_request",
            {"folder": "INBOX", "uid": reply_uid, "dest": "Archive"},
        )
        assert pending["kind"] == "move", pending
        confirmed = client.call_tool("mail_confirm", {"pending_id": pending["pending_id"]})
        assert (confirmed.get("move") or {}).get("moved") is True, confirmed
        expect_error(
            raw_call(client, "mail_confirm", {"pending_id": pending["pending_id"]}),
            "reused move confirmation",
        )
        assert search(client, "INBOX", "Re: Integration thread") == [], "reply must be gone from INBOX"
        archived = search(client, "Archive", "Integration thread")
        assert any(item.get("subject") == "Re: Integration thread" for item in archived), archived
        imap = imap_connection()
        try:
            typ, _ = imap.select("Archive")
            assert typ == "OK", typ
            typ, data = imap.search(None, "ALL")
            assert typ == "OK" and data and data[0].strip(), (typ, data)
        finally:
            imap.logout()

        # --- delete: freeze then confirm once, message disappears ---
        disposable = exact_subject(search(client, "INBOX", "Integration disposable"), "Integration disposable")
        disposable_uid = int(disposable["uid"])
        pending = client.call_tool(
            "mail_delete_request", {"folder": "INBOX", "uid": disposable_uid}
        )
        assert pending["kind"] == "delete", pending
        confirmed = client.call_tool("mail_confirm", {"pending_id": pending["pending_id"]})
        assert (confirmed.get("delete") or {}).get("deleted") is True, confirmed
        expect_error(
            raw_call(client, "mail_confirm", {"pending_id": pending["pending_id"]}),
            "reused delete confirmation",
        )
        assert search(client, "INBOX", "Integration disposable") == [], "disposable must be gone"
        expect_error(
            raw_call(client, "mail_get", {"folder": "INBOX", "uid": disposable_uid}),
            "get of deleted message",
        )

        # --- send: freeze then confirm once, message arrives via SMTP ---
        pending = client.call_tool(
            "mail_send_request",
            {
                "to": ["integration@localhost"],
                "subject": SEND_SUBJECT,
                "body_text": f"{SEND_MARKER}\n",
            },
        )
        assert pending["kind"] == "send", pending
        assert "body_text" not in pending and SEND_MARKER not in str(pending), pending
        confirmed = client.call_tool("mail_confirm", {"pending_id": pending["pending_id"]})
        sent = confirmed.get("send") or {}
        assert sent.get("to_count") == 1 and sent.get("message_id"), confirmed
        expect_error(
            raw_call(client, "mail_confirm", {"pending_id": pending["pending_id"]}),
            "reused send confirmation",
        )
        arrived = []
        for _ in range(30):
            arrived = search(client, "INBOX", SEND_SUBJECT)
            if arrived:
                break
            time.sleep(1)
        sent_summary = exact_subject(arrived, SEND_SUBJECT)
        fetched = client.call_tool(
            "mail_get", {"folder": "INBOX", "uid": int(sent_summary["uid"])}
        )["message"]
        assert SEND_MARKER in fetched.get("body_text", ""), fetched

        print("OK: MCP write integration suite passed")
        print("OK: request+confirm for flags, move, delete, and send passed, guardrails held")
        return 0
    finally:
        client.close()


if __name__ == "__main__":
    sys.exit(main())
