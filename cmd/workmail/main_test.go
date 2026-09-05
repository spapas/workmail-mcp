package main

import (
	"strings"
	"testing"
)

func TestParseDoctorArgs(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		got, err := parseDoctorArgs(nil)
		if err != nil {
			t.Fatalf("parseDoctorArgs() error = %v", err)
		}
		if got {
			t.Fatal("parseDoctorArgs() = true, want false")
		}
	})

	t.Run("latest subject", func(t *testing.T) {
		got, err := parseDoctorArgs([]string{"--latest-subject"})
		if err != nil {
			t.Fatalf("parseDoctorArgs() error = %v", err)
		}
		if !got {
			t.Fatal("parseDoctorArgs() = false, want true")
		}
	})

	t.Run("unknown", func(t *testing.T) {
		if _, err := parseDoctorArgs([]string{"--unknown"}); err == nil {
			t.Fatal("parseDoctorArgs() error = nil, want error")
		}
	})
}

func TestDisplaySubject(t *testing.T) {
	if got := displaySubject("  hello\r\n   world  "); got != "hello world" {
		t.Fatalf("displaySubject() = %q, want %q", got, "hello world")
	}
	if got := displaySubject(" \r\n\t "); got != "<no subject>" {
		t.Fatalf("displaySubject(empty) = %q, want %q", got, "<no subject>")
	}
	long := strings.Repeat("α", 201)
	got := displaySubject(long)
	if len([]rune(got)) != 203 || !strings.HasSuffix(got, "...") {
		t.Fatalf("displaySubject(long) was not rune-safe truncated: %q", got)
	}
}
