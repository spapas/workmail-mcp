package mcp

import (
	"context"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	maildomain "github.com/spapas/workmail-mcp/internal/mail"
)

func TestParseDate(t *testing.T) {
	tests := []struct {
		input string
		zero  bool
		err   bool
	}{
		{"", true, false},
		{"2026-09-05", false, false},
		{"2026-09-05T12:30:00Z", false, false},
		{"not-a-date", false, true},
	}
	for _, tc := range tests {
		got, err := parseDate(tc.input)
		if (err != nil) != tc.err {
			t.Fatalf("parseDate(%q) error=%v", tc.input, err)
		}
		if !tc.err && got.IsZero() != tc.zero {
			t.Fatalf("parseDate(%q) zero=%v", tc.input, got.IsZero())
		}
		if !tc.err && tc.input == "2026-09-05" && got.Format(time.DateOnly) != tc.input {
			t.Fatalf("unexpected date: %v", got)
		}
	}
}

// stubService is a minimal mail.Service for handler tests.
type stubService struct {
	stats maildomain.FolderStats
}

func (s *stubService) ListFolders(context.Context) ([]maildomain.Folder, error) {
	return nil, nil
}

func (s *stubService) Search(context.Context, maildomain.SearchQuery) ([]maildomain.MessageSummary, error) {
	return nil, nil
}

func (s *stubService) GetMessage(context.Context, string, uint32) (maildomain.Message, error) {
	return maildomain.Message{}, nil
}

func (s *stubService) GetAttachment(context.Context, string, uint32, int) (maildomain.Attachment, error) {
	return maildomain.Attachment{}, nil
}

func (s *stubService) GetThread(context.Context, string, uint32, int) ([]maildomain.MessageSummary, error) {
	return nil, nil
}

func (s *stubService) Stats(_ context.Context, folder string) (maildomain.FolderStats, error) {
	out := s.stats
	if out.Folder == "" {
		out.Folder = folder
	}
	return out, nil
}

func TestStatsHandlerReturnsCounts(t *testing.T) {
	ts := &toolSet{service: &stubService{stats: maildomain.FolderStats{Folder: "INBOX", Total: 120, Unseen: 7}}}
	_, out, err := ts.stats(context.Background(), &mcpsdk.CallToolRequest{}, statsInput{Folder: "INBOX"})
	if err != nil {
		t.Fatalf("stats failed: %v", err)
	}
	if out.Stats.Total != 120 || out.Stats.Unseen != 7 || out.Stats.Folder != "INBOX" {
		t.Fatalf("unexpected stats: %+v", out.Stats)
	}
}
