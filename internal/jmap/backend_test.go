package jmap

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spapas/workmail-mcp/internal/config"
	maildomain "github.com/spapas/workmail-mcp/internal/mail"
)

// fakeJMAP answers JMAP calls with canned Stalwart-flavored data.
type fakeJMAP struct {
	t *testing.T
}

func (f *fakeJMAP) session(w http.ResponseWriter, r *http.Request) {
	if user, _, ok := r.BasicAuth(); !ok || user == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	json.NewEncoder(w).Encode(map[string]any{
		"capabilities":    map[string]any{"urn:ietf:params:jmap:core": map[string]any{}},
		"accounts":        map[string]any{"a1": map[string]any{"name": "user", "isPersonal": true}},
		"primaryAccounts": map[string]any{"urn:ietf:params:jmap:mail": "a1"},
		"username":        "user",
		"apiUrl":          "http://" + r.Host + "/jmap/",
	})
}

func (f *fakeJMAP) call(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Using       []string          `json:"using"`
		MethodCalls []json.RawMessage `json:"methodCalls"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	has := func(cap string) bool {
		for _, c := range req.Using {
			if c == cap {
				return true
			}
		}

		return false
	}

	var head []json.RawMessage
	if err := json.Unmarshal(req.MethodCalls[0], &head); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var name, callID string
	if err := json.Unmarshal(head[0], &name); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := json.Unmarshal(head[2], &callID); err != nil {
		callID = "c1"
	}

	respond := func(payload any) {
		json.NewEncoder(w).Encode(map[string]any{
			"methodResponses": []any{[]any{name, payload, callID}},
		})
	}
	fail := func(typ string) {
		json.NewEncoder(w).Encode(map[string]any{
			"methodResponses": []any{[]any{"error", map[string]any{"type": typ}, callID}},
		})
	}

	switch name {
	case "Mailbox/get":
		respond(map[string]any{"accountId": "a1", "list": []any{
			map[string]any{"id": "m1", "name": "INBOX", "role": "inbox", "totalEmails": 10, "unreadEmails": 2},
			map[string]any{"id": "m2", "name": "Archive", "role": "archive", "totalEmails": 3, "unreadEmails": 0},
			map[string]any{"id": "m3", "name": "Drafts", "role": "drafts", "totalEmails": 0, "unreadEmails": 0},
			map[string]any{"id": "m4", "name": "Sent", "role": "sent", "totalEmails": 0, "unreadEmails": 0},
		}})
	case "Email/query":
		respond(map[string]any{"accountId": "a1", "ids": []string{"e1"}})
	case "Email/get":
		respond(map[string]any{"accountId": "a1", "list": f.emails(head[1])})
	case "Thread/get":
		respond(map[string]any{"accountId": "a1", "list": []any{
			map[string]any{"id": "t1", "emailIds": []string{"e1", "e2"}},
		}})
	case "Blob/get":
		if !has("urn:ietf:params:jmap:blob") {
			fail("unknownMethod")
			return
		}

		respond(map[string]any{"accountId": "a1", "list": []any{
			map[string]any{"id": "b1", "data": base64.StdEncoding.EncodeToString([]byte("hello")), "size": 5},
		}})
	case "Identity/get":
		if !has("urn:ietf:params:jmap:submission") {
			fail("unknownMethod")
			return
		}

		respond(map[string]any{"accountId": "a1", "list": []any{
			map[string]any{"id": "i1", "email": "user@example.test"},
		}})
	case "Email/set":
		respond(f.emailSet(head[1]))
	case "EmailSubmission/set":
		if !has("urn:ietf:params:jmap:submission") {
			fail("unknownMethod")
			return
		}

		respond(map[string]any{"accountId": "a1", "created": map[string]any{"s1": map[string]any{"id": "s1", "emailId": "e9"}}})
	default:
		fail("unknownMethod")
	}
}

func (f *fakeJMAP) emailSet(raw json.RawMessage) map[string]any {
	var args struct {
		Create  map[string]any `json:"create"`
		Update  map[string]any `json:"update"`
		Destroy []string       `json:"destroy"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return map[string]any{"accountId": "a1"}
	}

	if len(args.Create) > 0 {
		return map[string]any{"accountId": "a1", "created": map[string]any{"draft": map[string]any{"id": "e9"}}}
	}

	if len(args.Destroy) > 0 {
		return map[string]any{"accountId": "a1", "destroyed": args.Destroy}
	}

	updated := map[string]any{}
	for id := range args.Update {
		updated[id] = map[string]any{}
	}

	return map[string]any{"accountId": "a1", "updated": updated}
}

func (f *fakeJMAP) emails(raw json.RawMessage) []any {
	var args struct {
		IDs []string `json:"ids"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return []any{f.email("e1")}
	}

	out := make([]any, 0, len(args.IDs))
	for _, id := range args.IDs {
		out = append(out, f.email(id))
	}

	return out
}

func (f *fakeJMAP) email(id string) map[string]any {
	return map[string]any{
		"id":                id,
		"threadId":          "t1",
		"mailboxIds":        map[string]bool{"m1": true},
		"keywords":          map[string]bool{"$seen": true},
		"size":              100,
		"receivedAt":        "2026-11-10T10:00:00Z",
		"from":              []any{map[string]any{"name": "Sender", "email": "sender@example.test"}},
		"to":                []any{map[string]any{"email": "user@example.test"}},
		"subject":           "hello",
		"header:message-id": []string{"<m1@example.test>"},
		"bodyValues":        map[string]any{"t": map[string]any{"value": "body text", "isTruncated": false}},
		"textBody":          []any{map[string]any{"partId": "t"}},
		"attachments": []any{
			map[string]any{"blobId": "b1", "type": "text/plain", "name": "a.txt", "size": 5},
		},
	}
}

func testBackend(t *testing.T) (*Backend, func()) {
	t.Helper()
	f := &fakeJMAP{t: t}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/jmap", f.session)
	mux.HandleFunc("/jmap/", f.call)
	srv := httptest.NewServer(mux)
	cfg := config.Config{
		Backend:            "jmap",
		DefaultFolder:      "INBOX",
		JMAPURL:            srv.URL,
		JMAPUsername:       "user",
		JMAPPassword:       "secret",
		MaxResults:         50,
		MaxQueryLength:     512,
		MaxMessageBytes:    1 << 20,
		MaxBodyBytes:       1 << 20,
		MaxAttachmentBytes: 1 << 20,
		MaxThreadMessages:  50,
		MaxRecipients:      50,
		ConfirmTTL:         5 * time.Minute,
		OperationTimeout:   10 * time.Second,
	}
	return New(cfg), srv.Close
}

func TestListFolders(t *testing.T) {
	b, done := testBackend(t)
	defer done()
	folders, err := b.ListFolders(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if len(folders) != 4 || folders[0].Name != "Archive" || folders[3].Name != "Sent" {
		t.Fatalf("unexpected folders: %+v", folders)
	}
}

func TestStats(t *testing.T) {
	b, done := testBackend(t)
	defer done()
	stats, err := b.Stats(t.Context(), "inbox")
	if err != nil {
		t.Fatal(err)
	}

	if stats.Total != 10 || stats.Unseen != 2 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestStatsUnknownFolder(t *testing.T) {
	b, done := testBackend(t)
	defer done()
	if _, err := b.Stats(t.Context(), "nope"); err == nil {
		t.Fatal("expected unknown folder error")
	}
}

func TestSearch(t *testing.T) {
	b, done := testBackend(t)
	defer done()
	msgs, err := b.Search(t.Context(), maildomain.SearchQuery{Folder: "INBOX", Text: "hello", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}

	if len(msgs) != 1 || msgs[0].ID != "e1" || msgs[0].Subject != "hello" {
		t.Fatalf("unexpected search: %+v", msgs)
	}

	if len(msgs[0].Flags) != 1 || msgs[0].Flags[0] != `\Seen` {
		t.Fatalf("unexpected flags: %+v", msgs[0].Flags)
	}
}

func TestGetMessage(t *testing.T) {
	b, done := testBackend(t)
	defer done()
	ref, err := maildomain.NormalizeRef("INBOX", "INBOX", 0, "e1")
	if err != nil {
		t.Fatal(err)
	}

	msg, err := b.GetMessage(t.Context(), ref)
	if err != nil {
		t.Fatal(err)
	}

	if msg.BodyText != "body text" || len(msg.Attachments) != 1 {
		t.Fatalf("unexpected message: %+v", msg)
	}

	if msg.Attachments[0].ID != "b1" {
		t.Fatalf("missing blob id: %+v", msg.Attachments[0])
	}
}

func TestGetMessageRejectsUID(t *testing.T) {
	b, done := testBackend(t)
	defer done()
	if _, err := b.GetMessage(t.Context(), maildomain.Ref{Folder: "INBOX", UID: 1}); err == nil {
		t.Fatal("expected uid rejection on jmap backend")
	}
}

func TestGetAttachment(t *testing.T) {
	b, done := testBackend(t)
	defer done()
	ref, err := maildomain.NormalizeRef("INBOX", "INBOX", 0, "e1")
	if err != nil {
		t.Fatal(err)
	}

	att, err := b.GetAttachment(t.Context(), ref, 0)
	if err != nil {
		t.Fatal(err)
	}

	if att.Filename != "a.txt" {
		t.Fatalf("unexpected attachment: %+v", att)
	}
}

func TestGetThread(t *testing.T) {
	b, done := testBackend(t)
	defer done()
	ref, err := maildomain.NormalizeRef("INBOX", "INBOX", 0, "e1")
	if err != nil {
		t.Fatal(err)
	}

	msgs, err := b.GetThread(t.Context(), ref, 10)
	if err != nil {
		t.Fatal(err)
	}

	if len(msgs) != 2 {
		t.Fatalf("expected 2 thread messages, got %+v", msgs)
	}
}

func TestSendFlow(t *testing.T) {
	b, done := testBackend(t)
	defer done()
	receipt, err := b.SendMessage(t.Context(), maildomain.SendRequest{
		To: []string{"user@example.test"}, Subject: "hi", BodyText: "hello",
	})
	if err != nil {
		t.Fatal(err)
	}

	if receipt.ToCount != 1 {
		t.Fatalf("unexpected receipt: %+v", receipt)
	}
}

func TestMoveFlow(t *testing.T) {
	b, done := testBackend(t)
	defer done()
	ref, err := maildomain.NormalizeRef("INBOX", "INBOX", 0, "e1")
	if err != nil {
		t.Fatal(err)
	}

	res, err := b.MoveMessage(t.Context(), maildomain.MoveRequest{Target: ref, Dest: "Archive"})
	if err != nil {
		t.Fatal(err)
	}

	if !res.Moved || res.Dest != "Archive" {
		t.Fatalf("unexpected move: %+v", res)
	}
}

func TestMoveSameFolder(t *testing.T) {
	b, done := testBackend(t)
	defer done()
	ref, err := maildomain.NormalizeRef("INBOX", "INBOX", 0, "e1")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := b.MoveMessage(t.Context(), maildomain.MoveRequest{Target: ref, Dest: "INBOX"}); err == nil {
		t.Fatal("expected same-folder error")
	}
}

func TestDeleteFlow(t *testing.T) {
	b, done := testBackend(t)
	defer done()
	ref, err := maildomain.NormalizeRef("INBOX", "INBOX", 0, "e1")
	if err != nil {
		t.Fatal(err)
	}

	res, err := b.DeleteMessage(t.Context(), maildomain.DeleteRequest{Target: ref})
	if err != nil {
		t.Fatal(err)
	}

	if !res.Deleted || res.ID != "e1" {
		t.Fatalf("unexpected delete: %+v", res)
	}
}

func TestFlagsFlow(t *testing.T) {
	b, done := testBackend(t)
	defer done()
	ref, err := maildomain.NormalizeRef("INBOX", "INBOX", 0, "e1")
	if err != nil {
		t.Fatal(err)
	}

	res, err := b.SetFlags(t.Context(), maildomain.FlagsRequest{Target: ref, Add: []string{"\\Flagged"}})
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Added) != 1 || res.Added[0] != `\Flagged` {
		t.Fatalf("unexpected flags: %+v", res)
	}
}

func TestRewriteAPIHostKeepsConfiguredServer(t *testing.T) {
	got := rewriteAPIHost("http://127.0.0.1:8080", "https://mail.example.test/jmap/")
	if got != "http://127.0.0.1:8080/jmap/" {
		t.Fatalf("unexpected api url: %q", got)
	}

	got = rewriteAPIHost("http://127.0.0.1:8080/jmap", "https://mail.example.test/jmap/")
	if got != "http://127.0.0.1:8080/jmap/" {
		t.Fatalf("unexpected api url: %q", got)
	}
}

func TestSearchRejectsOversize(t *testing.T) {
	b, done := testBackend(t)
	defer done()
	q := maildomain.SearchQuery{Folder: "INBOX", Text: strings.Repeat("x", 600)}
	if _, err := b.Search(t.Context(), q); err == nil {
		t.Fatal("expected query length error")
	}
}

func TestHeaderListAcceptsStringOrArray(t *testing.T) {
	var single headerList
	if err := json.Unmarshal([]byte(`"<a@x>"`), &single); err != nil {
		t.Fatal(err)
	}

	if len(single) != 1 || single[0] != "<a@x>" {
		t.Fatalf("unexpected single: %+v", single)
	}

	var multi headerList
	if err := json.Unmarshal([]byte(`["<a@x>","<b@x>"]`), &multi); err != nil {
		t.Fatal(err)
	}

	if len(multi) != 2 {
		t.Fatalf("unexpected multi: %+v", multi)
	}
}
