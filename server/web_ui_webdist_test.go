//go:build webdist

package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	consoleui "github.com/chaserensberger/wingman/web/apps/console"
)

func TestEmbeddedConsoleServesUnderscoreRouteChunks(t *testing.T) {
	dist, err := fs.Sub(consoleui.Dist, consoleui.DistRoot)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := fs.ReadDir(dist, "assets")
	if err != nil {
		t.Fatal(err)
	}

	asset := ""
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "_") && strings.HasSuffix(entry.Name(), ".js") {
			asset = entry.Name()
			break
		}
	}
	if asset == "" {
		t.Fatal("embedded console has no underscore-prefixed route chunk")
	}

	s := New(Config{})
	t.Cleanup(func() { _ = s.Close(t.Context()) })
	response := httptest.NewRecorder()
	s.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/console/assets/"+asset, nil))

	if response.Code != http.StatusOK {
		t.Fatalf("route chunk status = %d, body = %s", response.Code, response.Body.String())
	}
	if strings.HasPrefix(response.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("route chunk content type = %q", response.Header().Get("Content-Type"))
	}
}
