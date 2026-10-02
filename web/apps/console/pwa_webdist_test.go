//go:build webdist

package console_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/server"
)

func TestBuiltConsolePWAWithExistingServer(t *testing.T) {
	s := server.New(server.Config{Password: "secret"})
	t.Cleanup(func() { _ = s.Close(t.Context()) })
	for _, asset := range []struct{ path, contentType string }{
		{"/console/", "text/html"},
		{"/console/providers/ollama.local", "text/html"},
		{"/console/sw.js", "javascript"},
		{"/console/manifest.json", "application/json"},
		{"/console/apple-touch-icon.png", "image/png"},
		{"/console/icon-192.png", "image/png"},
		{"/console/icon-512.png", "image/png"},
		{"/console/icon-512-maskable.png", "image/png"},
	} {
		t.Run(asset.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, asset.path, nil)
			response := httptest.NewRecorder()
			s.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated status = %d", response.Code)
			}
			request.SetBasicAuth("wingman", "secret")
			response = httptest.NewRecorder()
			s.ServeHTTP(response, request)
			if response.Code != http.StatusOK || !strings.Contains(response.Header().Get("Content-Type"), asset.contentType) {
				t.Fatalf("status = %d, content type = %q", response.Code, response.Header().Get("Content-Type"))
			}
			if strings.HasSuffix(asset.path, "manifest.json") {
				var manifest struct {
					Scope    string `json:"scope"`
					StartURL string `json:"start_url"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &manifest); err != nil {
					t.Fatal(err)
				}
				if manifest.Scope != "/console/" || manifest.StartURL != "/console/" {
					t.Fatalf("manifest = %+v", manifest)
				}
			}
		})
	}
}
