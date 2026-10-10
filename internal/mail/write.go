// Package mail defines the transport-independent mailbox domain.
//
// Write operations are always two-phase: a request step validates and freezes
// the exact operation, and a confirm step executes the frozen payload by ID.
// There is intentionally no direct (single-call) write path.
package mail

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
)

// Pending operation kinds. One kind per write capability.
const (
	PendingKindSend    = "send"
	PendingKindMove    = "move"
	PendingKindDelete  = "delete"
	PendingKindSetFlag = "set_flags"
)

// AllowedFlags is the bounded set of IMAP flags mutable through the write tools.
// \Deleted is intentionally excluded: deletion has its own confirmed flow.
var AllowedFlags = []string{"\\Seen", "\\Flagged", "\\Answered", "\\Draft"}

// SendRequest is a bounded request to send one email message.
type SendRequest struct {
	To         []string
	Cc         []string
	Bcc        []string
	Subject    string
	BodyText   string
	BodyHTML   string
	InReplyTo  string
	References []string
}

// SendReceipt is the result of an executed send.
type SendReceipt struct {
	ToCount   int    `json:"to_count"`
	CcCount   int    `json:"cc_count"`
	BccCount  int    `json:"bcc_count"`
	Subject   string `json:"subject,omitempty"`
	MessageID string `json:"message_id,omitempty"`
}

// MoveRequest moves one message to another existing folder.
type MoveRequest struct {
	Folder string
	UID    uint32
	Dest   string
}

// MoveResult is the result of an executed move.
type MoveResult struct {
	Folder string `json:"folder"`
	UID    uint32 `json:"uid"`
	Dest   string `json:"dest"`
	Moved  bool   `json:"moved"`
}

// DeleteRequest permanently deletes one message (STORE \Deleted + expunge).
type DeleteRequest struct {
	Folder string
	UID    uint32
}

// DeleteResult is the result of an executed delete.
type DeleteResult struct {
	Folder  string `json:"folder"`
	UID     uint32 `json:"uid"`
	Deleted bool   `json:"deleted"`
}

// FlagsRequest adds and/or removes IMAP flags on one message.
type FlagsRequest struct {
	Folder string
	UID    uint32
	Add    []string
	Remove []string
}

// FlagsResult is the result of an executed flag mutation.
type FlagsResult struct {
	Folder  string   `json:"folder"`
	UID     uint32   `json:"uid"`
	Added   []string `json:"added,omitempty"`
	Removed []string `json:"removed,omitempty"`
}

// WriteService is the write capability surface. Every method executes an
// already-confirmed (frozen) payload; validation happens before the pending
// operation is created, and implementations must re-validate defensively.
type WriteService interface {
	SendMessage(ctx context.Context, req SendRequest) (SendReceipt, error)
	MoveMessage(ctx context.Context, req MoveRequest) (MoveResult, error)
	DeleteMessage(ctx context.Context, req DeleteRequest) (DeleteResult, error)
	SetFlags(ctx context.Context, req FlagsRequest) (FlagsResult, error)
}

// NormalizeFolder trims and validates a folder name, falling back to def when empty.
func NormalizeFolder(folder, def string) (string, error) {
	folder = strings.TrimSpace(folder)
	if folder == "" {
		folder = def
	}
	if folder == "" || len(folder) > 1024 || strings.ContainsAny(folder, "\x00\r\n") {
		return "", errors.New("invalid folder name")
	}
	return folder, nil
}

// ValidateUID requires a non-zero IMAP UID.
func ValidateUID(uid uint32) error {
	if uid == 0 {
		return errors.New("uid must be greater than zero")
	}
	return nil
}

// ParseAddresses parses and validates a recipient list. Empty entries are
// dropped; each remaining entry must be a valid RFC 5322 address.
func ParseAddresses(list []string) ([]string, error) {
	out := make([]string, 0, len(list))
	for _, raw := range list {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		addr, err := mail.ParseAddress(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid recipient %q: %w", raw, err)
		}
		out = append(out, addr.String())
	}
	return out, nil
}

// ValidateSend checks a send request against bounded limits. maxRecipients
// caps To+Cc+Bcc, maxBodyBytes caps BodyText+BodyHTML, maxSubjectRunes caps subject.
func ValidateSend(req *SendRequest, maxRecipients int, maxBodyBytes int64, maxSubjectRunes int) error {
	if len(req.To)+len(req.Cc)+len(req.Bcc) == 0 {
		return errors.New("at least one recipient in to, cc, or bcc is required")
	}
	if len(req.To)+len(req.Cc)+len(req.Bcc) > maxRecipients {
		return fmt.Errorf("recipient count exceeds limit %d", maxRecipients)
	}
	for name, list := range map[string][]string{"to": req.To, "cc": req.Cc, "bcc": req.Bcc} {
		for _, raw := range list {
			if len(raw) > 1024 {
				return fmt.Errorf("%s recipient exceeds 1024 characters", name)
			}
			if strings.ContainsAny(raw, "\x00\r\n") {
				return fmt.Errorf("%s recipient contains invalid control characters", name)
			}
		}
	}
	if _, err := ParseAddresses(append(append(append([]string{}, req.To...), req.Cc...), req.Bcc...)); err != nil {
		return err
	}
	if strings.ContainsAny(req.Subject, "\x00\r\n") {
		return errors.New("subject contains invalid control characters")
	}
	if len([]rune(req.Subject)) > maxSubjectRunes {
		return fmt.Errorf("subject exceeds %d characters", maxSubjectRunes)
	}
	if int64(len(req.BodyText)+len(req.BodyHTML)) > maxBodyBytes {
		return fmt.Errorf("body exceeds limit %d", maxBodyBytes)
	}
	if strings.TrimSpace(req.BodyText) == "" && strings.TrimSpace(req.BodyHTML) == "" {
		return errors.New("body_text or body_html is required")
	}
	return nil
}

// NormalizeFlags validates flag names against the allowlist and returns them
// in canonical "\Seen" form, deduplicated with Add/Remove conflicts rejected.
func NormalizeFlags(add, remove []string) (normAdd, normRemove []string, err error) {
	norm := func(list []string) ([]string, error) {
		out := []string{}
		seen := map[string]bool{}
		for _, raw := range list {
			raw = strings.TrimSpace(raw)
			if raw == "" {
				continue
			}
			canon := "\\" + strings.TrimPrefix(strings.TrimSpace(raw), "\\")
			matched := false
			for _, allowed := range AllowedFlags {
				if strings.EqualFold(canon, allowed) {
					canon = allowed
					matched = true
					break
				}
			}
			if !matched {
				return nil, fmt.Errorf("flag %q is not allowed", raw)
			}
			if !seen[canon] {
				seen[canon] = true
				out = append(out, canon)
			}
		}
		return out, nil
	}
	normAdd, err = norm(add)
	if err != nil {
		return nil, nil, err
	}
	normRemove, err = norm(remove)
	if err != nil {
		return nil, nil, err
	}
	for _, a := range normAdd {
		for _, r := range normRemove {
			if strings.EqualFold(a, r) {
				return nil, nil, fmt.Errorf("flag %q cannot be both added and removed", a)
			}
		}
	}
	if len(normAdd) == 0 && len(normRemove) == 0 {
		return nil, nil, errors.New("at least one flag in add or remove is required")
	}
	return normAdd, normRemove, nil
}
