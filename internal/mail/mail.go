// Package mail defines the transport-independent, read-only mailbox domain.
package mail

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Address is an RFC 5322 mailbox address.
type Address struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
}

// Folder describes an IMAP mailbox folder.
type Folder struct {
	Name       string   `json:"name"`
	Delimiter  string   `json:"delimiter,omitempty"`
	Attributes []string `json:"attributes,omitempty"`
}

// AttachmentMeta describes an attachment without returning its content.
type AttachmentMeta struct {
	Index       int    `json:"index"`
	Filename    string `json:"filename,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Size        int64  `json:"size"`
	TooLarge    bool   `json:"too_large,omitempty"`
	// ID holds the server-issued blob identity (JMAP Blob id).
	// IMAP-only metadata leaves it empty and uses Index instead.
	ID string `json:"id,omitempty"`
}

// MessageSummary contains bounded metadata for one message.
type MessageSummary struct {
	UID       uint32    `json:"uid"`
	Folder    string    `json:"folder"`
	Date      time.Time `json:"date"`
	Subject   string    `json:"subject,omitempty"`
	From      []Address `json:"from,omitempty"`
	To        []Address `json:"to,omitempty"`
	Cc        []Address `json:"cc,omitempty"`
	MessageID string    `json:"message_id,omitempty"`
	InReplyTo []string  `json:"in_reply_to,omitempty"`
	Size      int64     `json:"size"`
	// ID holds the server-issued string identity (JMAP Email id).
	// IMAP-only summaries leave it empty and use UID instead.
	ID string `json:"id,omitempty"`
	// Flags holds the IMAP flags reported for the message (for example
	// \Seen, \Flagged, \Answered, \Draft, \Deleted), as returned by FETCH FLAGS.
	Flags []string `json:"flags,omitempty"`
}

// Message contains message metadata plus bounded text/HTML bodies and attachment metadata.
type Message struct {
	MessageSummary
	BodyText    string           `json:"body_text,omitempty"`
	BodyHTML    string           `json:"body_html,omitempty"`
	References  []string         `json:"references,omitempty"`
	Attachments []AttachmentMeta `json:"attachments,omitempty"`
	Truncated   bool             `json:"truncated,omitempty"`
}

// Attachment contains one attachment encoded for safe JSON/MCP transport.
type Attachment struct {
	AttachmentMeta
	DataBase64 string `json:"data_base64"`
}

// FolderStats contains total and unseen message counts for one folder.
// Counts come from IMAP STATUS and never include message content.
type FolderStats struct {
	Folder string `json:"folder"`
	Total  uint32 `json:"total"`
	Unseen uint32 `json:"unseen"`
}

// SearchQuery is a bounded, structured mailbox search.
type SearchQuery struct {
	Folder  string
	Text    string
	From    string
	To      string
	Subject string
	Since   time.Time
	Before  time.Time
	Limit   int
}

// Ref identifies one message by backend-native identity: IMAP UID
// or JMAP string id. Exactly one of UID, ID must be set.
type Ref struct {
	Folder string
	UID    uint32
	ID     string
}

// NormalizeRef trims and validates a message reference, falling back
// to def when folder is empty. UID and ID are mutually exclusive.
func NormalizeRef(folder, def string, uid uint32, id string) (Ref, error) {
	folder = strings.TrimSpace(folder)
	if folder == "" {
		folder = def
	}

	if folder == "" || len(folder) > 1024 || strings.ContainsAny(folder, "\x00\r\n") {
		return Ref{}, errors.New("invalid folder name")
	}

	id = strings.TrimSpace(id)
	if uid != 0 && id != "" {
		return Ref{}, errors.New("uid and id are mutually exclusive")
	}

	if uid == 0 && id == "" {
		return Ref{}, errors.New("uid or id is required")
	}

	if id != "" {
		if len(id) > 1024 || strings.ContainsAny(id, "\x00\r\n") {
			return Ref{}, errors.New("invalid message id")
		}
	}

	return Ref{Folder: folder, UID: uid, ID: id}, nil
}

// Service is the complete read-only capability surface exposed to MCP.
type Service interface {
	ListFolders(context.Context) ([]Folder, error)
	Search(context.Context, SearchQuery) ([]MessageSummary, error)
	GetMessage(context.Context, Ref) (Message, error)
	GetAttachment(context.Context, Ref, int) (Attachment, error)
	GetThread(context.Context, Ref, int) ([]MessageSummary, error)
	// Stats returns total and unseen message counts for one folder.
	Stats(context.Context, string) (FolderStats, error)
}
