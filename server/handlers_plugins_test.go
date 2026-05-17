package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chaserensberger/wingman/pluginhost"
)

func TestPluginsEndpoints(t *testing.T) {
	t.Parallel()

	mgr, err := pluginhost.New(context.Background(), []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()

	srv := New(Config{Plugins: mgr})

	for _, tc := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/plugins/"},
		{method: http.MethodPost, path: "/plugins/reload"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}
