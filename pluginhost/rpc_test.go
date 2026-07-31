package pluginhost

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRPCProcessHelper(t *testing.T) {
	if os.Getenv("GO_WANT_RPC_HELPER") != "1" {
		return
	}

	dec := json.NewDecoder(bufio.NewReader(os.Stdin))
	enc := json.NewEncoder(os.Stdout)
	read := func() map[string]any {
		var message map[string]any
		if err := dec.Decode(&message); err != nil {
			os.Exit(0)
		}
		return message
	}
	respond := func(request map[string]any, result any) {
		_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": request["id"], "result": result})
	}
	drain := func() {
		for {
			request := read()
			if request["method"] == ShutdownMethod {
				respond(request, map[string]any{"message": "bye"})
				os.Exit(0)
			}
		}
	}

	switch os.Getenv("RPC_HELPER_SCENARIO") {
	case "out-of-order":
		first, second := read(), read()
		respond(second, second["method"])
		respond(first, first["method"])
		drain()
	case "cancel":
		request := read()
		cancel := read()
		if cancel["method"] != CancelRequestMethod {
			os.Exit(2)
		}
		params, _ := cancel["params"].(map[string]any)
		if params["id"] != request["id"] {
			os.Exit(2)
		}
		alive := read()
		respond(alive, "alive")
		drain()
	case "progress":
		request := read()
		_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "method": ToolProgressMethod, "params": map[string]any{"request_id": request["id"], "output_delta": "halfway", "metadata": map[string]any{"percent": 50}}})
		respond(request, "done")
		drain()
	case "diagnostics":
		for i := 0; i < diagnosticsMaxLines+20; i++ {
			fmt.Fprintf(os.Stderr, "stderr-%03d\n", i)
			_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "method": PluginLogMethod, "params": map[string]any{"level": "info", "message": fmt.Sprintf("log-%03d", i), "fields": map[string]any{"index": i}}})
		}
		fmt.Fprintln(os.Stderr, "stderr-tail")
		request := read()
		respond(request, "done")
		drain()
	case "exit":
		read()
		os.Exit(3)
	case "shutdown":
		request := read()
		if request["method"] != ShutdownMethod {
			os.Exit(2)
		}
		respond(request, map[string]any{"message": "bye"})
		time.Sleep(10 * time.Millisecond)
		os.Exit(0)
	default:
		os.Exit(2)
	}
}

func TestRPCConcurrentOutOfOrderResponses(t *testing.T) {
	c := startTestRPC(t, "out-of-order")
	defer closeTestRPC(t, c)

	type outcome struct {
		value string
		err   error
	}
	results := make(chan outcome, 2)
	for _, method := range []string{"first", "second"} {
		go func() {
			var value string
			err := c.call(context.Background(), method, nil, &value)
			results <- outcome{value: value, err: err}
		}()
	}
	got := map[string]bool{}
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		got[result.value] = true
	}
	if !got["first"] || !got["second"] {
		t.Fatalf("responses = %#v", got)
	}
}

func TestRPCCancellationKeepsProcessAlive(t *testing.T) {
	c := startTestRPC(t, "cancel")
	defer closeTestRPC(t, c)
	c.SetCapabilities([]string{CapabilityCancellation})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.call(ctx, "slow", nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled call error = %v", err)
	}
	var result string
	if err := c.call(context.Background(), "alive", nil, &result); err != nil {
		t.Fatal(err)
	}
	if result != "alive" {
		t.Fatalf("result = %q", result)
	}
	select {
	case <-c.Done():
		t.Fatalf("plugin terminated: %v", c.Err())
	default:
	}
}

func TestRPCProgressRouting(t *testing.T) {
	c := startTestRPC(t, "progress")
	defer closeTestRPC(t, c)

	progress := make(chan ToolProgressParams, 1)
	var result string
	if err := c.callWithProgress(context.Background(), "tool.execute", nil, &result, func(value ToolProgressParams) { progress <- value }); err != nil {
		t.Fatal(err)
	}
	if result != "done" {
		t.Fatalf("result = %q", result)
	}
	select {
	case value := <-progress:
		if value.OutputDelta != "halfway" || value.RequestID == 0 || value.Metadata["percent"] != float64(50) {
			t.Fatalf("progress = %#v", value)
		}
	case <-time.After(time.Second):
		t.Fatal("no progress notification")
	}
}

func TestRPCDiagnosticsAreBounded(t *testing.T) {
	c := startTestRPC(t, "diagnostics")
	defer closeTestRPC(t, c)

	var result string
	if err := c.call(context.Background(), "diagnostics", nil, &result); err != nil {
		t.Fatal(err)
	}
	if result != "done" {
		t.Fatalf("result = %q", result)
	}
	deadline := time.Now().Add(time.Second)
	for len(c.Diagnostics()) < diagnosticsMaxLines && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	diagnostics := c.Diagnostics()
	if len(diagnostics) > diagnosticsMaxLines {
		t.Fatalf("diagnostic count = %d", len(diagnostics))
	}
	var bytes int
	for _, diagnostic := range diagnostics {
		bytes += len(diagnostic.Source) + len(diagnostic.Level) + len(diagnostic.Message)
		if diagnostic.Fields != nil {
			encoded, _ := json.Marshal(diagnostic.Fields)
			bytes += len(encoded)
		}
	}
	if bytes > diagnosticsMaxBytes {
		t.Fatalf("diagnostics size = %d", bytes)
	}
	if len(diagnostics) == 0 {
		t.Fatal("no diagnostics retained")
	}
}

func TestRPCProcessExitFailsPendingCalls(t *testing.T) {
	c := startTestRPC(t, "exit")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := c.call(ctx, "will-exit", nil, nil)
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("pending call error = %v", err)
	}
	select {
	case <-c.Done():
	case <-time.After(time.Second):
		t.Fatal("client did not terminate")
	}
}

func TestRPCGracefulShutdown(t *testing.T) {
	c := startTestRPC(t, "shutdown")
	c.SetCapabilities(nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.Close(ctx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close() error = %v", err)
	}
	if err := c.Close(context.Background()); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second Close() error = %v", err)
	}
}

func startTestRPC(t *testing.T, scenario string) *rpcClient {
	t.Helper()
	t.Setenv("GO_WANT_RPC_HELPER", "1")
	t.Setenv("RPC_HELPER_SCENARIO", scenario)
	c, err := startRPC(context.Background(), []string{os.Args[0], "-test.run=^TestRPCProcessHelper$"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func closeTestRPC(t *testing.T, c *rpcClient) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.Close(ctx); err != nil && !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("Close() error = %v", err)
	}
}
