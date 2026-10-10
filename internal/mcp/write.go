package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	maildomain "github.com/spapas/workmail-mcp/internal/mail"
)

// Write tool inputs. Every write starts here and only freezes a validated
// payload; nothing is executed until mail_confirm is called with the
// returned pending ID.
type sendRequestInput struct {
	To         []string `json:"to,omitempty" jsonschema:"recipient email addresses"`
	Cc         []string `json:"cc,omitempty" jsonschema:"cc recipient email addresses"`
	Bcc        []string `json:"bcc,omitempty" jsonschema:"bcc recipient email addresses"`
	Subject    string   `json:"subject,omitempty" jsonschema:"email subject"`
	BodyText   string   `json:"body_text,omitempty" jsonschema:"plain-text body"`
	BodyHTML   string   `json:"body_html,omitempty" jsonschema:"HTML body"`
	InReplyTo  string   `json:"in_reply_to,omitempty" jsonschema:"In-Reply-To message ID"`
	References []string `json:"references,omitempty" jsonschema:"References message IDs"`
}

type moveRequestInput struct {
	Folder string `json:"folder,omitempty" jsonschema:"IMAP folder; defaults to configured folder"`
	UID    uint32 `json:"uid" jsonschema:"IMAP UID of the message"`
	Dest   string `json:"dest" jsonschema:"destination folder (must already exist)"`
}

type deleteRequestInput struct {
	Folder string `json:"folder,omitempty" jsonschema:"IMAP folder; defaults to configured folder"`
	UID    uint32 `json:"uid" jsonschema:"IMAP UID of the message to permanently delete"`
}

type setFlagsRequestInput struct {
	Folder string   `json:"folder,omitempty" jsonschema:"IMAP folder; defaults to configured folder"`
	UID    uint32   `json:"uid" jsonschema:"IMAP UID of the message"`
	Add    []string `json:"add,omitempty" jsonschema:"flags to add: Seen, Flagged, Answered, Draft"`
	Remove []string `json:"remove,omitempty" jsonschema:"flags to remove: Seen, Flagged, Answered, Draft"`
}

type confirmInput struct {
	PendingID string `json:"pending_id" jsonschema:"pending operation ID returned by a *_request tool"`
}

// pendingOutput confirms that an operation was frozen and awaits mail_confirm.
// Bodies are never echoed back.
type pendingOutput struct {
	PendingID string `json:"pending_id"`
	ExpiresAt string `json:"expires_at"`
	Kind      string `json:"kind"`
	Summary   string `json:"summary"`
}

// confirmOutput is the result of an executed (confirmed) operation.
type confirmOutput struct {
	Kind    string                   `json:"kind"`
	Summary string                   `json:"summary"`
	Send    *maildomain.SendReceipt  `json:"send,omitempty"`
	Move    *maildomain.MoveResult   `json:"move,omitempty"`
	Delete  *maildomain.DeleteResult `json:"delete,omitempty"`
	Flags   *maildomain.FlagsResult  `json:"flags,omitempty"`
}

func (t toolSet) sendRequest(_ context.Context, _ *mcpsdk.CallToolRequest, in sendRequestInput) (*mcpsdk.CallToolResult, pendingOutput, error) {
	start := time.Now()
	req := maildomain.SendRequest{
		To: in.To, Cc: in.Cc, Bcc: in.Bcc,
		Subject: in.Subject, BodyText: in.BodyText, BodyHTML: in.BodyHTML,
		InReplyTo: in.InReplyTo, References: in.References,
	}
	if err := maildomain.ValidateSend(&req, t.cfg.MaxRecipients, t.cfg.MaxBodyBytes, 998); err != nil {
		t.audit("mail_send_request", start, err)
		return nil, pendingOutput{}, err
	}
	total := len(req.To) + len(req.Cc) + len(req.Bcc)
	summary := fmt.Sprintf("send to %d recipient(s), subject %q", total, truncateRunes(req.Subject, 120))
	id, expiresAt, err := t.pending.create(pendingPayload{Kind: maildomain.PendingKindSend, Summary: summary, Send: &req})
	if err != nil {
		t.audit("mail_send_request", start, err)
		return nil, pendingOutput{}, err
	}
	t.audit("mail_send_request", start, nil)
	return nil, pendingOutput{PendingID: id, ExpiresAt: expiresAt.UTC().Format(time.RFC3339), Kind: maildomain.PendingKindSend, Summary: summary}, nil
}

func (t toolSet) moveRequest(_ context.Context, _ *mcpsdk.CallToolRequest, in moveRequestInput) (*mcpsdk.CallToolResult, pendingOutput, error) {
	start := time.Now()
	folder, err := maildomain.NormalizeFolder(in.Folder, t.cfg.DefaultFolder)
	if err != nil {
		t.audit("mail_move_request", start, err)
		return nil, pendingOutput{}, err
	}
	if err := maildomain.ValidateUID(in.UID); err != nil {
		t.audit("mail_move_request", start, err)
		return nil, pendingOutput{}, err
	}
	dest := strings.TrimSpace(in.Dest)
	if dest == "" || len(dest) > 1024 || strings.ContainsAny(dest, "\x00\r\n") {
		err := fmt.Errorf("invalid destination folder")
		t.audit("mail_move_request", start, err)
		return nil, pendingOutput{}, err
	}
	if strings.EqualFold(folder, dest) {
		err := fmt.Errorf("source and destination folders are the same")
		t.audit("mail_move_request", start, err)
		return nil, pendingOutput{}, err
	}
	frozen := maildomain.MoveRequest{Folder: folder, UID: in.UID, Dest: dest}
	summary := fmt.Sprintf("move UID %d from %q to %q", frozen.UID, frozen.Folder, frozen.Dest)
	id, expiresAt, err := t.pending.create(pendingPayload{Kind: maildomain.PendingKindMove, Summary: summary, Move: &frozen})
	if err != nil {
		t.audit("mail_move_request", start, err)
		return nil, pendingOutput{}, err
	}
	t.audit("mail_move_request", start, nil)
	return nil, pendingOutput{PendingID: id, ExpiresAt: expiresAt.UTC().Format(time.RFC3339), Kind: maildomain.PendingKindMove, Summary: summary}, nil
}

func (t toolSet) deleteRequest(_ context.Context, _ *mcpsdk.CallToolRequest, in deleteRequestInput) (*mcpsdk.CallToolResult, pendingOutput, error) {
	start := time.Now()
	folder, err := maildomain.NormalizeFolder(in.Folder, t.cfg.DefaultFolder)
	if err != nil {
		t.audit("mail_delete_request", start, err)
		return nil, pendingOutput{}, err
	}
	if err := maildomain.ValidateUID(in.UID); err != nil {
		t.audit("mail_delete_request", start, err)
		return nil, pendingOutput{}, err
	}
	frozen := maildomain.DeleteRequest{Folder: folder, UID: in.UID}
	summary := fmt.Sprintf("permanently delete UID %d from %q", frozen.UID, frozen.Folder)
	id, expiresAt, err := t.pending.create(pendingPayload{Kind: maildomain.PendingKindDelete, Summary: summary, Delete: &frozen})
	if err != nil {
		t.audit("mail_delete_request", start, err)
		return nil, pendingOutput{}, err
	}
	t.audit("mail_delete_request", start, nil)
	return nil, pendingOutput{PendingID: id, ExpiresAt: expiresAt.UTC().Format(time.RFC3339), Kind: maildomain.PendingKindDelete, Summary: summary}, nil
}

func (t toolSet) setFlagsRequest(_ context.Context, _ *mcpsdk.CallToolRequest, in setFlagsRequestInput) (*mcpsdk.CallToolResult, pendingOutput, error) {
	start := time.Now()
	folder, err := maildomain.NormalizeFolder(in.Folder, t.cfg.DefaultFolder)
	if err != nil {
		t.audit("mail_set_flags_request", start, err)
		return nil, pendingOutput{}, err
	}
	if err := maildomain.ValidateUID(in.UID); err != nil {
		t.audit("mail_set_flags_request", start, err)
		return nil, pendingOutput{}, err
	}
	add, remove, err := maildomain.NormalizeFlags(in.Add, in.Remove)
	if err != nil {
		t.audit("mail_set_flags_request", start, err)
		return nil, pendingOutput{}, err
	}
	frozen := maildomain.FlagsRequest{Folder: folder, UID: in.UID, Add: add, Remove: remove}
	summary := fmt.Sprintf("update flags on UID %d in %q (add %v, remove %v)", frozen.UID, frozen.Folder, add, remove)
	id, expiresAt, err := t.pending.create(pendingPayload{Kind: maildomain.PendingKindSetFlag, Summary: summary, Flags: &frozen})
	if err != nil {
		t.audit("mail_set_flags_request", start, err)
		return nil, pendingOutput{}, err
	}
	t.audit("mail_set_flags_request", start, nil)
	return nil, pendingOutput{PendingID: id, ExpiresAt: expiresAt.UTC().Format(time.RFC3339), Kind: maildomain.PendingKindSetFlag, Summary: summary}, nil
}

// confirm executes a previously frozen operation by pending ID. Payloads are
// single-use and expire; there is no direct write path that skips this step.
func (t toolSet) confirm(ctx context.Context, _ *mcpsdk.CallToolRequest, in confirmInput) (*mcpsdk.CallToolResult, confirmOutput, error) {
	start := time.Now()
	if t.writer == nil {
		err := fmt.Errorf("write service is not configured")
		t.audit("mail_confirm", start, err)
		return nil, confirmOutput{}, err
	}
	payload, err := t.pending.claim(in.PendingID)
	if err != nil {
		t.audit("mail_confirm", start, err)
		return nil, confirmOutput{}, err
	}
	switch payload.Kind {
	case maildomain.PendingKindSend:
		if payload.Send == nil {
			err := fmt.Errorf("pending send payload is missing")
			t.audit("mail_confirm", start, err)
			return nil, confirmOutput{}, err
		}
		receipt, err := t.writer.SendMessage(ctx, *payload.Send)
		t.audit("mail_confirm_send", start, err)
		if err != nil {
			return nil, confirmOutput{}, err
		}
		return nil, confirmOutput{Kind: payload.Kind, Summary: payload.Summary, Send: &receipt}, nil
	case maildomain.PendingKindMove:
		if payload.Move == nil {
			err := fmt.Errorf("pending move payload is missing")
			t.audit("mail_confirm", start, err)
			return nil, confirmOutput{}, err
		}
		res, err := t.writer.MoveMessage(ctx, *payload.Move)
		t.audit("mail_confirm_move", start, err)
		if err != nil {
			return nil, confirmOutput{}, err
		}
		return nil, confirmOutput{Kind: payload.Kind, Summary: payload.Summary, Move: &res}, nil
	case maildomain.PendingKindDelete:
		if payload.Delete == nil {
			err := fmt.Errorf("pending delete payload is missing")
			t.audit("mail_confirm", start, err)
			return nil, confirmOutput{}, err
		}
		res, err := t.writer.DeleteMessage(ctx, *payload.Delete)
		t.audit("mail_confirm_delete", start, err)
		if err != nil {
			return nil, confirmOutput{}, err
		}
		return nil, confirmOutput{Kind: payload.Kind, Summary: payload.Summary, Delete: &res}, nil
	case maildomain.PendingKindSetFlag:
		if payload.Flags == nil {
			err := fmt.Errorf("pending flags payload is missing")
			t.audit("mail_confirm", start, err)
			return nil, confirmOutput{}, err
		}
		res, err := t.writer.SetFlags(ctx, *payload.Flags)
		t.audit("mail_confirm_set_flags", start, err)
		if err != nil {
			return nil, confirmOutput{}, err
		}
		return nil, confirmOutput{Kind: payload.Kind, Summary: payload.Summary, Flags: &res}, nil
	default:
		err := fmt.Errorf("unknown pending operation kind")
		t.audit("mail_confirm", start, err)
		return nil, confirmOutput{}, err
	}
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "..."
	}
	return s
}
