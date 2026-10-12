package imap

import (
	"strings"
	"testing"

	imapv2 "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	maildomain "github.com/spapas/workmail-mcp/internal/mail"
)

func TestBuildSendBytesHeaders(t *testing.T) {
	req := maildomain.SendRequest{
		To:       []string{"to@example.com"},
		Cc:       []string{"cc@example.com"},
		Bcc:      []string{"hidden@example.com"},
		Subject:  "hello",
		BodyText: "plain body",
		BodyHTML: "<p>html body</p>",
	}
	raw, msgID, err := buildSendBytes("me@example.com", req)
	if err != nil {
		t.Fatalf("buildSendBytes failed: %v", err)
	}
	s := string(raw)
	for _, want := range []string{"From: ", "To: <to@example.com>", "Cc: <cc@example.com>", "Subject: hello", "Message-ID: " + msgID, "multipart/alternative", "plain body", "<p>html body</p>"} {
		if !strings.Contains(s, want) {
			t.Fatalf("message missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "hidden@example.com") {
		t.Fatal("BCC must never appear in message headers")
	}
	if !strings.HasPrefix(msgID, "<") || !strings.HasSuffix(msgID, ">") {
		t.Fatalf("bad message ID: %q", msgID)
	}
}

func TestBuildSendBytesRequiresBody(t *testing.T) {
	// buildSendBytes itself renders whatever it is given; emptiness is
	// rejected one layer up by ValidateSend.
	req := maildomain.SendRequest{To: []string{"a@example.com"}, BodyText: "x"}
	if _, _, err := buildSendBytes("not-an-email", req); err == nil {
		t.Fatal("expected invalid from rejection")
	}
}

// TestSummaryFromBufferIncludesFlags ensures FETCH FLAGS are surfaced on
// message summaries so clients can distinguish Seen/Unseen messages.
func TestSummaryFromBufferIncludesFlags(t *testing.T) {
	item := &imapclient.FetchMessageBuffer{
		UID:   42,
		Flags: []imapv2.Flag{imapv2.FlagSeen, imapv2.FlagFlagged},
	}
	got := summaryFromBuffer("INBOX", item)
	if len(got.Flags) != 2 || got.Flags[0] != `\Flagged` || got.Flags[1] != `\Seen` {
		t.Fatalf("unexpected flags: %q", got.Flags)
	}
	empty := summaryFromBuffer("INBOX", &imapclient.FetchMessageBuffer{UID: 43})
	if len(empty.Flags) != 0 {
		t.Fatalf("expected no flags, got %q", empty.Flags)
	}
}
