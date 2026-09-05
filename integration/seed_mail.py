#!/usr/bin/env python3
from __future__ import annotations

import smtplib
import sys
from datetime import datetime, timedelta, timezone
from email.message import EmailMessage
from email.utils import format_datetime

HOST = "127.0.0.1"
PORT = 3025
RECIPIENT = "integration@example.com"
SENDER = "sender@example.org"


def message(subject: str, body: str, message_id: str, when: datetime) -> EmailMessage:
    msg = EmailMessage()
    msg["From"] = SENDER
    msg["To"] = RECIPIENT
    msg["Subject"] = subject
    msg["Date"] = format_datetime(when)
    msg["Message-ID"] = message_id
    msg.set_content(body)
    return msg


def main() -> int:
    now = datetime.now(timezone.utc)

    plain = message(
        "Integration plain message",
        "plain integration body marker\n",
        "<plain@workmail.test>",
        now - timedelta(minutes=4),
    )

    attachment = message(
        "Integration attachment",
        "attachment integration body marker\n",
        "<attachment@workmail.test>",
        now - timedelta(minutes=3),
    )
    attachment.add_attachment(
        b"hello from workmail-mcp integration\n",
        maintype="text",
        subtype="plain",
        filename="hello.txt",
    )

    thread_root = message(
        "Integration thread",
        "thread root body marker\n",
        "<thread-root@workmail.test>",
        now - timedelta(minutes=2),
    )

    thread_reply = message(
        "Re: Integration thread",
        "thread reply body marker\n",
        "<thread-reply@workmail.test>",
        now - timedelta(minutes=1),
    )
    thread_reply["In-Reply-To"] = "<thread-root@workmail.test>"
    thread_reply["References"] = "<thread-root@workmail.test>"

    with smtplib.SMTP(HOST, PORT, timeout=10) as smtp:
        for msg in (plain, attachment, thread_root, thread_reply):
            smtp.send_message(msg)

    print("Seeded 4 integration messages")
    return 0


if __name__ == "__main__":
    sys.exit(main())
