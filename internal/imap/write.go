// Package imap implements the mailbox service using IMAPS.
//
// Write operations in this file always run on a ReadWrite-selected folder and
// are only reachable through the two-phase confirm flow in the MCP layer.
package imap

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	imapv2 "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	maildomain "github.com/spapas/workmail-mcp/internal/mail"
)

var _ maildomain.WriteService = (*Backend)(nil)

// SendMessage builds a bounded MIME message and delivers it over verified SMTP.
func (b *Backend) SendMessage(ctx context.Context, req maildomain.SendRequest) (maildomain.SendReceipt, error) {
	ctx, cancel := context.WithTimeout(ctx, b.cfg.OperationTimeout)
	defer cancel()
	if err := b.cfg.ValidateSMTP(); err != nil {
		return maildomain.SendReceipt{}, err
	}
	if err := maildomain.ValidateSend(&req, b.cfg.MaxRecipients, b.cfg.MaxBodyBytes, 998); err != nil {
		return maildomain.SendReceipt{}, err
	}
	raw, msgID, err := buildSendBytes(b.cfg.SMTPFrom, req)
	if err != nil {
		return maildomain.SendReceipt{}, err
	}
	if err := sendSMTP(ctx, b.cfg.SMTPHost, b.cfg.SMTPPort, b.cfg.SMTPUsername, b.cfg.SMTPPassword, b.cfg.SMTPFrom, b.cfg.OperationTimeout, raw, req); err != nil {
		return maildomain.SendReceipt{}, err
	}
	return maildomain.SendReceipt{
		ToCount:   len(req.To),
		CcCount:   len(req.Cc),
		BccCount:  len(req.Bcc),
		Subject:   req.Subject,
		MessageID: msgID,
	}, nil
}

// MoveMessage moves one message to another existing folder.
func (b *Backend) MoveMessage(ctx context.Context, req maildomain.MoveRequest) (maildomain.MoveResult, error) {
	ctx, cancel := context.WithTimeout(ctx, b.cfg.OperationTimeout)
	defer cancel()
	folder, err := b.normalizeFolder(req.Folder)
	if err != nil {
		return maildomain.MoveResult{}, err
	}
	dest := strings.TrimSpace(req.Dest)
	if dest == "" || len(dest) > 1024 || strings.ContainsAny(dest, "\x00\r\n") {
		return maildomain.MoveResult{}, fmt.Errorf("invalid destination folder")
	}
	if strings.EqualFold(folder, dest) {
		return maildomain.MoveResult{}, fmt.Errorf("source and destination folders are the same")
	}
	if err := maildomain.ValidateUID(req.UID); err != nil {
		return maildomain.MoveResult{}, err
	}
	c, cleanup, err := b.connectSelectedWrite(ctx, folder)
	if err != nil {
		return maildomain.MoveResult{}, err
	}
	defer cleanup()

	set := imapv2.UIDSetNum(imapv2.UID(req.UID))
	if supports(c, imapv2.CapMove) {
		if _, err := c.Move(set, dest).Wait(); err != nil {
			return maildomain.MoveResult{}, fmt.Errorf("move message: %w", err)
		}
		return maildomain.MoveResult{Folder: folder, UID: req.UID, Dest: dest, Moved: true}, nil
	}
	if _, err := c.Copy(set, dest).Wait(); err != nil {
		return maildomain.MoveResult{}, fmt.Errorf("copy for move: %w", err)
	}
	if err := storeAdd(c, set, []imapv2.Flag{imapv2.FlagDeleted}); err != nil {
		return maildomain.MoveResult{}, err
	}
	if err := expungeUID(c, set); err != nil {
		return maildomain.MoveResult{}, err
	}
	return maildomain.MoveResult{Folder: folder, UID: req.UID, Dest: dest, Moved: true}, nil
}

// DeleteMessage permanently deletes one message via STORE \Deleted + expunge.
func (b *Backend) DeleteMessage(ctx context.Context, req maildomain.DeleteRequest) (maildomain.DeleteResult, error) {
	ctx, cancel := context.WithTimeout(ctx, b.cfg.OperationTimeout)
	defer cancel()
	folder, err := b.normalizeFolder(req.Folder)
	if err != nil {
		return maildomain.DeleteResult{}, err
	}
	if err := maildomain.ValidateUID(req.UID); err != nil {
		return maildomain.DeleteResult{}, err
	}
	c, cleanup, err := b.connectSelectedWrite(ctx, folder)
	if err != nil {
		return maildomain.DeleteResult{}, err
	}
	defer cleanup()

	set := imapv2.UIDSetNum(imapv2.UID(req.UID))
	if err := storeAdd(c, set, []imapv2.Flag{imapv2.FlagDeleted}); err != nil {
		return maildomain.DeleteResult{}, err
	}
	if err := expungeUID(c, set); err != nil {
		return maildomain.DeleteResult{}, err
	}
	return maildomain.DeleteResult{Folder: folder, UID: req.UID, Deleted: true}, nil
}

// SetFlags adds and/or removes allowlisted flags on one message.
func (b *Backend) SetFlags(ctx context.Context, req maildomain.FlagsRequest) (maildomain.FlagsResult, error) {
	ctx, cancel := context.WithTimeout(ctx, b.cfg.OperationTimeout)
	defer cancel()
	folder, err := b.normalizeFolder(req.Folder)
	if err != nil {
		return maildomain.FlagsResult{}, err
	}
	if err := maildomain.ValidateUID(req.UID); err != nil {
		return maildomain.FlagsResult{}, err
	}
	add, remove, err := maildomain.NormalizeFlags(req.Add, req.Remove)
	if err != nil {
		return maildomain.FlagsResult{}, err
	}
	c, cleanup, err := b.connectSelectedWrite(ctx, folder)
	if err != nil {
		return maildomain.FlagsResult{}, err
	}
	defer cleanup()

	set := imapv2.UIDSetNum(imapv2.UID(req.UID))
	if len(add) > 0 {
		if err := storeAdd(c, set, toIMAPFlags(add)); err != nil {
			return maildomain.FlagsResult{}, err
		}
	}
	if len(remove) > 0 {
		cmd := c.Store(set, &imapv2.StoreFlags{Op: imapv2.StoreFlagsDel, Silent: true, Flags: toIMAPFlags(remove)}, nil)
		if err := cmd.Close(); err != nil {
			return maildomain.FlagsResult{}, fmt.Errorf("remove flags: %w", err)
		}
	}
	return maildomain.FlagsResult{Folder: folder, UID: req.UID, Added: add, Removed: remove}, nil
}

func (b *Backend) connectSelectedWrite(ctx context.Context, folder string) (*imapclient.Client, func(), error) {
	c, cleanup, err := b.connect(ctx)
	if err != nil {
		return nil, nil, err
	}
	if _, err := c.Select(folder, &imapv2.SelectOptions{ReadOnly: false}).Wait(); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("select folder %q read-write: %w", folder, err)
	}
	return c, cleanup, nil
}

func storeAdd(c *imapclient.Client, set imapv2.UIDSet, flags []imapv2.Flag) error {
	cmd := c.Store(set, &imapv2.StoreFlags{Op: imapv2.StoreFlagsAdd, Silent: true, Flags: flags}, nil)
	if err := cmd.Close(); err != nil {
		return fmt.Errorf("store flags: %w", err)
	}
	return nil
}

func expungeUID(c *imapclient.Client, set imapv2.UIDSet) error {
	if supports(c, imapv2.CapUIDPlus) {
		if err := c.UIDExpunge(set).Close(); err == nil {
			return nil
		}
	}
	if err := c.Expunge().Close(); err != nil {
		return fmt.Errorf("expunge: %w", err)
	}
	return nil
}

func supports(c *imapclient.Client, cap imapv2.Cap) bool {
	caps, err := c.Capability().Wait()
	if err != nil {
		return false
	}
	return caps.Has(cap)
}

func toIMAPFlags(in []string) []imapv2.Flag {
	out := make([]imapv2.Flag, 0, len(in))
	for _, f := range in {
		out = append(out, imapv2.Flag(f))
	}
	return out
}

// buildSendBytes renders a bounded RFC 5322 message. Bodies are trusted here
// only as transport bytes: callers and clients must still treat mailbox
// content as untrusted data on the way back in.
func buildSendBytes(from string, req maildomain.SendRequest) ([]byte, string, error) {
	fromAddr, err := mail.ParseAddress(from)
	if err != nil {
		return nil, "", fmt.Errorf("invalid from address: %w", err)
	}
	to, err := maildomain.ParseAddresses(req.To)
	if err != nil {
		return nil, "", err
	}
	cc, err := maildomain.ParseAddresses(req.Cc)
	if err != nil {
		return nil, "", err
	}
	bcc, err := maildomain.ParseAddresses(req.Bcc)
	if err != nil {
		return nil, "", err
	}
	msgID := newMessageID(fromAddr.Address)
	var sb strings.Builder
	sb.WriteString("From: " + fromAddr.String() + "\r\n")
	if len(to) > 0 {
		sb.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	}
	if len(cc) > 0 {
		sb.WriteString("Cc: " + strings.Join(cc, ", ") + "\r\n")
	}
	sb.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", req.Subject) + "\r\n")
	sb.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	sb.WriteString("Message-ID: " + msgID + "\r\n")
	if strings.TrimSpace(req.InReplyTo) != "" {
		sb.WriteString("In-Reply-To: " + strings.TrimSpace(req.InReplyTo) + "\r\n")
	}
	if len(req.References) > 0 {
		sb.WriteString("References: " + strings.Join(req.References, " ") + "\r\n")
	}
	sb.WriteString("MIME-Version: 1.0\r\n")
	hasText := strings.TrimSpace(req.BodyText) != ""
	hasHTML := strings.TrimSpace(req.BodyHTML) != ""
	if hasText && hasHTML {
		boundary := newBoundary()
		sb.WriteString("Content-Type: multipart/alternative; boundary=" + boundary + "\r\n\r\n")
		sb.WriteString("--" + boundary + "\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n" + req.BodyText + "\r\n")
		sb.WriteString("--" + boundary + "\r\nContent-Type: text/html; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n" + req.BodyHTML + "\r\n")
		sb.WriteString("--" + boundary + "--\r\n")
	} else if hasHTML {
		sb.WriteString("Content-Type: text/html; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n" + req.BodyHTML + "\r\n")
	} else {
		sb.WriteString("Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n" + req.BodyText + "\r\n")
	}
	_ = bcc // BCC recipients receive the message via envelope only, never via headers.
	return []byte(sb.String()), msgID, nil
}

func newMessageID(domain string) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	host := domain
	if i := strings.LastIndex(host, "@"); i >= 0 && i+1 < len(host) {
		host = host[i+1:]
	}
	host = strings.TrimSpace(host)
	if host == "" {
		host = "workmail-mcp"
	}
	return "<" + hex.EncodeToString(b[:]) + "@" + host + ">"
}

func newBoundary() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return "workmail" + hex.EncodeToString(b[:])
}

// sendSMTP delivers raw bytes over TLS-verified SMTP (implicit TLS on 465,
// otherwise STARTTLS when offered). Authentication uses PLAIN when available.
func sendSMTP(ctx context.Context, host string, port int, username, password, from string, timeout time.Duration, raw []byte, req maildomain.SendRequest) error {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	dialer := &net.Dialer{Timeout: timeout}
	var conn net.Conn
	var err error
	if port == 465 {
		tlsCfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
		tlsDialer := &tls.Dialer{NetDialer: dialer, Config: tlsCfg}
		conn, err = tlsDialer.DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("connect SMTP %s: %w", addr, err)
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("SMTP handshake: %w", err)
	}
	defer func() { _ = client.Quit() }()
	if port != 465 {
		if ok, _ := client.Extension("STARTTLS"); ok {
			tlsCfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
			if err := client.StartTLS(tlsCfg); err != nil {
				return fmt.Errorf("SMTP STARTTLS: %w", err)
			}
		}
	}
	// Authenticate only when the server offers AUTH (production Zimbra always
	// does). Test SMTP listeners without AUTH still accept local delivery, and
	// a relay-denying server fails later at RCPT with a clear error.
	if username != "" {
		if ok, _ := client.Extension("AUTH"); ok {
			auth := smtp.PlainAuth("", username, password, host)
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("SMTP auth: %w", err)
			}
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("SMTP MAIL FROM: %w", err)
	}
	for _, rcpt := range append(append(append([]string{}, req.To...), req.Cc...), req.Bcc...) {
		parsed, err := mail.ParseAddress(rcpt)
		if err != nil {
			return fmt.Errorf("invalid recipient: %w", err)
		}
		if err := client.Rcpt(parsed.Address); err != nil {
			return fmt.Errorf("SMTP RCPT: %w", err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		_ = w.Close()
		return fmt.Errorf("SMTP write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("SMTP data close: %w", err)
	}
	return nil
}
