package mcp

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"time"

	maildomain "github.com/spapas/workmail-mcp/internal/mail"
)

// pendingPayload is a frozen, already-validated write operation. The confirm
// step executes exactly this payload: clients cannot alter parameters at
// confirm time, they can only approve or abandon the pending ID.
type pendingPayload struct {
	Kind    string
	Summary string
	Send    *maildomain.SendRequest
	Move    *maildomain.MoveRequest
	Delete  *maildomain.DeleteRequest
	Flags   *maildomain.FlagsRequest
}

type pendingEntry struct {
	payload   pendingPayload
	expiresAt time.Time
}

// pendingStore holds unconfirmed write operations. Entries are single-use and
// expire after a bounded TTL; expired or unknown IDs must be re-requested.
type pendingStore struct {
	mu    sync.Mutex
	items map[string]pendingEntry
	ttl   time.Duration
}

// newPendingStore creates a store with the given TTL (5 minutes when non-positive).
func newPendingStore(ttl time.Duration) *pendingStore {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &pendingStore{items: map[string]pendingEntry{}, ttl: ttl}
}

// create freezes a validated payload and returns its single-use ID and expiry.
func (s *pendingStore) create(payload pendingPayload) (id string, expiresAt time.Time, err error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", time.Time{}, errors.New("generate confirmation id: " + err.Error())
	}
	id = base64.RawURLEncoding.EncodeToString(b[:])
	expiresAt = time.Now().Add(s.ttl)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, e := range s.items {
		if !e.expiresAt.After(now) {
			delete(s.items, k)
		}
	}
	s.items[id] = pendingEntry{payload: payload, expiresAt: expiresAt}
	return id, expiresAt, nil
}

// claim returns and deletes the frozen payload for id. The second claim of the
// same ID, an unknown ID, or an expired ID all fail and require a fresh request.
func (s *pendingStore) claim(id string) (pendingPayload, error) {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 128 || strings.ContainsAny(id, "\x00\r\n ") {
		return pendingPayload{}, errors.New("invalid pending id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.items[id]
	if !ok {
		return pendingPayload{}, errors.New("unknown or already used confirmation id; re-request the operation")
	}
	delete(s.items, id)
	if !entry.expiresAt.After(time.Now()) {
		return pendingPayload{}, errors.New("confirmation expired; re-request the operation")
	}
	return entry.payload, nil
}
