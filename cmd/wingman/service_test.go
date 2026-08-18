package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	daemonconfig "github.com/chaserensberger/wingman/internal/config"
	"github.com/urfave/cli/v3"
)

type fakeManagedDaemonAPI struct {
	catalog      catalogResponse
	auth         providersAuthResponse
	stored       setProvidersAuthRequest
	requests     []string
	requestError error
}

func (f *fakeManagedDaemonAPI) DoJSON(_ context.Context, method, path string, requestBody, responseBody any) error {
	f.requests = append(f.requests, method+" "+path)
	if f.requestError != nil {
		return f.requestError
	}
	switch {
	case method == "GET" && path == "/catalog":
		*responseBody.(*catalogResponse) = f.catalog
	case method == "GET" && path == "/provider/auth":
		*responseBody.(*providersAuthResponse) = f.auth
	case method == "PUT" && path == "/provider/auth":
		f.stored = requestBody.(setProvidersAuthRequest)
	default:
		return fmt.Errorf("unexpected request %s %s", method, path)
	}
	return nil
}

func TestServiceCommandHierarchyAndHelp(t *testing.T) {
	cmd := newCommand(daemonconfig.Config{})
	if cmd.Command("pair") == nil {
		t.Fatal("pair command is missing")
	}
	service := cmd.Command("service")
	if service == nil {
		t.Fatal("service command is missing")
	}
	for _, name := range []string{"start", "stop", "restart", "status"} {
		if service.Command(name) == nil {
			t.Errorf("service %s command is missing", name)
		}
	}
	for _, name := range []string{"up", "down", "restart", "status"} {
		if cmd.Command(name) != nil {
			t.Errorf("legacy top-level %s command is present", name)
		}
	}

	var output bytes.Buffer
	cmd.Writer = &output
	if err := cmd.Run(context.Background(), []string{"wingman", "service", "--help"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Manage Wingman as a background service",
		"start",
		"stop",
		"restart",
		"status",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("service help missing %q:\n%s", want, output.String())
		}
	}
}

func TestSystemdUnitQuotesServiceArguments(t *testing.T) {
	unit := systemdUnit("/home/chase", "", "/home/chase/.config/wingman/service.env", []string{"/opt/wing man/wingman", "serve", "--state-dir", "/state", "--db", "/tmp/wing man.db"})
	if !strings.Contains(unit, `ExecStart="/opt/wing man/wingman" "serve" "--state-dir" "/state" "--db" "/tmp/wing man.db"`) {
		t.Fatalf("unit does not quote arguments: %s", unit)
	}
	for _, want := range []string{"EnvironmentFile=\"/home/chase/.config/wingman/service.env\"", "WantedBy=default.target"} {
		if !strings.Contains(unit, want) {
			t.Errorf("unit missing %q: %s", want, unit)
		}
	}
	if strings.Contains(unit, "network-online.target") {
		t.Fatalf("unit unexpectedly waits for network availability: %s", unit)
	}
}

func TestImportEnvironmentCredentials(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai-key")
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-key")
	api := &fakeManagedDaemonAPI{
		catalog: catalogResponse{Providers: []catalogProvider{
			{ID: "openai", Env: []string{"OPENAI_API_KEY"}},
			{ID: "anthropic", Env: []string{"ANTHROPIC_API_KEY"}},
			{ID: "missing", Env: []string{"MISSING_API_KEY"}},
		}},
		auth: providersAuthResponse{Providers: map[string]providerAuthInfo{
			"anthropic": {Configured: true},
		}},
	}

	if err := importEnvironmentCredentials(context.Background(), api); err != nil {
		t.Fatal(err)
	}
	if got, want := api.requests, []string{"GET /catalog", "GET /provider/auth", "PUT /provider/auth"}; !equalStrings(got, want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
	if got, want := api.stored.Providers, map[string]providerAPIKeyCredential{
		"openai": {Type: "api_key", Key: "openai-key"},
	}; !equalCredentials(got, want) {
		t.Fatalf("stored credentials = %#v, want %#v", got, want)
	}
}

func TestImportEnvironmentCredentialsDoesNotReplaceStoredCredentials(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "new-key")
	api := &fakeManagedDaemonAPI{
		catalog: catalogResponse{Providers: []catalogProvider{{ID: "openai", Env: []string{"OPENAI_API_KEY"}}}},
		auth:    providersAuthResponse{Providers: map[string]providerAuthInfo{"openai": {Configured: true}}},
	}

	if err := importEnvironmentCredentials(context.Background(), api); err != nil {
		t.Fatal(err)
	}
	if got, want := api.requests, []string{"GET /catalog", "GET /provider/auth"}; !equalStrings(got, want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
}

func TestImportEnvironmentCredentialsReturnsRequestError(t *testing.T) {
	want := errors.New("offline")
	err := importEnvironmentCredentials(context.Background(), &fakeManagedDaemonAPI{requestError: want})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want wrapped %v", err, want)
	}
}

func equalStrings(got, want []string) bool {
	return slices.Equal(got, want)
}

func equalCredentials(got, want map[string]providerAPIKeyCredential) bool {
	return maps.Equal(got, want)
}

func TestConsoleDevURLFlagAndServiceForwarding(t *testing.T) {
	var args []string
	cmd := &cli.Command{
		Flags: serveFlags(daemonconfig.Config{}),
		Action: func(_ context.Context, cmd *cli.Command) error {
			args = serveArgs("wingman", cmd, "/state")
			return nil
		},
	}
	if err := cmd.Run(context.Background(), []string{"wingman", "--console-dev-url", "http://127.0.0.1:5173"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(args, " "), "--console-dev-url http://127.0.0.1:5173") {
		t.Fatalf("service arguments = %q", args)
	}
	if strings.Contains(strings.Join(args, " "), "password") {
		t.Fatalf("service arguments expose a password: %q", args)
	}
}

func TestLaunchdPlistEscapesArguments(t *testing.T) {
	plist := launchdPlist("/Users/chase&co", "", "/Users/chase&co/.config/wingman/service.env", []string{"/Users/chase/bin/wingman", "serve", "--db", "a&b"})
	for _, want := range []string{
		`<string>actor.wingman</string>`,
		`<string>/Users/chase/bin/wingman</string>`,
		`<string>a&amp;b</string>`,
		`<key>RunAtLoad</key>`,
		`<key>KeepAlive</key>`,
		`<key>HOME</key>`,
		`<string>/Users/chase&amp;co</string>`,
		`<string>/Users/chase&amp;co/.config/wingman/service.env</string>`,
	} {
		if !strings.Contains(plist, want) {
			t.Errorf("plist missing %q:\n%s", want, plist)
		}
	}
}

func TestLaunchdPlistPath(t *testing.T) {
	if got, want := launchdPlistPath("/Users/chase"), "/Users/chase/Library/LaunchAgents/actor.wingman.plist"; got != want {
		t.Errorf("launchdPlistPath() = %q, want %q", got, want)
	}
}

func TestSystemdUsesXDGConfigHome(t *testing.T) {
	if got, want := systemdUnitPath("/home/chase", "/srv/config"), "/srv/config/systemd/user/wingman.service"; got != want {
		t.Fatalf("systemdUnitPath() = %q, want %q", got, want)
	}
	unit := systemdUnit("/home/chase", "/srv/config", "/srv/config/wingman/service.env", []string{"wingman", "serve"})
	if !strings.Contains(unit, `Environment="XDG_CONFIG_HOME=/srv/config"`) {
		t.Fatalf("unit does not preserve XDG_CONFIG_HOME: %s", unit)
	}
}

func TestLaunchdPreservesXDGConfigHome(t *testing.T) {
	plist := launchdPlist("/Users/chase", "/srv/config&more", "/srv/config&more/wingman/service.env", []string{"wingman", "serve"})
	for _, want := range []string{`<key>XDG_CONFIG_HOME</key>`, `<string>/srv/config&amp;more</string>`} {
		if !strings.Contains(plist, want) {
			t.Fatalf("plist missing %q:\n%s", want, plist)
		}
	}
}
