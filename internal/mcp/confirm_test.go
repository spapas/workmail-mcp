package mcp

import (
	"testing"
	"time"
)

func TestPendingStoreSingleUse(t *testing.T) {
	s := newPendingStore(time.Minute)
	id, _, err := s.create(pendingPayload{Kind: "delete", Summary: "x"})
	if err != nil || id == "" {
		t.Fatalf("create failed: %v", err)
	}
	if _, err := s.claim(id); err != nil {
		t.Fatalf("first claim failed: %v", err)
	}
	if _, err := s.claim(id); err == nil {
		t.Fatal("second claim should fail (single-use)")
	}
}

func TestPendingStoreExpiry(t *testing.T) {
	s := newPendingStore(-time.Second)
	id, _, err := s.create(pendingPayload{Kind: "send"})
	if err != nil {
		t.Fatal(err)
	}
	// Force expiry: negative TTL is normalized to 5m, so expire manually.
	s.mu.Lock()
	e := s.items[id]
	e.expiresAt = time.Now().Add(-time.Second)
	s.items[id] = e
	s.mu.Unlock()
	if _, err := s.claim(id); err == nil {
		t.Fatal("expired claim should fail")
	}
}

func TestPendingStoreInvalidID(t *testing.T) {
	s := newPendingStore(time.Minute)
	for _, id := range []string{"", "a b", "x\r\ny"} {
		if _, err := s.claim(id); err == nil {
			t.Fatalf("id %q should be rejected", id)
		}
	}
}
