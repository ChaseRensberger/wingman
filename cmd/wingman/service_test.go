package main

import (
	"runtime"
	"strings"
	"testing"
)

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
