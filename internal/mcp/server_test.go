package mcp

import (
	"testing"
	"time"
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
