package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestRunCancellationRequestPaths(t *testing.T) {
	for _, workflow := range []string{"production", "proxied-development", "vite-development"} {
		t.Run(workflow, func(t *testing.T) {
			frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("API request reached the frontend: %s", r.URL.Path)
			}))
			defer frontend.Close()
			data := memory.NewStore()
			owner, err := data.EnsureDefaultClient()
			if err != nil {
				t.Fatal(err)
			}
			cfg := Config{Store: data, Username: "test", Password: "recovery-test-password"}
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
			for _, specific := range []bool{false, true} {
				sid := fmt.Sprintf("ses_cancel_%t", specific)
				if err := data.CreateSession(&store.Session{ID: sid, ClientID: owner.ID}); err != nil {
					t.Fatal(err)
				}
				admitted, err := data.AdmitSessionRun(t.Context(), store.SessionRun{SessionID: sid})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := data.ClaimNextSessionRun(t.Context(), sid); err != nil {
					t.Fatal(err)
				}
				runCtx, cancel := context.WithCancel(t.Context())
				defer cancel()
				server.runs.runCancel[sid] = cancel
				server.runs.runIDs[sid] = admitted.Run.ID
				path := "/sessions/" + sid + "/abort"
				if specific {
					path = "/sessions/" + sid + "/runs/" + admitted.Run.ID + "/abort"
				}
				request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, target+path, nil)
				if workflow != "vite-development" {
					response, err := http.DefaultClient.Do(request)
					if err != nil {
						t.Fatal(err)
					}
					response.Body.Close()
					if response.StatusCode != http.StatusUnauthorized {
						t.Fatalf("unauthenticated cancellation = %d", response.StatusCode)
					}
					request.SetBasicAuth("test", "recovery-test-password")
				}
				response, err := http.DefaultClient.Do(request)
				if err != nil {
					t.Fatal(err)
				}
				if specific {
					var cancelled api.SessionRun
					err = json.NewDecoder(response.Body).Decode(&cancelled)
					if err != nil || cancelled.Status != "aborted" || cancelled.ErrorType != "cancelled" {
						response.Body.Close()
						t.Fatalf("cancellation response = %#v, %v", cancelled, err)
					}
				}
				response.Body.Close()
				want := http.StatusOK
				if specific {
					want = http.StatusAccepted
				}
				if response.StatusCode != want || runCtx.Err() == nil {
					t.Fatalf("cancel = %d, context = %v", response.StatusCode, runCtx.Err())
				}
				request, _ = http.NewRequestWithContext(t.Context(), http.MethodGet, target+"/sessions/"+sid+"/runs/"+admitted.Run.ID, nil)
				if workflow != "vite-development" {
					request.SetBasicAuth("test", "recovery-test-password")
				}
				response, err = http.DefaultClient.Do(request)
				if err != nil {
					t.Fatal(err)
				}
				var saved api.SessionRun
				err = json.NewDecoder(response.Body).Decode(&saved)
				response.Body.Close()
				if err != nil || saved.Status != "aborted" || saved.ErrorType != "cancelled" {
					t.Fatalf("saved cancellation = %#v, %v", saved, err)
				}
			}
		})
	}
}

func recoveryViteProxy(t *testing.T, target string) string {
	t.Helper()
	bun, err := exec.LookPath("bun")
	if err != nil {
		t.Skip("Bun is required for Vite proxy verification")
	}
	root, err := filepath.Abs("../web/apps/console")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "wingman"), 0700); err != nil {
		t.Fatal(err)
	}
	registration, _ := json.Marshal(map[string]string{"url": target})
	if err := os.WriteFile(filepath.Join(dir, "wingman", "registration.json"), registration, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wingman", "service.env"), []byte("WINGMAN_USERNAME='test'\nWINGMAN_PASSWORD='recovery-test-password'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(bun, "-e", `
import {createServer, loadConfigFromFile} from "vite";
const loaded = await loadConfigFromFile({command:"serve", mode:"development"}, "vite.config.ts");
const server = await createServer({configFile:false, root:process.env.PROXY_CACHE, cacheDir:process.env.PROXY_CACHE, publicDir:false, optimizeDeps:{noDiscovery:true, include:[]}, server:{host:"127.0.0.1", port:0, proxy:loaded.config.server.proxy}});
await server.listen();
console.log("RECOVERY_PROXY="+server.resolvedUrls.local[0]);
`)
	command.Dir = root
	command.Env = append(os.Environ(), "XDG_STATE_HOME="+dir, "XDG_CONFIG_HOME="+dir, "PROXY_CACHE="+filepath.Join(dir, "cache"))
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if url, ok := strings.CutPrefix(scanner.Text(), "RECOVERY_PROXY="); ok {
				ready <- strings.TrimRight(url, "/")
				return
			}
		}
		ready <- ""
	}()
	select {
	case url := <-ready:
		if url == "" {
			t.Fatal("Vite proxy did not start")
		}
		return url
	case <-time.After(30 * time.Second):
		t.Fatal("Vite proxy startup timed out")
		return ""
	}
}
