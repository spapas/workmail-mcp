// Package jmap implements the mailbox service over JMAP (RFC 8620/8621).
package jmap

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/spapas/workmail-mcp/internal/config"
	maildomain "github.com/spapas/workmail-mcp/internal/mail"
)

// Capabilities for read-only JMAP calls.
var usingCaps = []string{"urn:ietf:params:jmap:core", "urn:ietf:params:jmap:mail"}

// submitUsing extends the read capabilities with submission for
// Identity/get and EmailSubmission/set.
var submitUsing = []string{
	"urn:ietf:params:jmap:core",
	"urn:ietf:params:jmap:mail",
	"urn:ietf:params:jmap:submission",
}

// Client is a minimal stdlib-only JMAP client using Basic auth.
type Client struct {
	cfg     config.Config
	http    *http.Client
	apiURL  string
	account string
}

// New creates a JMAP backend bound to the configured account.
func New(cfg config.Config) *Backend {
	return &Backend{cfg: cfg}
}

// session discovers the JMAP session and resolves the mail account.
func (c *Client) session(ctx context.Context) error {
	base := strings.TrimSuffix(strings.TrimSpace(c.cfg.JMAPURL), "/")
	base = strings.TrimSuffix(base, "/jmap")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/.well-known/jmap", nil)
	if err != nil {
		return fmt.Errorf("jmap session request: %w", err)
	}

	c.auth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("jmap session: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jmap session: status %d", resp.StatusCode)
	}

	var session struct {
		APIURL          string            `json:"apiUrl"`
		PrimaryAccounts map[string]string `json:"primaryAccounts"`
		Username        string            `json:"username"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&session); err != nil {
		return fmt.Errorf("jmap session decode: %w", err)
	}

	if session.APIURL == "" {
		return fmt.Errorf("jmap session: missing apiUrl")
	}

	c.apiURL = rewriteAPIHost(strings.TrimSpace(c.cfg.JMAPURL), session.APIURL)
	account := strings.TrimSpace(c.cfg.JMAPAccount)
	if account == "" {
		account = session.PrimaryAccounts["urn:ietf:params:jmap:mail"]
	}

	if account == "" {
		return fmt.Errorf("jmap session: no mail account")
	}

	c.account = account
	return nil
}

// rewriteAPIHost keeps talking to the configured server: scheme and host
// come from configuration, only the advertised path is trusted. Servers
// behind other names would otherwise redirect the password elsewhere.
func rewriteAPIHost(configured, advertised string) string {
	adv, err := url.Parse(advertised)
	if err != nil || adv.Path == "" {
		return advertised
	}

	base, err := url.Parse(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(configured), "/"), "/jmap"))
	if err != nil || base.Host == "" {
		return advertised
	}

	adv.Scheme = base.Scheme
	adv.Host = base.Host
	return adv.String()
}

// auth sets Basic credentials on a JMAP request.
func (c *Client) auth(req *http.Request) {
	creds := c.cfg.JMAPUsername + ":" + c.cfg.JMAPPassword
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(creds)))
}

// response is one JMAP method response: [name, payload, callID].
type response struct {
	Name    string
	Payload json.RawMessage
}

// call posts one batch of method calls and returns the responses.
func (c *Client) call(ctx context.Context, calls [][]any, using []string) ([]response, error) {
	body, err := json.Marshal(map[string]any{"using": using, "methodCalls": calls})
	if err != nil {
		return nil, fmt.Errorf("jmap encode: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("jmap request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	c.auth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jmap call: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jmap call: status %d", resp.StatusCode)
	}

	var envelope struct {
		Responses []json.RawMessage `json:"methodResponses"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("jmap decode: %w", err)
	}

	out := make([]response, 0, len(envelope.Responses))
	for _, raw := range envelope.Responses {
		var parts []json.RawMessage
		if err := json.Unmarshal(raw, &parts); err != nil || len(parts) != 3 {
			return nil, fmt.Errorf("jmap malformed response")
		}

		var name string
		if err := json.Unmarshal(parts[0], &name); err != nil {
			return nil, fmt.Errorf("jmap malformed response name")
		}

		if name == "error" {
			var payload struct {
				Type        string `json:"type"`
				Description string `json:"description"`
			}
			_ = json.Unmarshal(parts[1], &payload)
			if payload.Description != "" {
				return nil, fmt.Errorf("jmap %s: %s", payload.Type, payload.Description)
			}

			return nil, fmt.Errorf("jmap %s", payload.Type)
		}

		out = append(out, response{Name: name, Payload: parts[1]})
	}

	return out, nil
}

// first calls one method and decodes its payload.
func (c *Client) first(ctx context.Context, name string, args map[string]any, out any) error {
	return c.firstWith(ctx, name, args, usingCaps, out)
}

// firstWith calls one method with explicit capabilities and decodes its payload.
func (c *Client) firstWith(ctx context.Context, name string, args map[string]any, using []string, out any) error {
	id := "c1"
	resps, err := c.call(ctx, [][]any{{name, args, id}}, using)
	if err != nil {
		return err
	}

	if len(resps) != 1 || resps[0].Name != name {
		return fmt.Errorf("jmap unexpected response for %s", name)
	}

	if err := json.Unmarshal(resps[0].Payload, out); err != nil {
		return fmt.Errorf("jmap decode %s: %w", name, err)
	}

	return nil
}

// dial opens an authenticated session bound to the mail account.
func dial(ctx context.Context, cfg config.Config) (*Client, error) {
	ctx, cancel := context.WithTimeout(ctx, cfg.OperationTimeout)
	defer cancel()
	c := &Client{cfg: cfg, http: &http.Client{Timeout: cfg.OperationTimeout}}
	if err := c.session(ctx); err != nil {
		return nil, err
	}

	return c, nil
}

// keywordToFlag maps JMAP keywords to IMAP flag names.
func keywordToFlag(keyword string) (string, bool) {
	switch keyword {
	case "$seen":
		return `\Seen`, true
	case "$flagged":
		return `\Flagged`, true
	case "$answered":
		return `\Answered`, true
	case "$draft":
		return `\Draft`, true
	case "$deleted":
		return `\Deleted`, true
	}

	return "", false
}

// flagToKeyword maps canonical IMAP flag names to JMAP keywords.
func flagToKeyword(flag string) (string, bool) {
	for _, allowed := range maildomain.AllowedFlags {
		if flag == allowed {
			return "$" + strings.ToLower(strings.TrimPrefix(allowed, `\`)), true
		}
	}

	return "", false
}

// cutStr returns the longest rune-aligned prefix within n bytes.
func cutStr(s string, n int64) string {
	if int64(len(s)) <= n {
		return s
	}

	cut := int(n)
	for cut > 0 && !utf8.ValidString(s[:cut]) {
		cut--
	}

	return s[:cut]
}

// utcDate formats a time as a JMAP UTCDate.
func utcDate(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}
