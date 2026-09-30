package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/chaserensberger/wingman/api"
	daemonconfig "github.com/chaserensberger/wingman/internal/config"
	"github.com/chaserensberger/wingman/internal/daemonclient"
	"github.com/chaserensberger/wingman/internal/daemonstate"
	"github.com/chaserensberger/wingman/server"
	"github.com/chaserensberger/wingman/store/memory"
)

type consoleAPICall struct {
	method string
	path   string
	body   any
}

type fakeConsoleDaemonClient struct {
	workspaces []api.Workspace
	calls      []consoleAPICall
	listErr    error
	createErr  error
	onCreate   func()
}

func (f *fakeConsoleDaemonClient) URL() string { return "http://127.0.0.1:2424" }

func (f *fakeConsoleDaemonClient) DoJSON(_ context.Context, method, path string, body, result any) error {
	f.calls = append(f.calls, consoleAPICall{method: method, path: path, body: body})
	if path != "/workspaces" {
		return fmt.Errorf("unexpected path: %s", path)
	}
	var value any
	switch method {
	case http.MethodGet:
		if f.listErr != nil {
			return f.listErr
		}
		value = f.workspaces
	case http.MethodPost:
		if f.onCreate != nil {
			f.onCreate()
		}
		if f.createErr != nil {
			return f.createErr
		}
		req := body.(api.CreateWorkspaceRequest)
		workspace := api.Workspace{ID: "ws_new", Name: req.Name, Path: req.Path}
		f.workspaces = append(f.workspaces, workspace)
		value = workspace
	default:
		return fmt.Errorf("unexpected method: %s", method)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, result)
}

func TestConsoleCommandOpensHome(t *testing.T) {
	client := &fakeConsoleDaemonClient{}
	target := runTestConsoleCommand(t, client)
	if target != client.URL()+"/console" || len(client.calls) != 0 {
		t.Fatalf("target = %q, calls = %#v", target, client.calls)
	}
}

func TestConsoleCommandCreatesWorkspaceForCurrentDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	client := &fakeConsoleDaemonClient{}
	target := runTestConsoleCommand(t, client, ".")
	if target != client.URL()+"/console/sessions/new?workspace=ws_new" {
		t.Fatalf("target = %q", target)
	}
	if len(client.calls) != 2 || client.calls[1].method != http.MethodPost {
		t.Fatalf("calls = %#v", client.calls)
	}
	req := client.calls[1].body.(api.CreateWorkspaceRequest)
	if req.Path != dir || req.Name != filepath.Base(dir) {
		t.Fatalf("workspace request = %#v", req)
	}
	// Opening another draft must not create another workspace or saved session.
	runTestConsoleCommand(t, client, ".")
	if len(client.calls) != 3 || client.calls[2].method != http.MethodGet {
		t.Fatalf("calls after second invocation = %#v", client.calls)
	}
}

func TestConsoleCommandReusesWorkspaceByPath(t *testing.T) {
	dir := t.TempDir()
	client := &fakeConsoleDaemonClient{workspaces: []api.Workspace{
		{ID: "ws_other", Name: filepath.Base(dir), Path: t.TempDir()},
		{ID: "ws/a & b", Name: "Renamed workspace", Path: dir},
	}}
	target := runTestConsoleCommand(t, client, filepath.Join(dir, "."))
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/console/sessions/new" || parsed.Query().Get("workspace") != "ws/a & b" || len(client.calls) != 1 {
		t.Fatalf("target = %q, calls = %#v", target, client.calls)
	}
}

func TestConsoleWorkspaceNameCollision(t *testing.T) {
	dir := t.TempDir()
	client := &fakeConsoleDaemonClient{workspaces: []api.Workspace{
		{ID: "ws_other", Name: strings.ToUpper(filepath.Base(dir)), Path: t.TempDir()},
	}}
	runTestConsoleCommand(t, client, dir)
	req := client.calls[1].body.(api.CreateWorkspaceRequest)
	if req.Name != dir || req.Path != dir {
		t.Fatalf("workspace request = %#v", req)
	}
}

func TestConsoleWorkspaceConcurrentCreation(t *testing.T) {
	dir := t.TempDir()
	client := &fakeConsoleDaemonClient{createErr: &daemonclient.APIError{StatusCode: http.StatusConflict}}
	client.onCreate = func() {
		client.workspaces = []api.Workspace{{ID: "ws_concurrent", Path: dir}}
	}
	target := runTestConsoleCommand(t, client, dir)
	if !strings.HasSuffix(target, "?workspace=ws_concurrent") || len(client.calls) != 3 {
		t.Fatalf("target = %q, calls = %#v", target, client.calls)
	}
}

func TestConsoleWorkspaceConcurrentNameCollision(t *testing.T) {
	dir := t.TempDir()
	client := &fakeConsoleDaemonClient{}
	client.onCreate = func() {
		if len(client.calls) == 2 {
			client.workspaces = []api.Workspace{{ID: "ws_other", Name: filepath.Base(dir), Path: t.TempDir()}}
			client.createErr = &daemonclient.APIError{StatusCode: http.StatusConflict}
		} else {
			client.createErr = nil
		}
	}
	target := runTestConsoleCommand(t, client, dir)
	if !strings.HasSuffix(target, "?workspace=ws_new") || len(client.calls) != 4 {
		t.Fatalf("target = %q, calls = %#v", target, client.calls)
	}
	first := client.calls[1].body.(api.CreateWorkspaceRequest)
	retry := client.calls[3].body.(api.CreateWorkspaceRequest)
	if first.Name != filepath.Base(dir) || retry.Name != dir || retry.Path != dir {
		t.Fatalf("first = %#v, retry = %#v", first, retry)
	}
}

func TestConsoleWorkspaceFallbackConflict(t *testing.T) {
	for _, sameDirectory := range []bool{true, false} {
		t.Run(fmt.Sprintf("same-directory=%t", sameDirectory), func(t *testing.T) {
			dir := t.TempDir()
			client := &fakeConsoleDaemonClient{createErr: &daemonclient.APIError{StatusCode: http.StatusConflict}}
			client.onCreate = func() {
				if len(client.calls) == 2 {
					client.workspaces = []api.Workspace{{ID: "ws_other", Name: filepath.Base(dir), Path: t.TempDir()}}
				} else {
					path := t.TempDir()
					if sameDirectory {
						path = dir
					}
					client.workspaces = append(client.workspaces, api.Workspace{ID: "ws_fallback", Name: dir, Path: path})
				}
			}
			target, err := consoleTargetURL(context.Background(), client, []string{dir})
			if sameDirectory {
				if err != nil || !strings.HasSuffix(target, "?workspace=ws_fallback") {
					t.Fatalf("target = %q, error = %v", target, err)
				}
			} else if err == nil {
				t.Fatal("expected a name conflict error")
			}
			if len(client.calls) != 5 {
				t.Fatalf("calls = %#v", client.calls)
			}
		})
	}
}

func TestConsoleInvalidDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", " ", filepath.Join(dir, "missing"), file} {
		t.Run(path, func(t *testing.T) {
			client := &fakeConsoleDaemonClient{}
			if _, err := consoleTargetURL(context.Background(), client, []string{path}); err == nil {
				t.Fatal("expected directory error")
			}
			if len(client.calls) != 0 {
				t.Fatalf("calls = %#v", client.calls)
			}
		})
	}
}

func TestConsoleAPIFailures(t *testing.T) {
	dir := t.TempDir()
	want := errors.New("API unavailable")
	for _, client := range []*fakeConsoleDaemonClient{
		{listErr: want},
		{createErr: want},
		{createErr: &daemonclient.APIError{StatusCode: http.StatusConflict}},
	} {
		if _, err := consoleTargetURL(context.Background(), client, []string{dir}); err == nil {
			t.Fatal("expected API error")
		}
	}
}

func TestConsoleCommandRejectsExtraDirectories(t *testing.T) {
	cmd := newCommand(daemonconfig.Config{}).Command("console")
	if err := cmd.Run(context.Background(), []string{"console", ".", "."}); err == nil || !strings.Contains(err.Error(), "expected at most one directory") {
		t.Fatalf("error = %v", err)
	}
}

func TestConsoleBrowserFailure(t *testing.T) {
	old := runBrowserCommand
	t.Cleanup(func() { runBrowserCommand = old })
	want := errors.New("browser unavailable")
	runBrowserCommand = func(context.Context, string, string) error { return want }
	cmd := newCommand(daemonconfig.Config{}).Command("console")
	cmd.Action = func(ctx context.Context, cmd *cli.Command) error {
		return runConsoleWithClient(ctx, cmd, &fakeConsoleDaemonClient{})
	}
	if err := cmd.Run(context.Background(), []string{"console"}); !errors.Is(err, want) {
		t.Fatalf("error = %v", err)
	}
}

func TestConsoleWorkspaceWithAuthenticatedDaemon(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	credentials, err := daemonstate.EnsureServiceConfig()
	if err != nil {
		t.Fatal(err)
	}
	frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/workspaces") {
			t.Error("workspace API reached the Console proxy")
		}
		_, _ = w.Write([]byte("Console development frontend"))
	}))
	defer frontend.Close()
	for _, devURL := range []string{"", frontend.URL} {
		t.Run("console-dev-url="+devURL, func(t *testing.T) {
			data := memory.NewStore()
			handler := server.New(server.Config{
				Store: data, Username: credentials.Username, Password: credentials.Password,
				ConsoleDevURL: devURL,
			})
			daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/ready" {
					_ = json.NewEncoder(w).Encode(api.ReadinessResponse{Ready: true, InstanceID: "console-test", Version: "dev"})
					return
				}
				handler.ServeHTTP(w, r)
			}))
			defer daemon.Close()
			state := daemonstate.New(t.TempDir())
			if err := state.WriteRegistration(daemonstate.Registration{
				InstanceID: "console-test", Version: "dev", URL: daemon.URL, PID: os.Getpid(),
				CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			}); err != nil {
				t.Fatal(err)
			}
			client, err := daemonclient.New(context.Background(), state, "dev")
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			first, err := consoleTargetURL(context.Background(), client, []string{dir})
			if err != nil {
				t.Fatal(err)
			}
			second, err := consoleTargetURL(context.Background(), client, []string{dir})
			if err != nil || first != second {
				t.Fatalf("first = %q, second = %q, error = %v", first, second, err)
			}
			var workspaces []api.Workspace
			if err := client.DoJSON(context.Background(), http.MethodGet, "/workspaces", nil, &workspaces); err != nil {
				t.Fatal(err)
			}
			if len(workspaces) != 1 || workspaces[0].Path != dir {
				t.Fatalf("workspaces = %#v", workspaces)
			}
			parsed, err := url.Parse(first)
			if err != nil {
				t.Fatal(err)
			}
			if parsed.Query().Get("workspace") != workspaces[0].ID {
				t.Fatalf("target = %q, workspace = %#v", first, workspaces[0])
			}
		})
	}
}

func runTestConsoleCommand(t *testing.T, client *fakeConsoleDaemonClient, args ...string) string {
	t.Helper()
	old := runBrowserCommand
	t.Cleanup(func() { runBrowserCommand = old })
	var target string
	runBrowserCommand = func(_ context.Context, name, value string) error {
		if name != "xdg-open" && name != "open" {
			t.Fatalf("browser command = %q", name)
		}
		target = value
		return nil
	}
	cmd := newCommand(daemonconfig.Config{}).Command("console")
	cmd.Action = func(ctx context.Context, cmd *cli.Command) error {
		return runConsoleWithClient(ctx, cmd, client)
	}
	if err := cmd.Run(context.Background(), append([]string{"console"}, args...)); err != nil {
		t.Fatal(err)
	}
	return target
}
