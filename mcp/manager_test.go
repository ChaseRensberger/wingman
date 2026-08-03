package mcp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/chaserensberger/wingman/tool"
)

type fakeConnection struct {
	started chan struct{}
	release chan struct{}

	mu     sync.Mutex
	calls  int
	closed int
}

func (c *fakeConnection) CallTool(_ context.Context, _ *mcpsdk.CallToolParams) (*mcpsdk.CallToolResult, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	if c.started != nil {
		c.started <- struct{}{}
	}
	if c.release != nil {
		<-c.release
	}
	return &mcpsdk.CallToolResult{}, nil
}

func (c *fakeConnection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed++
	return nil
}

func (c *fakeConnection) counts() (calls, closed int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls, c.closed
}

func testManager(connect connector) *Manager {
	enabled := true
	return newManager(Config{Servers: map[string]ServerConfig{
		"server": {Type: "local", Command: []string{"test"}, Enabled: &enabled},
	}}, connect)
}

func testTools(description string) []*mcpsdk.Tool {
	return []*mcpsdk.Tool{{Name: "run", Description: description, InputSchema: map[string]any{"type": "object"}}}
}

func currentTool(t *testing.T, m *Manager) *mcpTool {
	t.Helper()
	tools := m.Tools()
	if len(tools) != 1 {
		t.Fatalf("tool count = %d, want 1", len(tools))
	}
	return tools[0].(*mcpTool)
}

func TestConnectFailurePreservesPublishedGeneration(t *testing.T) {
	old := &fakeConnection{}
	connects := 0
	m := testManager(func(context.Context, string, ServerConfig) (connection, []*mcpsdk.Tool, error) {
		connects++
		if connects == 1 {
			return old, testTools("old"), nil
		}
		return nil, nil, errors.New("candidate failed")
	})
	if err := m.Connect(context.Background(), "server"); err != nil {
		t.Fatal(err)
	}
	oldTool := currentTool(t, m)
	if err := m.Connect(context.Background(), "server"); err == nil {
		t.Fatal("Connect succeeded after failed candidate")
	}
	if _, err := oldTool.Execute(context.Background(), tool.Invocation{}); err != nil {
		t.Fatalf("old tool execution: %v", err)
	}
	if got := currentTool(t, m).Description(); got != "old" {
		t.Fatalf("published tool = %q, want old", got)
	}
	status := m.Status()[0]
	if status.Status != "connected" || status.Error != "candidate failed" {
		t.Fatalf("status = %#v, want connected with candidate error", status)
	}
}

func TestReplacementRejectsStaleToolsAndDrainsCalls(t *testing.T) {
	old := &fakeConnection{started: make(chan struct{}, 1), release: make(chan struct{})}
	new := &fakeConnection{}
	connects := 0
	m := testManager(func(context.Context, string, ServerConfig) (connection, []*mcpsdk.Tool, error) {
		connects++
		if connects == 1 {
			return old, testTools("old"), nil
		}
		return new, testTools("new"), nil
	})
	if err := m.Connect(context.Background(), "server"); err != nil {
		t.Fatal(err)
	}
	oldTool := currentTool(t, m)
	callDone := make(chan error, 1)
	go func() { _, err := oldTool.Execute(context.Background(), tool.Invocation{}); callDone <- err }()
	<-old.started

	replaceDone := make(chan error, 1)
	go func() { replaceDone <- m.Connect(context.Background(), "server") }()
	waitFor(t, func() bool { return currentTool(t, m).Description() == "new" })
	if _, err := oldTool.Execute(context.Background(), tool.Invocation{}); err == nil {
		t.Fatal("stale tool execution succeeded")
	}
	select {
	case err := <-replaceDone:
		t.Fatalf("replacement returned before active call drained: %v", err)
	default:
	}
	close(old.release)
	if err := <-callDone; err != nil {
		t.Fatalf("active old call: %v", err)
	}
	if err := <-replaceDone; err != nil {
		t.Fatalf("replacement: %v", err)
	}
	if _, closed := old.counts(); closed != 1 {
		t.Fatalf("old close count = %d, want 1", closed)
	}
	if _, err := currentTool(t, m).Execute(context.Background(), tool.Invocation{}); err != nil {
		t.Fatalf("new tool execution: %v", err)
	}
}

func TestConcurrentConnectsPublishNewestRequest(t *testing.T) {
	firstStarted := make(chan struct{})
	firstRelease := make(chan struct{})
	secondStarted := make(chan struct{})
	old := &fakeConnection{}
	first := &fakeConnection{}
	second := &fakeConnection{}
	connects := 0
	m := testManager(func(context.Context, string, ServerConfig) (connection, []*mcpsdk.Tool, error) {
		connects++
		switch connects {
		case 1:
			return old, testTools("old"), nil
		case 2:
			close(firstStarted)
			<-firstRelease
			return first, testTools("first"), nil
		default:
			close(secondStarted)
			return second, testTools("second"), nil
		}
	})
	if err := m.Connect(context.Background(), "server"); err != nil {
		t.Fatal(err)
	}
	firstDone := make(chan error, 1)
	go func() { firstDone <- m.Connect(context.Background(), "server") }()
	<-firstStarted
	secondDone := make(chan error, 1)
	go func() { secondDone <- m.Connect(context.Background(), "server") }()
	select {
	case <-secondStarted:
		t.Fatal("second connection started before first request completed")
	case <-time.After(20 * time.Millisecond):
	}
	close(firstRelease)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	<-secondStarted
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	if got := currentTool(t, m).Description(); got != "second" {
		t.Fatalf("published tool = %q, want second", got)
	}
}

func TestCloseRetiresPublishedGeneration(t *testing.T) {
	conn := &fakeConnection{}
	m := testManager(func(context.Context, string, ServerConfig) (connection, []*mcpsdk.Tool, error) {
		return conn, testTools("current"), nil
	})
	if err := m.Connect(context.Background(), "server"); err != nil {
		t.Fatal(err)
	}
	stale := currentTool(t, m)
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := stale.Execute(context.Background(), tool.Invocation{}); err == nil {
		t.Fatal("stale tool execution succeeded after Close")
	}
	if err := m.Connect(context.Background(), "server"); err == nil {
		t.Fatal("Connect succeeded after Close")
	}
	if _, closed := conn.counts(); closed != 1 {
		t.Fatalf("close count = %d, want 1", closed)
	}
}

func TestDisconnectRejectsStaleToolsAndDrainsCalls(t *testing.T) {
	conn := &fakeConnection{started: make(chan struct{}, 1), release: make(chan struct{})}
	m := testManager(func(context.Context, string, ServerConfig) (connection, []*mcpsdk.Tool, error) {
		return conn, testTools("current"), nil
	})
	if err := m.Connect(context.Background(), "server"); err != nil {
		t.Fatal(err)
	}
	stale := currentTool(t, m)
	callDone := make(chan error, 1)
	go func() { _, err := stale.Execute(context.Background(), tool.Invocation{}); callDone <- err }()
	<-conn.started

	disconnectDone := make(chan error, 1)
	go func() { disconnectDone <- m.Disconnect("server") }()
	waitFor(t, func() bool { return len(m.Tools()) == 0 })
	if _, err := stale.Execute(context.Background(), tool.Invocation{}); err == nil {
		t.Fatal("stale tool execution succeeded")
	}
	select {
	case err := <-disconnectDone:
		t.Fatalf("disconnect returned before active call drained: %v", err)
	default:
	}
	close(conn.release)
	if err := <-callDone; err != nil {
		t.Fatalf("active call: %v", err)
	}
	if err := <-disconnectDone; err != nil {
		t.Fatal(err)
	}
	if _, closed := conn.counts(); closed != 1 {
		t.Fatalf("close count = %d, want 1", closed)
	}
}

func TestCloseContextUsesOneDeadlineForAllConnections(t *testing.T) {
	enabled := true
	connections := map[string]*fakeConnection{
		"first":  {started: make(chan struct{}, 1), release: make(chan struct{})},
		"second": {started: make(chan struct{}, 1), release: make(chan struct{})},
	}
	m := newManager(Config{Servers: map[string]ServerConfig{
		"first":  {Type: "local", Command: []string{"test"}, Enabled: &enabled},
		"second": {Type: "local", Command: []string{"test"}, Enabled: &enabled},
	}}, func(_ context.Context, name string, _ ServerConfig) (connection, []*mcpsdk.Tool, error) {
		return connections[name], testTools(name), nil
	})
	for name := range connections {
		if err := m.Connect(context.Background(), name); err != nil {
			t.Fatal(err)
		}
	}
	callDone := make(chan error, len(connections))
	for _, candidate := range m.Tools() {
		candidate := candidate
		go func() { _, err := candidate.Execute(context.Background(), tool.Invocation{}); callDone <- err }()
	}
	for _, conn := range connections {
		<-conn.started
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	start := time.Now()
	err := m.CloseContext(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("CloseContext error = %v, want deadline", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("CloseContext took %s for one shared deadline", elapsed)
	}
	for _, conn := range connections {
		if _, closed := conn.counts(); closed != 1 {
			t.Fatalf("connection close count = %d, want 1", closed)
		}
		close(conn.release)
	}
	for range connections {
		if err := <-callDone; err != nil {
			t.Fatal(err)
		}
	}
}

func TestToolExecutionUsesConfiguredTimeout(t *testing.T) {
	enabled := true
	conn := &timeoutConnection{}
	m := newManager(Config{Servers: map[string]ServerConfig{
		"server": {Type: "local", Command: []string{"test"}, Enabled: &enabled, ExecutionTimeout: 10},
	}}, func(_ context.Context, _ string, _ ServerConfig) (connection, []*mcpsdk.Tool, error) {
		return conn, testTools("current"), nil
	})
	if err := m.Connect(context.Background(), "server"); err != nil {
		t.Fatal(err)
	}
	if _, err := currentTool(t, m).Execute(context.Background(), tool.Invocation{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("tool execution error = %v, want deadline exceeded", err)
	}
}

type timeoutConnection struct{}

func (timeoutConnection) CallTool(ctx context.Context, _ *mcpsdk.CallToolParams) (*mcpsdk.CallToolResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (timeoutConnection) Close() error { return nil }

func waitFor(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !ready() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for condition")
		}
		time.Sleep(time.Millisecond)
	}
}
