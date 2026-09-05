package mail

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestParseRawMultipart(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString([]byte("attachment data"))
	raw := strings.Join([]string{
		"From: Sender <sender@example.test>",
		"To: Receiver <receiver@example.test>",
		"Subject: parser test",
		"Message-ID: <child@example.test>",
		"References: <root@example.test>",
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="b"`,
		"",
		"--b",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"hello world",
		"--b",
		`Content-Type: application/octet-stream; name="test.bin"`,
		`Content-Disposition: attachment; filename="test.bin"`,
		"Content-Transfer-Encoding: base64",
		"",
		payload,
		"--b--",
		"",
	}, "\r\n")

	parsed, err := ParseRaw([]byte(raw), 1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(parsed.Text, "hello world") {
		t.Fatalf("unexpected text: %q", parsed.Text)
	}
	if len(parsed.References) != 1 || parsed.References[0] != "root@example.test" {
		t.Fatalf("unexpected references: %#v", parsed.References)
	}
	if len(parsed.Attachments) != 1 {
		t.Fatalf("expected one attachment, got %d", len(parsed.Attachments))
	}
	if got := string(parsed.Attachments[0].Data); got != "attachment data" {
		t.Fatalf("unexpected attachment data: %q", got)
	}
}

func TestParseRawBodyLimit(t *testing.T) {
	raw := "Subject: x\r\nContent-Type: text/plain\r\n\r\n0123456789"
	parsed, err := ParseRaw([]byte(raw), 5, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Text != "01234" || !parsed.Truncated {
		t.Fatalf("unexpected bounded body: %q truncated=%v", parsed.Text, parsed.Truncated)
	}
}
