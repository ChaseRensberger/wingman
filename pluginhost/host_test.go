package pluginhost

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/plugin/protocol"
)

func TestHostCallRequiresOwnedInvocationOrBackgroundGrant(t *testing.T) {
	var output bytes.Buffer
	owner := WithIdentity(context.Background(), protocol.Identity{ClientID: "owner"})
	client := &rpcClient{processCtx: context.Background(), enc: json.NewEncoder(&output), pending: map[int64]*pendingCall{7: {ctx: owner}}, bindings: map[string]context.Context{}}
	client.hostHandler = func(ctx context.Context, method string, input json.RawMessage) (any, error) {
		if method == "identity" {
			return Identity(ctx).ClientID, nil
		}
		return nil, nil
	}
	call := func(invocation int64, token, method string) rpcEnvelope {
		t.Helper()
		output.Reset()
		input, _ := json.Marshal(protocol.HostCall{InvocationID: invocation, ScopeToken: token, Method: method})
		client.handleHostCall(context.Background(), rpcEnvelope{ID: json.RawMessage(`1`), Params: input})
		var response rpcEnvelope
		if err := json.Unmarshal(output.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response
	}
	for _, response := range []rpcEnvelope{call(0, "", "identity"), call(99, "", "identity"), call(0, "invented", "identity")} {
		if response.Error == nil {
			t.Fatal("unowned callback succeeded")
		}
	}
	response := call(7, "", "scope.bind")
	var token string
	if err := json.Unmarshal(response.Result, &token); err != nil {
		t.Fatal(err)
	}
	delete(client.pending, 7)
	response = call(0, token, "identity")
	if response.Error != nil || string(response.Result) != `"owner"` {
		t.Fatalf("background scope = %#v", response)
	}
	response = call(0, token, "empty")
	if response.Error != nil || string(response.Result) != "null" {
		t.Fatalf("nil result must be explicit JSON null: %#v", response)
	}
	lifetime := client.bindings[token].Value(scopeGrantKey{}).(*scopeGrant).ctx
	if response := call(0, token, "scope.release"); response.Error != nil {
		t.Fatal(response.Error)
	}
	if lifetime.Err() == nil {
		t.Fatal("released grant retained its subscriptions' lifetime")
	}
	if response := call(0, token, "identity"); response.Error == nil {
		t.Fatal("released scope remained usable")
	}
}

func TestHTTPSourceGrantIsLimitedToItsBody(t *testing.T) {
	var output bytes.Buffer
	bodyCtx := context.WithValue(t.Context(), httpBodyKey{}, io.NopCloser(strings.NewReader("source")))
	owner := WithIdentity(context.WithValue(t.Context(), httpBodyLifetimeKey{}, bodyCtx), protocol.Identity{ClientID: "client"})
	client := &rpcClient{processCtx: t.Context(), enc: json.NewEncoder(&output), pending: map[int64]*pendingCall{7: {ctx: owner}}, bindings: map[string]context.Context{}}
	client.hostHandler = bindHost("plugin", func(context.Context, string, json.RawMessage) (any, error) {
		t.Fatal("body grant reached general host API")
		return nil, nil
	})
	call := func(invocation int64, token, method string) rpcEnvelope {
		t.Helper()
		output.Reset()
		input, _ := json.Marshal(protocol.HostCall{InvocationID: invocation, ScopeToken: token, Method: method})
		client.handleHostCall(t.Context(), rpcEnvelope{ID: json.RawMessage(`1`), Params: input})
		var response rpcEnvelope
		if err := json.Unmarshal(output.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response
	}
	response := call(7, "", "http.body.bind")
	var token string
	if response.Error != nil || json.Unmarshal(response.Result, &token) != nil {
		t.Fatalf("bind: %+v", response)
	}
	delete(client.pending, 7)
	if response := call(0, token, "api.request"); response.Error == nil {
		t.Fatal("HTTP grant allowed API access")
	}
	response = call(0, token, "http.body.read")
	var chunk protocol.BodyChunk
	if response.Error != nil || json.Unmarshal(response.Result, &chunk) != nil || string(chunk.Data) != "source" {
		t.Fatalf("source read: %+v", response)
	}
	if response := call(0, token, "scope.release"); response.Error != nil {
		t.Fatal(response.Error)
	}
	if response := call(0, token, "http.body.read"); response.Error == nil {
		t.Fatal("released source remained readable")
	}
}

func TestHTTPBodyReadUsesExchangeDeadline(t *testing.T) {
	deadline := time.Now().Add(time.Minute)
	owner, cancel := context.WithDeadline(t.Context(), deadline)
	defer cancel()
	var output bytes.Buffer
	client := &rpcClient{processCtx: t.Context(), enc: json.NewEncoder(&output), pending: map[int64]*pendingCall{7: {ctx: owner}}}
	client.hostHandler = func(ctx context.Context, _ string, _ json.RawMessage) (any, error) {
		if got, ok := ctx.Deadline(); !ok || !got.Equal(deadline) {
			t.Fatalf("body deadline = %v, want exchange deadline %v", got, deadline)
		}
		return protocol.BodyChunk{EOF: true}, nil
	}
	input, _ := json.Marshal(protocol.HostCall{InvocationID: 7, Method: "http.body.read"})
	client.handleHostCall(t.Context(), rpcEnvelope{ID: json.RawMessage(`1`), Params: input})
}

func TestCancelledHostCallDoesNotEnterHandler(t *testing.T) {
	var output bytes.Buffer
	owner := WithIdentity(t.Context(), protocol.Identity{ClientID: "owner"})
	client := &rpcClient{enc: json.NewEncoder(&output), pending: map[int64]*pendingCall{7: {ctx: owner}}, hostHandler: func(context.Context, string, json.RawMessage) (any, error) {
		t.Fatal("cancelled request entered handler")
		return nil, nil
	}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	input, _ := json.Marshal(protocol.HostCall{InvocationID: 7, Method: "wait"})
	client.handleHostCall(ctx, rpcEnvelope{ID: json.RawMessage(`1`), Params: input})
	var response rpcEnvelope
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error == nil {
		t.Fatal("cancelled request succeeded")
	}
}

type hostResponseWriter chan []byte

func (w hostResponseWriter) Write(data []byte) (int, error) {
	w <- append([]byte(nil), data...)
	return len(data), nil
}

func TestHostCancellationIsRegisteredBeforeDispatch(t *testing.T) {
	responses := make(hostResponseWriter, 1)
	gate := make(chan struct{})
	defer close(gate)
	owner := WithIdentity(t.Context(), protocol.Identity{ClientID: "owner"})
	client := &rpcClient{processCtx: t.Context(), enc: json.NewEncoder(responses), pending: map[int64]*pendingCall{7: {ctx: owner}}, hostCalls: map[string]context.CancelFunc{}, hostHandler: func(ctx context.Context, _ string, _ json.RawMessage) (any, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-gate:
			return nil, nil
		}
	}}
	input, _ := json.Marshal(protocol.HostCall{InvocationID: 7, Method: "wait"})
	client.dispatchHostCall(rpcEnvelope{ID: json.RawMessage(`1`), Params: input})
	client.handleNotification(CancelRequestMethod, json.RawMessage(`{"id":1}`))
	select {
	case raw := <-responses:
		var response rpcEnvelope
		if err := json.Unmarshal(raw, &response); err != nil {
			t.Fatal(err)
		}
		if response.Error == nil {
			t.Fatal("cancelled request succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("host cancellation was lost before dispatch")
	}
}
