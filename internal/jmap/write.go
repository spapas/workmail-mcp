package jmap

import (
	"context"
	"errors"
	"fmt"
	"strings"

	maildomain "github.com/spapas/workmail-mcp/internal/mail"
)

// SendMessage submits one email via JMAP EmailSubmission.
func (b *Backend) SendMessage(ctx context.Context, req maildomain.SendRequest) (maildomain.SendReceipt, error) {
	ctx, cancel := context.WithTimeout(ctx, b.cfg.OperationTimeout)
	defer cancel()
	if err := maildomain.ValidateSend(&req, b.cfg.MaxRecipients, b.cfg.MaxBodyBytes, 998); err != nil {
		return maildomain.SendReceipt{}, err
	}

	c, err := dial(ctx, b.cfg)
	if err != nil {
		return maildomain.SendReceipt{}, err
	}

	drafts, err := b.mailboxByRole(ctx, c, "drafts")
	if err != nil {
		return maildomain.SendReceipt{}, err
	}

	sent, err := b.mailboxByRole(ctx, c, "sent")
	if err != nil {
		return maildomain.SendReceipt{}, err
	}

	identityID, sender, err := b.resolveIdentity(ctx, c)
	if err != nil {
		return maildomain.SendReceipt{}, err
	}

	create := map[string]any{
		"mailboxIds": map[string]bool{drafts.ID: true},
		"from":       []any{map[string]string{"email": sender}},
		"subject":    req.Subject,
	}
	if len(req.To) > 0 {
		create["to"] = addrs(req.To)
	}

	if len(req.Cc) > 0 {
		create["cc"] = addrs(req.Cc)
	}

	if len(req.Bcc) > 0 {
		create["bcc"] = addrs(req.Bcc)
	}

	if strings.TrimSpace(req.BodyText) != "" {
		create["textBody"] = []any{map[string]string{"partId": "text", "type": "text/plain"}}
		create["bodyValues"] = map[string]any{"text": map[string]string{"value": req.BodyText}}
	}

	if strings.TrimSpace(req.BodyHTML) != "" {
		html := map[string]string{"partId": "html", "type": "text/html"}
		if _, ok := create["textBody"]; ok {
			create["htmlBody"] = []any{html}
		} else {
			create["textBody"] = []any{html}
		}
	}

	if strings.TrimSpace(req.InReplyTo) != "" {
		create["header:In-Reply-To"] = strings.TrimSpace(req.InReplyTo)
	}

	if len(req.References) > 0 {
		create["header:References"] = strings.Join(req.References, " ")
	}

	var created struct {
		Created map[string]struct {
			ID string `json:"id"`
		} `json:"created"`
		NotCreated map[string]any `json:"notCreated"`
	}
	args := map[string]any{"accountId": c.account, "create": map[string]any{"draft": create}}
	if err := c.first(ctx, "Email/set", args, &created); err != nil {
		return maildomain.SendReceipt{}, err
	}

	draft, ok := created.Created["draft"]
	if !ok || draft.ID == "" {
		return maildomain.SendReceipt{}, fmt.Errorf("jmap Email/set created nothing: %+v", created.NotCreated)
	}

	rcptTo := make([]any, 0, len(req.To)+len(req.Cc)+len(req.Bcc))
	for _, list := range [][]string{req.To, req.Cc, req.Bcc} {
		for _, raw := range list {
			rcptTo = append(rcptTo, map[string]string{"email": raw})
		}
	}

	envelope := map[string]any{
		"mailFrom": map[string]string{"email": sender},
		"rcptTo":   rcptTo,
	}
	if err := b.submit(ctx, c, draft.ID, identityID, envelope); err != nil {
		return maildomain.SendReceipt{}, err
	}

	// Record the sent draft in Sent. Submission delivers a separate copy;
	// the draft stays behind and is moved here only after a successful send.
	update := map[string]any{draft.ID: map[string]any{"mailboxIds": map[string]bool{sent.ID: true}}}
	var moved struct {
		Updated map[string]any `json:"updated"`
	}
	if err := c.first(ctx, "Email/set", map[string]any{"accountId": c.account, "update": update}, &moved); err != nil {
		return maildomain.SendReceipt{}, err
	}

	if _, ok := moved.Updated[draft.ID]; !ok {
		return maildomain.SendReceipt{}, errors.New("jmap sent update updated nothing")
	}

	msgID := b.createdMessageID(ctx, c, draft.ID)
	return maildomain.SendReceipt{
		ToCount:   len(req.To),
		CcCount:   len(req.Cc),
		BccCount:  len(req.Bcc),
		Subject:   req.Subject,
		MessageID: msgID,
	}, nil
}

// submit creates the EmailSubmission for a draft.
func (b *Backend) submit(ctx context.Context, c *Client, draftID, identityID string, envelope map[string]any) error {
	submission := map[string]any{
		"emailId":    draftID,
		"identityId": identityID,
		"envelope":   envelope,
	}
	var out struct {
		Created map[string]any `json:"created"`
	}
	args := map[string]any{"accountId": c.account, "create": map[string]any{"send": submission}}
	if err := c.firstWith(ctx, "EmailSubmission/set", args, submitUsing, &out); err != nil {
		return err
	}

	if len(out.Created) == 0 {
		return errors.New("jmap EmailSubmission/set created nothing")
	}

	return nil
}

// createdMessageID reads back the submitted message id header.
func (b *Backend) createdMessageID(ctx context.Context, c *Client, id string) string {
	var out struct {
		List []email `json:"list"`
	}
	args := map[string]any{"accountId": c.account, "ids": []string{id}, "properties": []string{"header:message-id"}}
	if err := c.first(ctx, "Email/get", args, &out); err != nil {
		return ""
	}

	if len(out.List) == 0 || len(out.List[0].MessageID) == 0 {
		return ""
	}

	return out.List[0].MessageID[0]
}

// resolveIdentity returns the submission identity id and sender address,
// preferring the identity matching the configured JMAP username.
func (b *Backend) resolveIdentity(ctx context.Context, c *Client) (string, string, error) {
	var out struct {
		List []struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"list"`
	}
	args := map[string]any{"accountId": c.account}
	if err := c.firstWith(ctx, "Identity/get", args, submitUsing, &out); err != nil {
		return "", "", err
	}

	if len(out.List) == 0 {
		return "", "", errors.New("jmap: no submission identities")
	}

	want := strings.ToLower(strings.TrimSpace(c.cfg.JMAPUsername))
	for _, ident := range out.List {
		if strings.ToLower(strings.TrimSpace(ident.Email)) == want {
			return ident.ID, ident.Email, nil
		}
	}

	return out.List[0].ID, out.List[0].Email, nil
}

// mailboxByRole resolves a mailbox by its JMAP role.
func (b *Backend) mailboxByRole(ctx context.Context, c *Client, role string) (mailbox, error) {
	boxes, err := b.mailboxes(ctx, c)
	if err != nil {
		return mailbox{}, err
	}

	for _, m := range boxes {
		if strings.EqualFold(m.Role, role) {
			return m, nil
		}
	}

	return mailbox{}, fmt.Errorf("jmap: no mailbox with role %q", role)
}

func addrs(list []string) []any {
	out := make([]any, 0, len(list))
	for _, raw := range list {
		out = append(out, map[string]string{"email": raw})
	}

	return out
}

// MoveMessage moves one message to another existing mailbox.
func (b *Backend) MoveMessage(ctx context.Context, req maildomain.MoveRequest) (maildomain.MoveResult, error) {
	ctx, cancel := context.WithTimeout(ctx, b.cfg.OperationTimeout)
	defer cancel()
	if req.Target.UID != 0 {
		return maildomain.MoveResult{}, errors.New("uid lookup requires the imap backend; use id")
	}

	dest := strings.TrimSpace(req.Dest)
	if dest == "" || len(dest) > 1024 || strings.ContainsAny(dest, "\x00\r\n") {
		return maildomain.MoveResult{}, fmt.Errorf("invalid destination folder")
	}

	c, err := dial(ctx, b.cfg)
	if err != nil {
		return maildomain.MoveResult{}, err
	}

	target, err := b.resolveFolder(ctx, c, dest)
	if err != nil {
		return maildomain.MoveResult{}, err
	}

	id := strings.TrimSpace(req.Target.ID)
	current, err := b.fetchOne(ctx, c, req.Target.Folder, id)
	if err != nil {
		return maildomain.MoveResult{}, err
	}

	if len(current.MailboxIDs) == 1 && current.MailboxIDs[target.ID] {
		return maildomain.MoveResult{}, fmt.Errorf("source and destination folders are the same")
	}

	update := map[string]any{id: map[string]any{"mailboxIds": map[string]bool{target.ID: true}}}
	var out struct {
		Updated map[string]any `json:"updated"`
	}
	args := map[string]any{"accountId": c.account, "update": update}
	if err := c.first(ctx, "Email/set", args, &out); err != nil {
		return maildomain.MoveResult{}, err
	}

	if _, ok := out.Updated[id]; !ok {
		return maildomain.MoveResult{}, fmt.Errorf("jmap move updated nothing")
	}

	return maildomain.MoveResult{Folder: req.Target.Folder, ID: id, Dest: dest, Moved: true}, nil
}

// DeleteMessage permanently destroys one message.
func (b *Backend) DeleteMessage(ctx context.Context, req maildomain.DeleteRequest) (maildomain.DeleteResult, error) {
	ctx, cancel := context.WithTimeout(ctx, b.cfg.OperationTimeout)
	defer cancel()
	if req.Target.UID != 0 {
		return maildomain.DeleteResult{}, errors.New("uid lookup requires the imap backend; use id")
	}

	id := strings.TrimSpace(req.Target.ID)
	if id == "" {
		return maildomain.DeleteResult{}, errors.New("id is required")
	}

	c, err := dial(ctx, b.cfg)
	if err != nil {
		return maildomain.DeleteResult{}, err
	}

	var out struct {
		Destroyed []string `json:"destroyed"`
	}
	args := map[string]any{"accountId": c.account, "destroy": []string{id}}
	if err := c.first(ctx, "Email/set", args, &out); err != nil {
		return maildomain.DeleteResult{}, err
	}

	if len(out.Destroyed) == 0 {
		return maildomain.DeleteResult{}, fmt.Errorf("jmap destroy destroyed nothing")
	}

	folder := strings.TrimSpace(req.Target.Folder)
	if folder == "" {
		folder = b.cfg.DefaultFolder
	}

	return maildomain.DeleteResult{Folder: folder, ID: id, Deleted: true}, nil
}

// SetFlags adds and/or removes keywords on one message.
func (b *Backend) SetFlags(ctx context.Context, req maildomain.FlagsRequest) (maildomain.FlagsResult, error) {
	ctx, cancel := context.WithTimeout(ctx, b.cfg.OperationTimeout)
	defer cancel()
	if req.Target.UID != 0 {
		return maildomain.FlagsResult{}, errors.New("uid lookup requires the imap backend; use id")
	}

	add, remove, err := maildomain.NormalizeFlags(req.Add, req.Remove)
	if err != nil {
		return maildomain.FlagsResult{}, err
	}

	id := strings.TrimSpace(req.Target.ID)
	if id == "" {
		return maildomain.FlagsResult{}, errors.New("id is required")
	}

	c, err := dial(ctx, b.cfg)
	if err != nil {
		return maildomain.FlagsResult{}, err
	}

	patch := map[string]any{}
	for _, flag := range add {
		keyword, ok := flagToKeyword(flag)
		if !ok {
			return maildomain.FlagsResult{}, fmt.Errorf("flag %q cannot be applied over jmap", flag)
		}

		patch["keywords/"+keyword] = true
	}

	for _, flag := range remove {
		keyword, ok := flagToKeyword(flag)
		if !ok {
			return maildomain.FlagsResult{}, fmt.Errorf("flag %q cannot be applied over jmap", flag)
		}

		patch["keywords/"+keyword] = nil
	}

	var out struct {
		Updated map[string]any `json:"updated"`
	}
	args := map[string]any{"accountId": c.account, "update": map[string]any{id: patch}}
	if err := c.first(ctx, "Email/set", args, &out); err != nil {
		return maildomain.FlagsResult{}, err
	}

	if _, ok := out.Updated[id]; !ok {
		return maildomain.FlagsResult{}, fmt.Errorf("jmap flags updated nothing")
	}

	folder := strings.TrimSpace(req.Target.Folder)
	if folder == "" {
		folder = b.cfg.DefaultFolder
	}

	return maildomain.FlagsResult{Folder: folder, ID: id, Added: add, Removed: remove}, nil
}
