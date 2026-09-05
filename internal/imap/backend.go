// Package imap implements the read-only mailbox service using IMAPS.
package imap

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net"
	"sort"
	"strings"
	"sync"

	imapv2 "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/charset"

	"github.com/spapas/workmail-mcp/internal/config"
	maildomain "github.com/spapas/workmail-mcp/internal/mail"
)

// Backend is a live, read-only IMAP mailbox backend.
type Backend struct {
	cfg config.Config
}

// New creates a read-only IMAP backend.
func New(cfg config.Config) *Backend {
	return &Backend{cfg: cfg}
}

// Doctor verifies TLS, authentication, and read-only mailbox listing.
func (b *Backend) Doctor(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, b.cfg.OperationTimeout)
	defer cancel()
	c, cleanup, err := b.connect(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	_, err = c.List("", "*", nil).Collect()
	if err != nil {
		return fmt.Errorf("list folders: %w", err)
	}
	return nil
}

// ListFolders returns all folders visible to the configured account.
func (b *Backend) ListFolders(ctx context.Context) ([]maildomain.Folder, error) {
	ctx, cancel := context.WithTimeout(ctx, b.cfg.OperationTimeout)
	defer cancel()
	c, cleanup, err := b.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	items, err := c.List("", "*", nil).Collect()
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	folders := make([]maildomain.Folder, 0, len(items))
	for _, item := range items {
		attrs := make([]string, 0, len(item.Attrs))
		for _, attr := range item.Attrs {
			attrs = append(attrs, string(attr))
		}
		delim := ""
		if item.Delim != 0 {
			delim = string(item.Delim)
		}
		folders = append(folders, maildomain.Folder{Name: item.Mailbox, Delimiter: delim, Attributes: attrs})
	}
	sort.Slice(folders, func(i, j int) bool { return strings.ToLower(folders[i].Name) < strings.ToLower(folders[j].Name) })
	return folders, nil
}

// Search executes a structured IMAP search and returns bounded metadata.
func (b *Backend) Search(ctx context.Context, q maildomain.SearchQuery) ([]maildomain.MessageSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, b.cfg.OperationTimeout)
	defer cancel()
	folder, limit, err := b.normalizeQuery(&q)
	if err != nil {
		return nil, err
	}
	c, cleanup, err := b.connectSelected(ctx, folder)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	criteria := &imapv2.SearchCriteria{Since: q.Since, Before: q.Before}
	if q.Text != "" {
		criteria.Text = []string{q.Text}
	}
	for _, hv := range []struct{ key, value string }{{"From", q.From}, {"To", q.To}, {"Subject", q.Subject}} {
		if hv.value != "" {
			criteria.Header = append(criteria.Header, imapv2.SearchCriteriaHeaderField{Key: hv.key, Value: hv.value})
		}
	}
	data, err := c.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("search %q: %w", folder, err)
	}
	uids := data.AllUIDs()
	if len(uids) > limit {
		uids = uids[len(uids)-limit:]
	}
	return b.fetchSummaries(c, folder, uids)
}

// GetMessage fetches one whole RFC 822 message after a server-reported size check.
func (b *Backend) GetMessage(ctx context.Context, folder string, uid uint32) (maildomain.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, b.cfg.OperationTimeout)
	defer cancel()
	folder, err := b.normalizeFolder(folder)
	if err != nil {
		return maildomain.Message{}, err
	}
	c, cleanup, err := b.connectSelected(ctx, folder)
	if err != nil {
		return maildomain.Message{}, err
	}
	defer cleanup()

	summary, raw, err := b.fetchRaw(c, folder, imapv2.UID(uid))
	if err != nil {
		return maildomain.Message{}, err
	}
	parsed, err := maildomain.ParseRaw(raw, b.cfg.MaxBodyBytes, b.cfg.MaxAttachmentBytes)
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

// GetAttachment returns one attachment by zero-based attachment index.
func (b *Backend) GetAttachment(ctx context.Context, folder string, uid uint32, index int) (maildomain.Attachment, error) {
	ctx, cancel := context.WithTimeout(ctx, b.cfg.OperationTimeout)
	defer cancel()
	if index < 0 {
		return maildomain.Attachment{}, errors.New("attachment index must be non-negative")
	}
	folder, err := b.normalizeFolder(folder)
	if err != nil {
		return maildomain.Attachment{}, err
	}
	c, cleanup, err := b.connectSelected(ctx, folder)
	if err != nil {
		return maildomain.Attachment{}, err
	}
	defer cleanup()
	_, raw, err := b.fetchRaw(c, folder, imapv2.UID(uid))
	if err != nil {
		return maildomain.Attachment{}, err
	}
	parsed, err := maildomain.ParseRaw(raw, b.cfg.MaxBodyBytes, b.cfg.MaxAttachmentBytes)
	if err != nil {
		return maildomain.Attachment{}, err
	}
	if index >= len(parsed.Attachments) {
		return maildomain.Attachment{}, fmt.Errorf("attachment index %d out of range; message has %d attachment(s)", index, len(parsed.Attachments))
	}
	att := parsed.Attachments[index]
	if att.Meta.TooLarge {
		return maildomain.Attachment{}, fmt.Errorf("attachment is %d bytes, exceeding limit %d", att.Meta.Size, b.cfg.MaxAttachmentBytes)
	}
	return maildomain.Attachment{AttachmentMeta: att.Meta, DataBase64: base64.StdEncoding.EncodeToString(att.Data)}, nil
}

// GetThread reconstructs a bounded thread using Message-ID/In-Reply-To/References headers.
func (b *Backend) GetThread(ctx context.Context, folder string, uid uint32, limit int) ([]maildomain.MessageSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, b.cfg.OperationTimeout)
	defer cancel()
	folder, err := b.normalizeFolder(folder)
	if err != nil {
		return nil, err
	}
	if limit == 0 {
		limit = b.cfg.MaxThreadMessages
	}
	if limit < 1 || limit > b.cfg.MaxThreadMessages {
		return nil, fmt.Errorf("thread limit must be between 1 and %d", b.cfg.MaxThreadMessages)
	}
	c, cleanup, err := b.connectSelected(ctx, folder)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	initial, err := b.fetchSummaries(c, folder, []imapv2.UID{imapv2.UID(uid)})
	if err != nil || len(initial) == 0 {
		if err == nil {
			err = fmt.Errorf("message UID %d not found", uid)
		}
		return nil, err
	}
	result := []maildomain.MessageSummary{initial[0]}
	seenUID := map[uint32]bool{uid: true}
	seenID := map[string]bool{}
	queue := append([]string{}, initial[0].InReplyTo...)
	if initial[0].MessageID != "" {
		queue = append(queue, initial[0].MessageID)
	}

	for len(queue) > 0 && len(result) < limit {
		id := strings.TrimSpace(queue[0])
		queue = queue[1:]
		if id == "" || seenID[id] {
			continue
		}
		seenID[id] = true
		var candidates []imapv2.UID
		for _, key := range []string{"Message-ID", "In-Reply-To", "References"} {
			data, err := c.UIDSearch(&imapv2.SearchCriteria{Header: []imapv2.SearchCriteriaHeaderField{{Key: key, Value: id}}}, nil).Wait()
			if err != nil {
				return nil, fmt.Errorf("thread search %s: %w", key, err)
			}
			candidates = append(candidates, data.AllUIDs()...)
		}
		var newUIDs []imapv2.UID
		for _, candidate := range candidates {
			cu := uint32(candidate)
			if !seenUID[cu] {
				seenUID[cu] = true
				newUIDs = append(newUIDs, candidate)
				if len(result)+len(newUIDs) >= limit {
					break
				}
			}
		}
		if len(newUIDs) == 0 {
			continue
		}
		msgs, err := b.fetchSummaries(c, folder, newUIDs)
		if err != nil {
			return nil, err
		}
		for _, msg := range msgs {
			result = append(result, msg)
			queue = append(queue, msg.MessageID)
			queue = append(queue, msg.InReplyTo...)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Date.Before(result[j].Date) })
	return result, nil
}

func (b *Backend) connect(ctx context.Context) (*imapclient.Client, func(), error) {
	addr := net.JoinHostPort(b.cfg.IMAPHost, fmt.Sprintf("%d", b.cfg.IMAPPort))
	opts := &imapclient.Options{
		TLSConfig:   &tls.Config{ServerName: b.cfg.IMAPHost, MinVersion: tls.VersionTLS12},
		Dialer:      &net.Dialer{Timeout: b.cfg.OperationTimeout},
		WordDecoder: &mime.WordDecoder{CharsetReader: charset.Reader},
	}
	c, err := imapclient.DialTLS(addr, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("connect IMAPS %s: %w", addr, err)
	}
	var once sync.Once
	done := make(chan struct{})
	cleanup := func() {
		once.Do(func() {
			close(done)
			_ = c.Close()
		})
	}
	go func() {
		select {
		case <-ctx.Done():
			cleanup()
		case <-done:
		}
	}()
	if err := c.Login(b.cfg.IMAPUsername, b.cfg.IMAPPassword).Wait(); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("IMAP login: %w", err)
	}
	return c, cleanup, nil
}

func (b *Backend) connectSelected(ctx context.Context, folder string) (*imapclient.Client, func(), error) {
	c, cleanup, err := b.connect(ctx)
	if err != nil {
		return nil, nil, err
	}
	if _, err := c.Select(folder, &imapv2.SelectOptions{ReadOnly: true}).Wait(); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("select folder %q read-only: %w", folder, err)
	}
	return c, cleanup, nil
}

func (b *Backend) fetchSummaries(c *imapclient.Client, folder string, uids []imapv2.UID) ([]maildomain.MessageSummary, error) {
	if len(uids) == 0 {
		return []maildomain.MessageSummary{}, nil
	}
	items, err := c.Fetch(imapv2.UIDSetNum(uids...), &imapv2.FetchOptions{UID: true, Envelope: true, InternalDate: true, RFC822Size: true}).Collect()
	if err != nil {
		return nil, fmt.Errorf("fetch summaries: %w", err)
	}
	out := make([]maildomain.MessageSummary, 0, len(items))
	for _, item := range items {
		out = append(out, summaryFromBuffer(folder, item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.After(out[j].Date) })
	return out, nil
}

func (b *Backend) fetchRaw(c *imapclient.Client, folder string, uid imapv2.UID) (maildomain.MessageSummary, []byte, error) {
	summaries, err := b.fetchSummaries(c, folder, []imapv2.UID{uid})
	if err != nil {
		return maildomain.MessageSummary{}, nil, err
	}
	if len(summaries) == 0 {
		return maildomain.MessageSummary{}, nil, fmt.Errorf("message UID %d not found", uid)
	}
	summary := summaries[0]
	if summary.Size > b.cfg.MaxMessageBytes {
		return maildomain.MessageSummary{}, nil, fmt.Errorf("message is %d bytes, exceeding limit %d", summary.Size, b.cfg.MaxMessageBytes)
	}
	section := &imapv2.FetchItemBodySection{Peek: true}
	items, err := c.Fetch(imapv2.UIDSetNum(uid), &imapv2.FetchOptions{UID: true, BodySection: []*imapv2.FetchItemBodySection{section}}).Collect()
	if err != nil {
		return maildomain.MessageSummary{}, nil, fmt.Errorf("fetch message body: %w", err)
	}
	if len(items) == 0 {
		return maildomain.MessageSummary{}, nil, fmt.Errorf("message UID %d disappeared during fetch", uid)
	}
	raw := items[0].FindBodySection(section)
	if raw == nil {
		return maildomain.MessageSummary{}, nil, fmt.Errorf("server returned no body for UID %d", uid)
	}
	if int64(len(raw)) > b.cfg.MaxMessageBytes {
		return maildomain.MessageSummary{}, nil, fmt.Errorf("received message exceeds limit %d", b.cfg.MaxMessageBytes)
	}
	return summary, raw, nil
}

func summaryFromBuffer(folder string, item *imapclient.FetchMessageBuffer) maildomain.MessageSummary {
	out := maildomain.MessageSummary{UID: uint32(item.UID), Folder: folder, Date: item.InternalDate, Size: item.RFC822Size}
	if env := item.Envelope; env != nil {
		out.Subject = env.Subject
		out.From = convertAddresses(env.From)
		out.To = convertAddresses(env.To)
		out.Cc = convertAddresses(env.Cc)
		out.MessageID = env.MessageID
		out.InReplyTo = append([]string(nil), env.InReplyTo...)
		if !env.Date.IsZero() {
			out.Date = env.Date
		}
	}
	return out
}

func convertAddresses(in []imapv2.Address) []maildomain.Address {
	out := make([]maildomain.Address, 0, len(in))
	for i := range in {
		email := in[i].Addr()
		if email == "" {
			continue
		}
		out = append(out, maildomain.Address{Name: in[i].Name, Email: email})
	}
	return out
}

func (b *Backend) normalizeQuery(q *maildomain.SearchQuery) (string, int, error) {
	folder, err := b.normalizeFolder(q.Folder)
	if err != nil {
		return "", 0, err
	}
	limit := q.Limit
	if limit == 0 {
		limit = b.cfg.MaxResults
	}
	if limit < 1 || limit > b.cfg.MaxResults {
		return "", 0, fmt.Errorf("limit must be between 1 and %d", b.cfg.MaxResults)
	}
	for name, value := range map[string]string{"text": q.Text, "from": q.From, "to": q.To, "subject": q.Subject} {
		if len(value) > b.cfg.MaxQueryLength {
			return "", 0, fmt.Errorf("%s query exceeds %d characters", name, b.cfg.MaxQueryLength)
		}
		if strings.ContainsAny(value, "\x00\r\n") {
			return "", 0, fmt.Errorf("%s query contains invalid control characters", name)
		}
	}
	if !q.Since.IsZero() && !q.Before.IsZero() && !q.Since.Before(q.Before) {
		return "", 0, errors.New("since must be before before")
	}
	return folder, limit, nil
}

func (b *Backend) normalizeFolder(folder string) (string, error) {
	folder = strings.TrimSpace(folder)
	if folder == "" {
		folder = b.cfg.DefaultFolder
	}
	if folder == "" || len(folder) > 1024 || strings.ContainsAny(folder, "\x00\r\n") {
		return "", errors.New("invalid folder name")
	}
	return folder, nil
}

var _ maildomain.Service = (*Backend)(nil)
