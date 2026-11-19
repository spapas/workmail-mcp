package jmap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spapas/workmail-mcp/internal/config"
	maildomain "github.com/spapas/workmail-mcp/internal/mail"
)

// Backend is a JMAP mailbox backend.
type Backend struct {
	cfg config.Config
}

var _ maildomain.Service = (*Backend)(nil)
var _ maildomain.WriteService = (*Backend)(nil)

// mailbox is the subset of JMAP Mailbox used here.
type mailbox struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	Total  uint32 `json:"totalEmails"`
	Unseen uint32 `json:"unreadEmails"`
}

// emailAddr maps JMAP EmailAddress to the domain type.
type emailAddr struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// headerList decodes a JMAP header field given either as one string or
// as an array of strings. Servers differ here; accept both.
type headerList []string

// UnmarshalJSON implements json.Unmarshaler.
func (h *headerList) UnmarshalJSON(raw []byte) error {
	var multi []string
	if err := json.Unmarshal(raw, &multi); err == nil {
		*h = multi
		return nil
	}

	var single string
	if err := json.Unmarshal(raw, &single); err != nil {
		return err
	}

	*h = []string{single}
	return nil
}

// email is the subset of JMAP Email used here.
type email struct {
	ID          string             `json:"id"`
	ThreadID    string             `json:"threadId"`
	MailboxIDs  map[string]bool    `json:"mailboxIds"`
	Keywords    map[string]bool    `json:"keywords"`
	Size        int64              `json:"size"`
	ReceivedAt  time.Time          `json:"receivedAt"`
	SentAt      time.Time          `json:"sentAt"`
	From        []emailAddr        `json:"from"`
	To          []emailAddr        `json:"to"`
	Cc          []emailAddr        `json:"cc"`
	Subject     string             `json:"subject"`
	MessageID   headerList         `json:"header:message-id"`
	InReplyTo   headerList         `json:"header:in-reply-to"`
	References  headerList         `json:"header:references"`
	TextBody    []bodyPart         `json:"textBody"`
	HTMLBody    []bodyPart         `json:"htmlBody"`
	BodyValues  map[string]bodyVal `json:"bodyValues"`
	Attachments []attachment       `json:"attachments"`
}

type bodyPart struct {
	PartID string `json:"partId"`
}

type bodyVal struct {
	Value       string `json:"value"`
	IsTruncated bool   `json:"isTruncated"`
}

type attachment struct {
	BlobID string `json:"blobId"`
	Type   string `json:"type"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
}

// metaProps are Email/get properties for metadata-only fetches.
var metaProps = []string{
	"id", "threadId", "mailboxIds", "keywords", "size",
	"receivedAt", "sentAt", "from", "to", "cc", "subject",
	"header:message-id", "header:in-reply-to",
}

// Doctor verifies JMAP session and mailbox listing.
func (b *Backend) Doctor(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, b.cfg.OperationTimeout)
	defer cancel()
	c, err := dial(ctx, b.cfg)
	if err != nil {
		return err
	}

	_, err = b.mailboxes(ctx, c)
	return err
}

// mailboxes returns all visible mailboxes sorted by name.
func (b *Backend) mailboxes(ctx context.Context, c *Client) ([]mailbox, error) {
	var out struct {
		List []mailbox `json:"list"`
	}
	args := map[string]any{"accountId": c.account, "ids": nil}
	if err := c.first(ctx, "Mailbox/get", args, &out); err != nil {
		return nil, err
	}

	sort.Slice(out.List, func(i, j int) bool {
		return strings.ToLower(out.List[i].Name) < strings.ToLower(out.List[j].Name)
	})

	return out.List, nil
}

// resolveFolder maps a folder name to a mailbox id (case-insensitive).
func (b *Backend) resolveFolder(ctx context.Context, c *Client, folder string) (mailbox, error) {
	folder = strings.TrimSpace(folder)
	if folder == "" {
		folder = b.cfg.DefaultFolder
	}

	boxes, err := b.mailboxes(ctx, c)
	if err != nil {
		return mailbox{}, err
	}

	for _, m := range boxes {
		if strings.EqualFold(m.Name, folder) {
			return m, nil
		}
	}

	return mailbox{}, fmt.Errorf("unknown folder %q", folder)
}

// ListFolders returns all mailboxes visible to the configured account.
func (b *Backend) ListFolders(ctx context.Context) ([]maildomain.Folder, error) {
	ctx, cancel := context.WithTimeout(ctx, b.cfg.OperationTimeout)
	defer cancel()
	c, err := dial(ctx, b.cfg)
	if err != nil {
		return nil, err
	}

	boxes, err := b.mailboxes(ctx, c)
	if err != nil {
		return nil, err
	}

	folders := make([]maildomain.Folder, 0, len(boxes))
	for _, m := range boxes {
		folders = append(folders, maildomain.Folder{Name: m.Name})
	}

	return folders, nil
}

// Stats returns total and unseen counts for one mailbox.
func (b *Backend) Stats(ctx context.Context, folder string) (maildomain.FolderStats, error) {
	ctx, cancel := context.WithTimeout(ctx, b.cfg.OperationTimeout)
	defer cancel()
	c, err := dial(ctx, b.cfg)
	if err != nil {
		return maildomain.FolderStats{}, err
	}

	m, err := b.resolveFolder(ctx, c, folder)
	if err != nil {
		return maildomain.FolderStats{}, err
	}

	return maildomain.FolderStats{Folder: m.Name, Total: m.Total, Unseen: m.Unseen}, nil
}

// Search executes a structured JMAP Email/query and returns bounded metadata.
func (b *Backend) Search(ctx context.Context, q maildomain.SearchQuery) ([]maildomain.MessageSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, b.cfg.OperationTimeout)
	defer cancel()
	c, err := dial(ctx, b.cfg)
	if err != nil {
		return nil, err
	}

	m, limit, err := b.normalizeSearch(ctx, c, &q)
	if err != nil {
		return nil, err
	}

	filter := map[string]any{"inMailbox": m.ID}
	if q.Text != "" {
		filter["text"] = q.Text
	}

	if q.From != "" {
		filter["from"] = q.From
	}

	if q.To != "" {
		filter["to"] = q.To
	}

	if q.Subject != "" {
		filter["subject"] = q.Subject
	}

	if !q.Since.IsZero() {
		filter["after"] = utcDate(q.Since)
	}

	if !q.Before.IsZero() {
		filter["before"] = utcDate(q.Before)
	}

	var query struct {
		IDs []string `json:"ids"`
	}
	args := map[string]any{
		"accountId": c.account,
		"filter":    filter,
		"sort":      []any{map[string]any{"property": "receivedAt", "isAscending": false}},
		"limit":     limit,
	}
	if err := c.first(ctx, "Email/query", args, &query); err != nil {
		return nil, err
	}

	return b.fetchSummaries(ctx, c, m.Name, query.IDs)
}

// fetchSummaries returns bounded metadata for email ids.
func (b *Backend) fetchSummaries(ctx context.Context, c *Client, folder string, ids []string) ([]maildomain.MessageSummary, error) {
	if len(ids) == 0 {
		return []maildomain.MessageSummary{}, nil
	}

	var out struct {
		List []email `json:"list"`
	}
	args := map[string]any{"accountId": c.account, "ids": ids, "properties": metaProps}
	if err := c.first(ctx, "Email/get", args, &out); err != nil {
		return nil, err
	}

	summaries := make([]maildomain.MessageSummary, 0, len(out.List))
	for i := range out.List {
		summaries = append(summaries, summaryFromEmail(folder, &out.List[i]))
	}

	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Date.After(summaries[j].Date) })
	return summaries, nil
}

// fetchOne returns one email with full body and attachment metadata.
func (b *Backend) fetchOne(ctx context.Context, c *Client, folder, id string) (email, error) {
	props := append(append([]string{}, metaProps...),
		"header:references", "textBody", "htmlBody", "bodyValues", "attachments")
	var out struct {
		List []email `json:"list"`
	}
	args := map[string]any{
		"accountId":           c.account,
		"ids":                 []string{id},
		"properties":          props,
		"fetchTextBodyValues": true,
		"fetchHTMLBodyValues": true,
		"fetchAllBodyValues":  false,
		"maxBodyValueBytes":   b.cfg.MaxBodyBytes,
	}
	if err := c.first(ctx, "Email/get", args, &out); err != nil {
		return email{}, err
	}

	if len(out.List) == 0 {
		return email{}, fmt.Errorf("message id %q not found", id)
	}

	return out.List[0], nil
}

// GetMessage fetches one email with bounded text/HTML body and attachment metadata.
func (b *Backend) GetMessage(ctx context.Context, ref maildomain.Ref) (maildomain.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, b.cfg.OperationTimeout)
	defer cancel()
	if ref.UID != 0 {
		return maildomain.Message{}, errors.New("uid lookup requires the imap backend; use id")
	}

	folder := strings.TrimSpace(ref.Folder)
	if folder == "" {
		folder = b.cfg.DefaultFolder
	}

	c, err := dial(ctx, b.cfg)
	if err != nil {
		return maildomain.Message{}, err
	}

	summary, raw, err := b.fetchRaw(ctx, c, folder, strings.TrimSpace(ref.ID))
	if err != nil {
		return maildomain.Message{}, err
	}

	parsed, err := parseBodies(raw, b.cfg.MaxBodyBytes, b.cfg.MaxAttachmentBytes)
	if err != nil {
		return maildomain.Message{}, err
	}

	return maildomain.Message{
		MessageSummary: summary,
		BodyText:       parsed.Text,
		BodyHTML:       parsed.HTML,
		References:     parsed.References,
		Attachments:    parsed.AttachmentMetadata(),
		Truncated:      parsed.Truncated,
	}, nil
}

// GetAttachment returns one attachment by zero-based index as base64.
func (b *Backend) GetAttachment(ctx context.Context, ref maildomain.Ref, index int) (maildomain.Attachment, error) {
	ctx, cancel := context.WithTimeout(ctx, b.cfg.OperationTimeout)
	defer cancel()
	if ref.UID != 0 {
		return maildomain.Attachment{}, errors.New("uid lookup requires the imap backend; use id")
	}

	if index < 0 {
		return maildomain.Attachment{}, errors.New("attachment index must be non-negative")
	}

	c, err := dial(ctx, b.cfg)
	if err != nil {
		return maildomain.Attachment{}, err
	}

	id := strings.TrimSpace(ref.ID)
	msg, err := b.fetchOne(ctx, c, ref.Folder, id)
	if err != nil {
		return maildomain.Attachment{}, err
	}

	if index >= len(msg.Attachments) {
		return maildomain.Attachment{}, fmt.Errorf("attachment index %d out of range; message has %d attachment(s)", index, len(msg.Attachments))
	}

	att := msg.Attachments[index]
	if att.Size > b.cfg.MaxAttachmentBytes {
		return maildomain.Attachment{}, fmt.Errorf("attachment is %d bytes, exceeding limit %d", att.Size, b.cfg.MaxAttachmentBytes)
	}

	var out struct {
		List []struct {
			ID   string `json:"id"`
			Data string `json:"data"`
			Size int64  `json:"size"`
		} `json:"list"`
	}
	args := map[string]any{"accountId": c.account, "ids": []string{att.BlobID}}
	if err := c.firstWith(ctx, "Blob/get", args, append(usingCaps, "urn:ietf:params:jmap:blob"), &out); err != nil {
		return maildomain.Attachment{}, err
	}

	if len(out.List) == 0 {
		return maildomain.Attachment{}, fmt.Errorf("blob %q not found", att.BlobID)
	}

	meta := maildomain.AttachmentMeta{
		Index: index, Filename: att.Name, ContentType: att.Type, Size: att.Size, ID: att.BlobID,
	}
	return maildomain.Attachment{AttachmentMeta: meta, DataBase64: out.List[0].Data}, nil
}

// GetThread reconstructs a bounded thread via JMAP Thread/get.
func (b *Backend) GetThread(ctx context.Context, ref maildomain.Ref, limit int) ([]maildomain.MessageSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, b.cfg.OperationTimeout)
	defer cancel()
	if ref.UID != 0 {
		return nil, errors.New("uid lookup requires the imap backend; use id")
	}

	if limit == 0 {
		limit = b.cfg.MaxThreadMessages
	}

	if limit < 1 || limit > b.cfg.MaxThreadMessages {
		return nil, fmt.Errorf("thread limit must be between 1 and %d", b.cfg.MaxThreadMessages)
	}

	c, err := dial(ctx, b.cfg)
	if err != nil {
		return nil, err
	}

	id := strings.TrimSpace(ref.ID)
	msg, err := b.fetchOne(ctx, c, ref.Folder, id)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(msg.ThreadID) == "" {
		folder := strings.TrimSpace(ref.Folder)
		if folder == "" {
			folder = b.cfg.DefaultFolder
		}

		return []maildomain.MessageSummary{summaryFromEmail(folder, &msg)}, nil
	}

	var thread struct {
		List []struct {
			EmailIDs []string `json:"emailIds"`
		} `json:"list"`
	}
	args := map[string]any{"accountId": c.account, "ids": []string{msg.ThreadID}}
	if err := c.first(ctx, "Thread/get", args, &thread); err != nil {
		return nil, err
	}

	if len(thread.List) == 0 {
		return nil, fmt.Errorf("thread %q not found", msg.ThreadID)
	}

	ids := thread.List[0].EmailIDs
	if len(ids) > limit {
		ids = ids[:limit]
	}

	folder := strings.TrimSpace(ref.Folder)
	if folder == "" {
		folder = b.cfg.DefaultFolder
	}

	return b.fetchSummaries(ctx, c, folder, ids)
}

// fetchRaw checks the server-reported size before fetching bodies.
func (b *Backend) fetchRaw(ctx context.Context, c *Client, folder, id string) (maildomain.MessageSummary, email, error) {
	if id == "" {
		return maildomain.MessageSummary{}, email{}, errors.New("id is required")
	}

	if folder == "" {
		folder = b.cfg.DefaultFolder
	}

	msg, err := b.fetchOne(ctx, c, folder, id)
	if err != nil {
		return maildomain.MessageSummary{}, email{}, err
	}

	if msg.Size > b.cfg.MaxMessageBytes {
		return maildomain.MessageSummary{}, email{}, fmt.Errorf("message is %d bytes, exceeding limit %d", msg.Size, b.cfg.MaxMessageBytes)
	}

	return summaryFromEmail(folder, &msg), msg, nil
}

// normalizeSearch validates a search query and resolves the mailbox.
func (b *Backend) normalizeSearch(ctx context.Context, c *Client, q *maildomain.SearchQuery) (mailbox, int, error) {
	m, err := b.resolveFolder(ctx, c, q.Folder)
	if err != nil {
		return mailbox{}, 0, err
	}

	limit := q.Limit
	if limit == 0 {
		limit = b.cfg.MaxResults
	}

	if limit < 1 || limit > b.cfg.MaxResults {
		return mailbox{}, 0, fmt.Errorf("limit must be between 1 and %d", b.cfg.MaxResults)
	}

	for name, value := range map[string]string{"text": q.Text, "from": q.From, "to": q.To, "subject": q.Subject} {
		if len(value) > b.cfg.MaxQueryLength {
			return mailbox{}, 0, fmt.Errorf("%s query exceeds %d characters", name, b.cfg.MaxQueryLength)
		}

		if strings.ContainsAny(value, "\x00\r\n") {
			return mailbox{}, 0, fmt.Errorf("%s query contains invalid control characters", name)
		}
	}

	if !q.Since.IsZero() && !q.Before.IsZero() && !q.Since.Before(q.Before) {
		return mailbox{}, 0, fmt.Errorf("since must be before before")
	}

	return m, limit, nil
}

func summaryFromEmail(folder string, msg *email) maildomain.MessageSummary {
	out := maildomain.MessageSummary{ID: msg.ID, Folder: folder, Date: msg.ReceivedAt, Size: msg.Size}
	if out.Date.IsZero() {
		out.Date = msg.SentAt
	}

	out.Subject = msg.Subject
	out.From = convertAddrs(msg.From)
	out.To = convertAddrs(msg.To)
	out.Cc = convertAddrs(msg.Cc)
	if len(msg.MessageID) > 0 {
		out.MessageID = msg.MessageID[0]
	}

	out.InReplyTo = append([]string(nil), msg.InReplyTo...)
	for keyword := range msg.Keywords {
		if flag, ok := keywordToFlag(keyword); ok {
			out.Flags = append(out.Flags, flag)
		}
	}

	sort.Strings(out.Flags)
	return out
}

func convertAddrs(in []emailAddr) []maildomain.Address {
	out := make([]maildomain.Address, 0, len(in))
	for _, a := range in {
		if strings.TrimSpace(a.Email) == "" {
			continue
		}

		out = append(out, maildomain.Address{Name: a.Name, Email: a.Email})
	}

	return out
}
