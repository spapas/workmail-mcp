package mcp

import (
	"context"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/spapas/workmail-mcp/internal/config"
	maildomain "github.com/spapas/workmail-mcp/internal/mail"
)

type fakeWriter struct {
	sent    []maildomain.SendRequest
	moved   []maildomain.MoveRequest
	deleted []maildomain.DeleteRequest
	flagged []maildomain.FlagsRequest
}

func (f *fakeWriter) SendMessage(_ context.Context, req maildomain.SendRequest) (maildomain.SendReceipt, error) {
	f.sent = append(f.sent, req)
	return maildomain.SendReceipt{ToCount: len(req.To), Subject: req.Subject, MessageID: "<test@example.com>"}, nil
}

func (f *fakeWriter) MoveMessage(_ context.Context, req maildomain.MoveRequest) (maildomain.MoveResult, error) {
	f.moved = append(f.moved, req)
	return maildomain.MoveResult{Folder: req.Folder, UID: req.UID, Dest: req.Dest, Moved: true}, nil
}

func (f *fakeWriter) DeleteMessage(_ context.Context, req maildomain.DeleteRequest) (maildomain.DeleteResult, error) {
	f.deleted = append(f.deleted, req)
	return maildomain.DeleteResult{Folder: req.Folder, UID: req.UID, Deleted: true}, nil
}

func (f *fakeWriter) SetFlags(_ context.Context, req maildomain.FlagsRequest) (maildomain.FlagsResult, error) {
	f.flagged = append(f.flagged, req)
	return maildomain.FlagsResult{Folder: req.Folder, UID: req.UID, Added: req.Add, Removed: req.Remove}, nil
}

func testToolSet() (*toolSet, *fakeWriter) {
	w := &fakeWriter{}
	ts := &toolSet{
		writer:  w,
		pending: newPendingStore(time.Minute),
		cfg:     config.Config{DefaultFolder: "INBOX", MaxRecipients: 50, MaxBodyBytes: 1 << 20},
	}
	return ts, w
}

func TestSendRequestConfirmFlow(t *testing.T) {
	ts, w := testToolSet()
	ctx := context.Background()
	req := &mcpsdk.CallToolRequest{}
	_, pending, err := ts.sendRequest(ctx, req, sendRequestInput{To: []string{"a@example.com"}, Subject: "hi", BodyText: "hello"})
	if err != nil {
		t.Fatalf("sendRequest failed: %v", err)
	}
	if pending.PendingID == "" {
		t.Fatal("empty pending ID")
	}
	if len(w.sent) != 0 {
		t.Fatal("request step must not send")
	}
	_, out, err := ts.confirm(ctx, req, confirmInput{PendingID: pending.PendingID})
	if err != nil {
		t.Fatalf("confirm failed: %v", err)
	}
	if out.Send == nil || len(w.sent) != 1 {
		t.Fatalf("confirm did not execute send: %+v", out)
	}
	if _, _, err := ts.confirm(ctx, req, confirmInput{PendingID: pending.PendingID}); err == nil {
		t.Fatal("second confirm must fail (single-use)")
	}
}

func TestDeleteConfirmExecutesFrozenPayload(t *testing.T) {
	ts, w := testToolSet()
	ctx := context.Background()
	req := &mcpsdk.CallToolRequest{}
	_, pending, err := ts.deleteRequest(ctx, req, deleteRequestInput{Folder: "INBOX", UID: 42})
	if err != nil {
		t.Fatalf("deleteRequest failed: %v", err)
	}
	if _, _, err := ts.confirm(ctx, req, confirmInput{PendingID: "nope"}); err == nil {
		t.Fatal("unknown ID must fail")
	}
	if _, _, err := ts.confirm(ctx, req, confirmInput{PendingID: pending.PendingID}); err != nil {
		t.Fatalf("confirm failed: %v", err)
	}
	if len(w.deleted) != 1 || w.deleted[0].UID != 42 {
		t.Fatalf("unexpected delete calls: %+v", w.deleted)
	}
}

func TestFlagsRequestRejectsDeleted(t *testing.T) {
	ts, _ := testToolSet()
	if _, _, err := ts.setFlagsRequest(context.Background(), &mcpsdk.CallToolRequest{}, setFlagsRequestInput{Folder: "INBOX", UID: 1, Add: []string{"\\Deleted"}}); err == nil {
		t.Fatal("expected \\Deleted rejection")
	}
}
