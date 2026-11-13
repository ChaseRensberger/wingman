package pluginhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/models"
	provider "github.com/chaserensberger/wingman/models/providers"
	sdk "github.com/chaserensberger/wingman/plugin"
	"github.com/chaserensberger/wingman/plugin/protocol"
	"github.com/chaserensberger/wingman/tool"
)

func TestRuntimeRegressionPluginProcess(t *testing.T) {
	if os.Getenv("WINGMAN_RUNTIME_REGRESSION") != "1" {
		return
	}
	err := sdk.Serve(context.Background(), sdk.Definition{ID: "test/regression", Setup: func(_ context.Context, host *sdk.Host, config map[string]any) (*sdk.Extension, error) {
		return &sdk.Extension{
			Close: func(ctx context.Context) error {
				marker, _ := config["marker"].(string)
				if marker == "" {
					return nil
				}
				select {
				case <-ctx.Done():
					_ = os.WriteFile(marker, []byte("cancelled"), 0600)
					return ctx.Err()
				case <-time.After(50 * time.Millisecond):
					if err := os.WriteFile(marker, []byte("flushed"), 0600); err != nil {
						return err
					}
				}
				if err := host.Call(ctx, "cleanup.flush", nil, nil); err != nil {
					return err
				}
				if config["fail_close"] == true {
					return errors.New("flush failed")
				}
				return nil
			},
			OnEvent: func(context.Context, protocol.Event) error { return nil },
			Tools: []sdk.Tool{{Tool: protocol.Tool{Name: "burst", Description: "Report a burst of progress", InputSchema: map[string]any{"type": "object"}}, Execute: func(ctx context.Context, inv tool.Invocation) (tool.Result, error) {
				for range 200 {
					inv.Progress.Report("x", nil)
				}
				return tool.Result{Text: "done"}, nil
			}}},
			Providers: []sdk.Provider{{Provider: protocol.Provider{Meta: provider.ProviderMeta{ID: "regression", Name: "Regression"}, Models: map[string]models.ModelInfo{"echo": {ID: "echo", Provider: "regression"}}}, Prepare: func(_ context.Context, call protocol.ProviderCall) (*models.PreparedRequest, error) {
				return &models.PreparedRequest{URL: call.Options.BaseURL, Body: map[string]any{"query": call.Options.Query}}, nil
			}, Stream: func(_ context.Context, call protocol.ProviderCall, emit func(models.StreamPart) error) (*models.Message, error) {
				if call.Request.System == "options" {
					return &models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: call.Options.BaseURL + "|" + call.Options.Query["tenant"]}}, FinishReason: models.FinishReasonStop}, nil
				}
				if err := emit(models.TextStartPart{ID: "text"}); err != nil {
					return nil, err
				}
				for range 100 {
					if err := emit(models.TextDeltaPart{ID: "text", Delta: "x"}); err != nil {
						return nil, err
					}
				}
				return nil, errors.New("upstream failed")
			}}},
		}, nil
	}})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func regressionManager(t *testing.T, config map[string]any) *Manager {
	t.Helper()
	t.Setenv("WINGMAN_RUNTIME_REGRESSION", "1")
	root := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(Manifest{ID: "test/regression", Command: []string{executable, "-test.run=^TestRuntimeRegressionPluginProcess$"}, Config: config})
	if err := os.WriteFile(filepath.Join(root, "wingman-plugin.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	m, err := New(context.Background(), []string{root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m
}

func TestProviderFailureDrainsAcceptedParts(t *testing.T) {
	m := regressionManager(t, nil)
	a, err := m.Acquire(protocol.Identity{})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close(context.Background())
	client := a.Providers()[0].NewClient(provider.Credential{}, provider.ProviderOptions{})
	stream, err := client.Stream(t.Context(), models.Request{Model: models.ModelRef{Provider: "regression", ID: "echo"}})
	if err != nil {
		t.Fatal(err)
	}
	// Hold the consumer until the terminal error arrives behind a full delivery buffer.
	time.Sleep(100 * time.Millisecond)
	count := 0
	for part := range stream.Iter() {
		if _, ok := part.(models.TextDeltaPart); ok {
			count++
		}
	}
	_, err = stream.Final()
	if count != 100 || err == nil || !strings.Contains(err.Error(), "upstream failed") {
		t.Fatalf("deltas=%d, error=%v", count, err)
	}
}

func TestGracefulShutdownFlushesAndReportsErrors(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "closed")
			m := regressionManager(t, map[string]any{"marker": marker, "fail_close": fail})
			flushed := make(chan struct{}, 1)
			m.SetHostHandler(func(_ context.Context, method string, _ json.RawMessage) (any, error) {
				if method != "cleanup.flush" {
					return nil, fmt.Errorf("unexpected callback %q", method)
				}
				flushed <- struct{}{}
				return nil, nil
			})
			err := m.Close()
			select {
			case <-flushed:
			default:
				t.Errorf("shutdown did not finish its host callback: %v", err)
			}
			data, readErr := os.ReadFile(marker)
			if readErr != nil || string(data) != "flushed" {
				t.Fatalf("cleanup=%q, read=%v, close=%v", data, readErr, err)
			}
			if fail && (err == nil || !strings.Contains(err.Error(), "flush failed")) {
				t.Fatalf("cleanup error lost: %v", err)
			}
			if !fail && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProgressDoesNotBlockObserverResponses(t *testing.T) {
	m := regressionManager(t, nil)
	a, err := m.Acquire(protocol.Identity{})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close(context.Background())
	queue := make(chan string, 64)
	blocked := make(chan struct{})
	finished := make(chan error, 1)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	go func() {
		count := 0
		_, err := a.Tools()[0].Execute(ctx, tool.Invocation{Input: map[string]any{}, Progress: tool.NewProgress(func(delta string, _ map[string]any) {
			count++
			if count == 65 {
				close(blocked)
			}
			select {
			case queue <- delta:
			case <-ctx.Done():
			}
		})})
		finished <- err
	}()
	select {
	case <-blocked:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	<-queue
	sink := &externalSink{plugin: &externalPlugin{activation: a, plugin: a.plugins[0]}}
	start := time.Now()
	sink.OnEvent(run.MessageEvent{})
	elapsed := time.Since(start)
	go func() {
		for {
			select {
			case <-queue:
			case <-ctx.Done():
				return
			}
		}
	}()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	cancel()
	if elapsed >= 900*time.Millisecond {
		t.Fatalf("no-op observer stalled for %v", elapsed)
	}
}

func TestExecutableProviderReceivesRoutingOptions(t *testing.T) {
	m := regressionManager(t, nil)
	a, err := m.Acquire(protocol.Identity{})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close(context.Background())
	options := provider.ProviderOptions{BaseURL: "https://gateway.example/v1", Query: map[string]string{"tenant": "private"}}
	registry, err := provider.NewRegistry(map[string]provider.ProviderConfig{"regression": {Options: options}})
	if err != nil {
		t.Fatal(err)
	}
	registry, err = registry.WithRegistrations(a.Providers())
	if err != nil {
		t.Fatal(err)
	}
	client := registry.NewClient(nil)
	request := models.Request{Model: models.ModelRef{Provider: "regression", ID: "echo"}, System: "options"}
	prepared, err := client.Prepare(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.URL != options.BaseURL || prepared.Body["query"].(map[string]any)["tenant"] != "private" {
		t.Fatalf("prepared=%+v", prepared)
	}
	message, err := client.Generate(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if message.Content[0].(models.TextPart).Text != options.BaseURL+"|private" {
		t.Fatalf("message=%+v", message)
	}
}
