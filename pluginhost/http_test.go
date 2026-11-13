package pluginhost

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/route"
	sdk "github.com/chaserensberger/wingman/plugin"
	"github.com/chaserensberger/wingman/plugin/protocol"
)

type upperBody struct{ io.ReadCloser }

func (b upperBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	copy(p[:n], bytes.ToUpper(p[:n]))
	return n, err
}

func TestHTTPPluginHelper(t *testing.T) {
	if os.Getenv("WINGMAN_HTTP_PLUGIN") != "1" {
		return
	}
	err := sdk.Serve(context.Background(), sdk.Definition{ID: "test/http", Setup: func(context.Context, *sdk.Host, map[string]any) (*sdk.Extension, error) {
		return &sdk.Extension{HTTP: sdk.HTTPHooks{
			Request: func(ctx context.Context, model models.ModelRef, req *http.Request) (*http.Request, error) {
				req.Header.Set("X-Plugin", "yes")
				return req, nil
			},
			Response: func(ctx context.Context, model models.ModelRef, resp *http.Response) (*http.Response, error) {
				if model.ID == "pipe" {
					reader, writer := io.Pipe()
					source := resp.Body
					go func() { _, err := io.Copy(writer, upperBody{source}); _ = writer.CloseWithError(err) }()
					resp.Body, resp.ContentLength = reader, -1
					return resp, nil
				}
				if model.ID == "prefix" {
					prefix := make([]byte, 2)
					if _, err := io.ReadFull(resp.Body, prefix); err != nil {
						return nil, err
					}
					return resp, nil
				}
				resp.Body = upperBody{resp.Body}
				return resp, nil
			},
		}}, nil
	}})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func httpActivation(t *testing.T) *Activation {
	t.Helper()
	t.Setenv("WINGMAN_HTTP_PLUGIN", "1")
	dir := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(Manifest{ID: "test/http", Command: []string{exe, "-test.run=^TestHTTPPluginHelper$"}})
	if err := os.WriteFile(filepath.Join(dir, "wingman-plugin.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	m, err := New(context.Background(), []string{dir})
	if err != nil {
		t.Fatal(err)
	}
	m.SetHostHandler(func(context.Context, string, json.RawMessage) (any, error) {
		return nil, fmt.Errorf("unsupported callback")
	})
	a, err := m.Acquire(protocol.Identity{ClientID: "client"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close(context.Background()); _ = m.Close() })
	return a
}

func TestNativeHTTPHooksPreserveStreamingBodies(t *testing.T) {
	for _, model := range []string{"upper", "pipe"} {
		t.Run(model, func(t *testing.T) { testHTTPStreaming(t, model) })
	}
}

func TestNativeHTTPHeaderHookPreservesRedirectReplay(t *testing.T) {
	a := httpActivation(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			w.Header().Set("Location", "/final")
			status := http.StatusTemporaryRedirect
			if r.URL.Query().Get("permanent") != "" {
				status = http.StatusPermanentRedirect
			}
			w.WriteHeader(status)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || string(body) != `{"prompt":"hello"}` || r.Header.Get("X-Plugin") != "yes" {
			t.Errorf("redirected request: %s %s %v", r.Method, body, r.Header)
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer upstream.Close()
	for _, suffix := range []string{"", "?permanent=1"} {
		response, err := (route.HTTP{}).Open(route.WithHTTPHooks(t.Context(), a.HTTPHooks()), route.Route{}, &models.PreparedRequest{Model: models.ModelRef{Provider: "test", ID: "upper"}, URL: upstream.URL + "/redirect" + suffix, Body: map[string]any{"prompt": "hello"}})
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
	}
}

func testHTTPStreaming(t *testing.T, model string) {
	a := httpActivation(t)
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plugin") != "yes" {
			t.Error("request hook did not run")
		}
		_, _ = io.WriteString(w, "first")
		w.(http.Flusher).Flush()
		select {
		case <-release:
			_, _ = io.WriteString(w, "last")
		case <-r.Context().Done():
		}
	}))
	defer upstream.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := (route.HTTP{}).Open(route.WithHTTPHooks(ctx, a.HTTPHooks()), route.Route{}, &models.PreparedRequest{Model: models.ModelRef{Provider: "test", ID: model}, URL: upstream.URL, Body: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	first := make([]byte, 5)
	if _, err := io.ReadFull(response.Body, first); err != nil {
		t.Fatal(err)
	}
	if string(first) != "FIRST" {
		t.Fatal(string(first))
	}
	close(release)
	rest, err := io.ReadAll(response.Body)
	if err != nil || string(rest) != "LAST" {
		t.Fatalf("rest=%q err=%v", rest, err)
	}
}

func TestNativeHTTPHookPartialReadDoesNotDiscardBufferedBytes(t *testing.T) {
	a := httpActivation(t)
	request, _ := http.NewRequest("GET", "https://example.com", nil)
	response, err := a.HTTPHooks().Response(context.Background(), models.ModelRef{ID: "prefix"}, request, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("abcdef")), Header: http.Header{}})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil || string(data) != "cdef" {
		t.Fatalf("body=%q err=%v", data, err)
	}
}

type blockingBody struct {
	closed chan struct{}
	closes atomic.Int32
}

func (b *blockingBody) Read([]byte) (int, error) { <-b.closed; return 0, io.ErrClosedPipe }
func (b *blockingBody) Close() error {
	if b.closes.Add(1) == 1 {
		close(b.closed)
	}
	return nil
}

func TestNativeHTTPBodyCancellationClosesSource(t *testing.T) {
	a := httpActivation(t)
	body := &blockingBody{closed: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	request, _ := http.NewRequest("GET", "https://example.com", nil)
	response, err := a.HTTPHooks().Response(ctx, models.ModelRef{ID: "upper"}, request, &http.Response{StatusCode: 200, Body: body, Header: http.Header{}})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := response.Body.Read(make([]byte, 1)); done <- err }()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("read did not cancel")
	}
	_ = response.Body.Close()
	select {
	case <-body.closed:
	default:
		t.Fatal("source was not closed")
	}
}
