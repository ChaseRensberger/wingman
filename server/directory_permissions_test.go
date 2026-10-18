package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestPermissionGrantCoversDirectoriesOnlyForReadAndEdit(t *testing.T) {
	directory := filepath.ToSlash(filepath.Join(t.TempDir(), "reference?"))
	for _, test := range []struct {
		name, grantAction, grant, action, resource string
		want                                       bool
	}{
		{"same directory", "read", directory + "/*", "read", directory + "/*", true},
		{"descendant", "read", directory + "/*", "read", directory + "/nested/*", true},
		{"descendant file", "edit", directory + "/*", "edit", directory + "/nested/file.md", true},
		{"different action", "read", directory + "/*", "edit", directory + "/*", false},
		{"sibling", "read", directory + "/*", "read", directory + "-other/*", false},
		{"parent", "read", directory + "/nested/*", "read", directory + "/*", false},
		{"traversal", "read", directory + "/*", "read", directory + "/../other/*", false},
		{"literal question mark", "read", directory + "/*", "read", directory[:len(directory)-1] + "x/*", false},
		{"exact file", "read", directory + "/one.md", "read", directory + "/one.md", true},
		{"other file", "read", directory + "/one.md", "read", directory + "/two.md", false},
		{"exact command", "bash", "echo *", "bash", "echo *", true},
		{"command wildcard stays literal", "bash", "echo *", "bash", "echo secret", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			grant := store.PermissionGrant{Action: test.grantAction, Resource: test.grant}
			if got := permissionGrantCovers(grant, test.action, test.resource); got != test.want {
				t.Fatalf("permissionGrantCovers() = %v; want %v", got, test.want)
			}
		})
	}
}

func TestDirectoryApprovalSurvivesRestartAndStaysSessionScoped(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "wingman.db")
	data, err := store.NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"ses_approved", "ses_other"} {
		if err := data.CreateSession(&store.Session{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	directory := filepath.ToSlash(filepath.Join(t.TempDir(), "reference"))
	request, err := data.CreatePermissionRequest(ctx, store.PermissionRequest{SessionID: "ses_approved", Action: "read", Resources: []string{directory + "/*"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.ResolvePermissionRequest(ctx, store.PermissionRequestResolution{
		SessionID: "ses_approved", RequestID: request.Request.ID, Status: store.PermissionRequestStatusApproved, Response: store.PermissionResponseAlways,
	}); err != nil {
		t.Fatal(err)
	}
	if err := data.Close(); err != nil {
		t.Fatal(err)
	}
	data, err = store.NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })
	server := New(Config{Store: data, PermissionTimeout: 10 * time.Millisecond})
	for _, test := range []struct {
		name, sessionID, action, resource string
		approved                          bool
	}{
		{"same directory", "ses_approved", "read", directory + "/*", true},
		{"subdirectory", "ses_approved", "read", directory + "/nested/*", true},
		{"write is separate", "ses_approved", "edit", directory + "/*", false},
		{"other session", "ses_other", "read", directory + "/*", false},
		{"sibling directory", "ses_approved", "read", directory + "-other/*", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, err := server.permissionRequests.prompter(test.sessionID, "").Request(ctx, run.PermissionRequestInfo{Action: test.action, Resources: []string{test.resource}})
			if test.approved {
				if err != nil || response != run.PermissionResponseAlways {
					t.Fatalf("remembered approval = %q, %v", response, err)
				}
			} else if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("unapproved request = %q, %v; want a new approval request", response, err)
			}
		})
	}
	requests, err := data.ListPermissionRequests(ctx, "ses_approved")
	if err != nil || len(requests) != 3 {
		t.Fatalf("requests = %#v, %v; want the original approval and two new requests", requests, err)
	}
}

func TestDirectoryPermissionRequestPaths(t *testing.T) {
	for _, workflow := range []string{"production", "proxied-development", "vite-development"} {
		t.Run(workflow, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
			defer cancel()
			data := memory.NewStore()
			owner, err := data.EnsureDefaultClient()
			if err != nil {
				t.Fatal(err)
			}
			const sessionID = "ses_directory_http"
			if err := data.CreateSession(&store.Session{ID: sessionID, ClientID: owner.ID}); err != nil {
				t.Fatal(err)
			}
			frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("API request reached the frontend: %s", r.URL.Path)
			}))
			defer frontend.Close()
			cfg := Config{Store: data, Username: "test", Password: "recovery-test-password", PermissionTimeout: 30 * time.Second}
			if workflow == "proxied-development" {
				cfg.ConsoleDevURL = frontend.URL
			}
			server := New(cfg)
			defer server.Close(context.Background())
			host := httptest.NewServer(server)
			defer host.Close()
			target := host.URL
			if workflow == "vite-development" {
				target = recoveryViteProxy(t, host.URL)
			}
			request := func(method, path, body string) *http.Response {
				t.Helper()
				req, err := http.NewRequestWithContext(ctx, method, target+path, strings.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				if workflow != "vite-development" {
					req.SetBasicAuth("test", "recovery-test-password")
				}
				req.Header.Set("X-Wingman-Client", owner.ID)
				req.Header.Set("Content-Type", "application/json")
				response, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				if response.StatusCode != http.StatusOK {
					response.Body.Close()
					t.Fatalf("%s %s = %d", method, path, response.StatusCode)
				}
				return response
			}
			directory := filepath.ToSlash(filepath.Join(t.TempDir(), "reference"))
			outcome := make(chan struct {
				response run.PermissionResponse
				err      error
			}, 1)
			go func() {
				response, err := server.permissionRequests.prompter(sessionID, "").Request(ctx, run.PermissionRequestInfo{Action: "read", Resources: []string{directory + "/*"}})
				outcome <- struct {
					response run.PermissionResponse
					err      error
				}{response, err}
			}()
			pending := waitForPermissionRequest(t, data, sessionID)
			response := request(http.MethodGet, "/sessions/"+sessionID+"/permission-requests", "")
			var requests []api.PermissionRequest
			err = json.NewDecoder(response.Body).Decode(&requests)
			response.Body.Close()
			if err != nil || len(requests) != 1 || requests[0].Action != "read" || len(requests[0].Resources) != 1 || requests[0].Resources[0] != directory+"/*" {
				t.Fatalf("requests = %#v, %v", requests, err)
			}
			response = request(http.MethodPost, "/sessions/"+sessionID+"/permission-requests/"+pending.ID+"/reply", `{"response":"always"}`)
			var resolved api.PermissionRequest
			err = json.NewDecoder(response.Body).Decode(&resolved)
			response.Body.Close()
			if err != nil || resolved.Status != "approved" || resolved.Response != "always" {
				t.Fatalf("resolved = %#v, %v", resolved, err)
			}
			select {
			case result := <-outcome:
				if result.err != nil || result.response != run.PermissionResponseAlways {
					t.Fatalf("approval = %#v", result)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			response = request(http.MethodGet, "/sessions/"+sessionID+"/permission-grants", "")
			var grants []api.PermissionGrant
			err = json.NewDecoder(response.Body).Decode(&grants)
			response.Body.Close()
			if err != nil || len(grants) != 1 || grants[0].Action != "read" || grants[0].Resource != directory+"/*" {
				t.Fatalf("grants = %#v, %v", grants, err)
			}
			remembered, err := server.permissionRequests.prompter(sessionID, "").Request(ctx, run.PermissionRequestInfo{Action: "read", Resources: []string{directory + "/nested/*"}})
			if err != nil || remembered != run.PermissionResponseAlways {
				t.Fatalf("remembered = %q, %v", remembered, err)
			}
		})
	}
}
