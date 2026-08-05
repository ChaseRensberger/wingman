package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/api"
	daemonconfig "github.com/chaserensberger/wingman/internal/config"
)

func TestAuthCommandHierarchy(t *testing.T) {
	cmd := newCommand(daemonconfig.Config{})
	auth := cmd.Command("auth")
	if auth == nil {
		t.Fatal("auth command is missing")
	}
	for _, name := range []string{"sessions", "revoke"} {
		if auth.Command(name) == nil {
			t.Errorf("auth %s command is missing", name)
		}
	}
	if auth.Command("enroll") != nil {
		t.Error("auth enroll command is present")
	}
	if auth.Command("revoke").ArgsUsage != "<session-id>" {
		t.Errorf("revoke ArgsUsage = %q", auth.Command("revoke").ArgsUsage)
	}
}

func TestPublicURLValidationAndResolution(t *testing.T) {
	for _, raw := range []string{"/console", "ftp://example.test", "https:///console", "example.test", "http://example.test"} {
		if _, err := validatePublicURL(raw); err == nil {
			t.Errorf("validatePublicURL(%q) succeeded", raw)
		}
	}
	for _, raw := range []string{"http://localhost:2323", "http://127.0.0.1:2323", "https://example.test"} {
		if _, err := validatePublicURL(raw); err != nil {
			t.Errorf("validatePublicURL(%q) error = %v", raw, err)
		}
	}
	got, err := resolveURL("https://console.example/base/", "/console#view=sessions")
	if err != nil {
		t.Fatal(err)
	}
	if want := "https://console.example/console#view=sessions"; got != want {
		t.Fatalf("resolved URL = %q, want %q", got, want)
	}
}

func TestAuthRevokeRequiresExactlyOneID(t *testing.T) {
	for _, args := range [][]string{{"wingman", "auth", "revoke"}, {"wingman", "auth", "revoke", "one", "two"}} {
		cmd := newCommand(daemonconfig.Config{})
		if err := cmd.Run(context.Background(), args); err == nil || !strings.Contains(err.Error(), "exactly one") {
			t.Errorf("Run(%v) error = %v", args, err)
		}
	}
}

func TestWriteAuthSessions(t *testing.T) {
	var output bytes.Buffer
	writeAuthSessions(&output, nil)
	if got := output.String(); got != "No auth sessions.\n" {
		t.Fatalf("empty sessions output = %q", got)
	}
	output.Reset()
	writeAuthSessions(&output, []api.AuthSession{
		{ID: "ats_active", ClientID: "client", CreatedAt: "created"},
		{ID: "ats_revoked", ClientID: "client", CreatedAt: "created", RevokedAt: "revoked"},
		{ID: "ats_expired", ClientID: "client", CreatedAt: "created", ExpiresAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)},
	})
	got := output.String()
	if !strings.Contains(got, "ats_active") || !strings.Contains(got, "active") || !strings.Contains(got, "ats_revoked") || !strings.Contains(got, "revoked") || !strings.Contains(got, "ats_expired") || !strings.Contains(got, "expired") {
		t.Fatalf("sessions output = %q", got)
	}
}

func TestBrowserCommandName(t *testing.T) {
	for goos, want := range map[string]string{"linux": "xdg-open", "darwin": "open"} {
		got, err := browserCommandName(goos)
		if err != nil || got != want {
			t.Errorf("browserCommandName(%q) = %q, %v", goos, got, err)
		}
	}
	if _, err := browserCommandName("windows"); err == nil {
		t.Error("browserCommandName(windows) succeeded")
	}
}
