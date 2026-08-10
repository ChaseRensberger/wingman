package main

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"

	daemonconfig "github.com/chaserensberger/wingman/internal/config"
	"github.com/urfave/cli/v3"
)

func TestServiceCommandHierarchyAndHelp(t *testing.T) {
	cmd := newCommand(daemonconfig.Config{})
	service := cmd.Command("service")
	if service == nil {
		t.Fatal("service command is missing")
	}
	for _, name := range []string{"start", "stop", "restart", "status", "password"} {
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
		"password",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("service help missing %q:\n%s", want, output.String())
		}
	}
}

func TestSystemdUnitQuotesServiceArguments(t *testing.T) {
	unit := systemdUnit("/opt/wing man/wingman", "chase", "/home/chase", []string{"/opt/wing man/wingman", "serve", "--state-dir", systemdStateDirEnv, "--db", "/tmp/wing man.db"})
	if !strings.Contains(unit, `User=chase`) {
		t.Fatalf("unit missing service user: %s", unit)
	}
	if !strings.Contains(unit, `ExecStart="/opt/wing man/wingman" "serve" "--state-dir" "${STATE_DIRECTORY}" "--db" "/tmp/wing man.db"`) {
		t.Fatalf("unit does not quote arguments: %s", unit)
	}
	for _, want := range []string{
		"StateDirectory=wingman",
		"StateDirectoryMode=0700",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("unit missing %q: %s", want, unit)
		}
	}
	if strings.Contains(unit, "network-online.target") {
		t.Fatalf("unit unexpectedly waits for network availability: %s", unit)
	}
}

func TestManagedStateDir(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux systemd state directory only")
	}
	if got, err := managedStateDir(); err != nil || got != systemdStateDir {
		t.Fatalf("managedStateDir() = %q, %v; want %q, nil", got, err, systemdStateDir)
	}
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
	plist := launchdPlist("/Users/chase&co", []string{"/Users/chase/bin/wingman", "serve", "--db", "a&b"})
	for _, want := range []string{
		`<string>actor.wingman</string>`,
		`<string>/Users/chase/bin/wingman</string>`,
		`<string>a&amp;b</string>`,
		`<key>RunAtLoad</key>`,
		`<key>KeepAlive</key>`,
		`<key>HOME</key>`,
		`<string>/Users/chase&amp;co</string>`,
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
