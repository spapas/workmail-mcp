package mail

import (
	"strings"
	"testing"
)

func TestNormalizeRefUID(t *testing.T) {
	r, err := NormalizeRef("", "INBOX", 42, "")
	if err != nil {
		t.Fatal(err)
	}

	if r.Folder != "INBOX" || r.UID != 42 || r.ID != "" {
		t.Fatalf("unexpected ref: %+v", r)
	}
}

func TestNormalizeRefID(t *testing.T) {
	r, err := NormalizeRef("Archive", "INBOX", 0, "abc123")
	if err != nil {
		t.Fatal(err)
	}

	if r.Folder != "Archive" || r.ID != "abc123" {
		t.Fatalf("unexpected ref: %+v", r)
	}
}

func TestNormalizeRefRejectsBothOrNeither(t *testing.T) {
	if _, err := NormalizeRef("INBOX", "INBOX", 42, "abc"); err == nil {
		t.Fatal("expected uid+id conflict error")
	}

	if _, err := NormalizeRef("INBOX", "INBOX", 0, ""); err == nil {
		t.Fatal("expected missing identity error")
	}
}

func TestNormalizeRefRejectsBadFolderOrID(t *testing.T) {
	if _, err := NormalizeRef("a\r\nb", "INBOX", 1, ""); err == nil {
		t.Fatal("expected folder control character error")
	}

	if _, err := NormalizeRef("INBOX", "INBOX", 0, "a\r\nb"); err == nil {
		t.Fatal("expected id control character error")
	}

	if _, err := NormalizeRef("INBOX", "INBOX", 0, strings.Repeat("x", 2048)); err == nil {
		t.Fatal("expected overlong id error")
	}
}
