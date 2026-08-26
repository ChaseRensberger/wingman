package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/api"
)

func TestConnectPersistsPasswordAfterReadiness(t *testing.T) {
	const password = "client-password"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ready":
			username, supplied, ok := r.BasicAuth()
			if !ok || username != "wingman" || supplied != password {
				t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
			}
			_ = json.NewEncoder(w).Encode(api.ReadinessResponse{Ready: true, InstanceID: "instance_one", Version: "dev"})
		default:
			t.Fatalf("path = %q", r.URL.Path)
		}
	}))
	defer server.Close()

	file := filepath.Join(t.TempDir(), "state", "password")
	var output strings.Builder
	if err := connect(context.Background(), []string{"--server", server.URL, "--password", password, "--password-file", file}, &output); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil || string(data) != password+"\n" {
		t.Fatalf("password file = %q, %v", data, err)
	}
	info, err := os.Stat(file)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("password mode = %v, %v", info.Mode(), err)
	}
	if !strings.Contains(output.String(), "Instance: instance_one") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestConnectRejectsEnrollmentCredential(t *testing.T) {
	err := connect(context.Background(), []string{"--credential", "enrollment"}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("connect error = %v", err)
	}
}

func TestStatusReportsAuthenticationFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(api.ErrorResponse{Error: api.Error{Message: "authentication required"}})
	}))
	defer server.Close()

	file := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(file, []byte("expired\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := status(context.Background(), []string{"--server", server.URL, "--password-file", file}, &strings.Builder{}); err == nil || !strings.Contains(err.Error(), "authentication required") {
		t.Fatalf("status error = %v", err)
	}
}

func TestServerURLRejectsInsecureRemoteOrigin(t *testing.T) {
	if _, err := serverURL("http://wingman.example"); err == nil {
		t.Fatal("insecure remote origin was accepted")
	}
	if _, err := serverURL("http://localhost:2424"); err != nil {
		t.Fatal(err)
	}
}
