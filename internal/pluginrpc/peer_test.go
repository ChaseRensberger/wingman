package pluginrpc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/models"
)

func TestPeerConcurrentCallbackAndCancellation(t *testing.T) {
	left, right := net.Pipe()
	t.Cleanup(func() { _ = left.Close(); _ = right.Close() })
	started := make(chan struct{})
	stopped := make(chan struct{})
	var server *Peer
	client := New(left, func(ctx context.Context, method string, input json.RawMessage) (any, error) { return "callback", nil })
	server = New(right, func(ctx context.Context, method string, input json.RawMessage) (any, error) {
		if method == "wait" {
			close(started)
			<-ctx.Done()
			close(stopped)
			return nil, ctx.Err()
		}
		var result string
		err := server.Call(ctx, "identity", nil, &result)
		return result, err
	})
	go func() { _ = client.Serve(t.Context(), left) }()
	go func() { _ = server.Serve(t.Context(), right) }()
	var result string
	if err := client.Call(t.Context(), "callback", nil, &result); err != nil || result != "callback" {
		t.Fatalf("nested call = %q, %v", result, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	returned := make(chan error, 1)
	go func() { returned <- client.Call(ctx, "wait", nil, nil) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("call did not start")
	}
	cancel()
	select {
	case err := <-returned:
		if err != context.Canceled {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("caller did not cancel")
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("handler did not cancel")
	}
	if err := client.Call(t.Context(), "callback", nil, &result); err != nil {
		t.Fatal(err)
	}
}

func TestPeerEOFStopsNotificationHandlers(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	reader, writer := io.Pipe()
	peer := New(io.Discard, func(ctx context.Context, method string, input json.RawMessage) (any, error) {
		close(started)
		<-ctx.Done()
		close(stopped)
		return nil, nil
	})
	returned := make(chan error, 1)
	go func() { returned <- peer.Serve(t.Context(), reader) }()
	if _, err := io.WriteString(writer, "{\"jsonrpc\":\"2.0\",\"method\":\"notice\"}\n"); err != nil {
		t.Fatal(err)
	}
	<-started
	_ = writer.Close()
	if err := <-returned; err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("notification survived EOF")
	}
	peer.Wait()
}

func TestPeerRejectsInvalidAndOversizedMessages(t *testing.T) {
	for _, input := range []string{"not JSON\n", "{\"jsonrpc\":\"1.0\"}\n", strings.Repeat("x", maxMessage+1)} {
		peer := New(io.Discard, nil)
		if err := peer.Serve(t.Context(), strings.NewReader(input)); err == nil {
			t.Fatal("invalid message was accepted")
		}
	}
}

func TestPeerPreservesProviderFailuresInBothDirections(t *testing.T) {
	left, right := net.Pipe()
	t.Cleanup(func() { _ = left.Close(); _ = right.Close() })
	failure := &models.ProviderError{Category: models.ErrorInvalidRequest, Classification: "context-overflow", Provider: "native", RequestID: "req-native", Message: "context too long"}
	var server *Peer
	client := New(left, func(context.Context, string, json.RawMessage) (any, error) { return nil, failure })
	server = New(right, func(ctx context.Context, method string, input json.RawMessage) (any, error) {
		if method == "callback" {
			return nil, server.Call(ctx, "provider", nil, nil)
		}
		return nil, failure
	})
	go func() { _ = client.Serve(t.Context(), left) }()
	go func() { _ = server.Serve(t.Context(), right) }()
	for _, method := range []string{"provider", "callback"} {
		err := client.Call(t.Context(), method, nil, nil)
		var restored *models.ProviderError
		if !errors.As(err, &restored) || restored.Classification != failure.Classification || restored.RequestID != failure.RequestID {
			t.Fatalf("%s failure = %#v, %v", method, restored, err)
		}
	}
}
