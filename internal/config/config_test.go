package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateLoopbackAddr(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:8787", "[::1]:8787"} {
		if err := ValidateLoopbackAddr(addr); err != nil {
			t.Fatalf("%s should be accepted: %v", addr, err)
		}
	}
	for _, addr := range []string{"0.0.0.0:8787", "192.0.2.1:8787", "localhost:8787"} {
		if err := ValidateLoopbackAddr(addr); err == nil {
			t.Fatalf("%s should be rejected", addr)
		}
	}
}

func TestLoadSecretFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "password.txt")
	if err := os.WriteFile(path, []byte("secret-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMAIL_IMAP_PASSWORD", "")
	t.Setenv("WORKMAIL_IMAP_PASSWORD_FILE", path)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.IMAPPassword != "secret-value" {
		t.Fatalf("unexpected secret: %q", cfg.IMAPPassword)
	}
}

func TestLoadRejectsAmbiguousSecret(t *testing.T) {
	t.Setenv("WORKMAIL_API_TOKEN", "one")
	t.Setenv("WORKMAIL_API_TOKEN_FILE", "two")
	if _, err := Load(); err == nil {
		t.Fatal("expected ambiguous secret error")
	}
}

func TestValidateLimitsRejectsBadIMAPPort(t *testing.T) {
	cfg := Config{
		IMAPPort:           0,
		SMTPPort:           465,
		MaxResults:         50,
		MaxQueryLength:     512,
		MaxMessageBytes:    1024,
		MaxBodyBytes:       1024,
		MaxAttachmentBytes: 1024,
		MaxThreadMessages:  50,
		MaxRecipients:      50,
		ConfirmTTL:         5 * time.Minute,
		OperationTimeout:   30 * time.Second,
	}
	if err := cfg.validateLimits(); err == nil {
		t.Fatal("expected IMAP port range error")
	}
}
