// Package mail defines the transport-independent, read-only mailbox domain.
package mail

import (
	"context"
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

// Service is the complete read-only capability surface exposed to MCP.
type Service interface {
	ListFolders(context.Context) ([]Folder, error)
	Search(context.Context, SearchQuery) ([]MessageSummary, error)
	GetMessage(context.Context, string, uint32) (Message, error)
	GetAttachment(context.Context, string, uint32, int) (Attachment, error)
	GetThread(context.Context, string, uint32, int) ([]MessageSummary, error)
}
