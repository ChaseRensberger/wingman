package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWritePairingInfo(t *testing.T) {
	info := pairingInfo{URLs: []string{"http://192.0.2.1:2323", "http://198.51.100.1:2323"}, Username: "wingman", Password: "secret"}
	payload, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(payload), `{"urls":["http://192.0.2.1:2323","http://198.51.100.1:2323"],"username":"wingman","password":"secret"}`; got != want {
		t.Fatalf("pairing payload = %q, want %q", got, want)
	}
	var output bytes.Buffer
	if err := writePairingInfo(&output, info); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"URLs      http://192.0.2.1:2323",
		"            http://198.51.100.1:2323",
		"Username  wingman",
		"Password  secret",
		"Scan to pair",
		"█",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("pairing output missing %q:\n%s", want, output.String())
		}
	}
}

func TestWritePairingInfoIncludesRemoteHintForLoopbackURL(t *testing.T) {
	var output bytes.Buffer
	err := writePairingInfo(&output, pairingInfo{URLs: []string{"http://127.0.0.1:2323"}, Username: "wingman", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "wingman service start --host 0.0.0.0") {
		t.Fatalf("pairing output missing remote hint:\n%s", output.String())
	}
}

func TestWritePairingInfoOmitsRemoteHintForRemoteURL(t *testing.T) {
	var output bytes.Buffer
	err := writePairingInfo(&output, pairingInfo{URLs: []string{"https://wingman.example"}, Username: "wingman", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "0.0.0.0") {
		t.Fatalf("pairing output includes remote hint:\n%s", output.String())
	}
}
