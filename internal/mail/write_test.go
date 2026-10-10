package mail

import (
	"strings"
	"testing"
)

func TestValidateSend(t *testing.T) {
	ok := SendRequest{To: []string{"a@example.com"}, Subject: "hi", BodyText: "hello"}
	if err := ValidateSend(&ok, 50, 1<<20, 998); err != nil {
		t.Fatalf("valid send rejected: %v", err)
	}
	noRecipient := SendRequest{Subject: "hi", BodyText: "x"}
	if err := ValidateSend(&noRecipient, 50, 1<<20, 998); err == nil {
		t.Fatal("expected recipient error")
	}
	bad := SendRequest{To: []string{"not-an-email"}, BodyText: "x"}
	if err := ValidateSend(&bad, 50, 1<<20, 998); err == nil {
		t.Fatal("expected address error")
	}
	big := SendRequest{To: []string{"a@example.com"}, BodyText: strings.Repeat("x", 100)}
	if err := ValidateSend(&big, 50, 10, 998); err == nil {
		t.Fatal("expected body limit error")
	}
	inject := SendRequest{To: []string{"a@example.com"}, Subject: "a\r\nBcc: x", BodyText: "x"}
	if err := ValidateSend(&inject, 50, 1<<20, 998); err == nil {
		t.Fatal("expected subject injection error")
	}
}

func TestNormalizeFlags(t *testing.T) {
	add, remove, err := NormalizeFlags([]string{"seen", "\\Flagged"}, []string{"draft"})
	if err != nil {
		t.Fatalf("valid flags rejected: %v", err)
	}
	if len(add) != 2 || len(remove) != 1 {
		t.Fatalf("unexpected normalization: %v / %v", add, remove)
	}
	if _, _, err := NormalizeFlags([]string{"\\Deleted"}, nil); err == nil {
		t.Fatal("expected \\Deleted rejection")
	}
	if _, _, err := NormalizeFlags([]string{"\\Seen"}, []string{"seen"}); err == nil {
		t.Fatal("expected add/remove conflict error")
	}
	if _, _, err := NormalizeFlags(nil, nil); err == nil {
		t.Fatal("expected empty flags error")
	}
}

func TestNormalizeFolder(t *testing.T) {
	got, err := NormalizeFolder("", "INBOX")
	if err != nil || got != "INBOX" {
		t.Fatalf("default folder failed: %q %v", got, err)
	}
	if _, err := NormalizeFolder("a\r\nb", "INBOX"); err == nil {
		t.Fatal("expected control character rejection")
	}
	if err := ValidateUID(0); err == nil {
		t.Fatal("expected zero UID rejection")
	}
}
