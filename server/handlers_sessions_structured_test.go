package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/chaserensberger/wingman/agent/loop/looptest"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/store"
)

var (
	testModelMu     sync.Mutex
	testModelFactory func() models.Model
)

func init() {
	provider.Register(provider.ProviderMeta{
		ID:        "test-structured",
		Name:      "Test Structured Output Provider",
		AuthTypes: []provider.AuthType{},
		Factory: func(opts map[string]any) (models.Model, error) {
			testModelMu.Lock()
			defer testModelMu.Unlock()
			if testModelFactory == nil {
				return nil, fmt.Errorf("no test model factory set")
			}
			return testModelFactory(), nil
		},
	})
}

func TestPostMessageWithOutputSchemaEmitsStructuredOutputEvent(t *testing.T) {
	ts, _ := setupTestServer(t)

	model := looptest.NewRecordingModel(looptest.Reply(`{"name":"Alice"}`))
	model.SetInfo(models.ModelInfo{
		Provider:     "test-structured",
		ID:           "fake-model",
		Capabilities: models.ModelCapabilities{StructuredOutput: true},
	})

	testModelMu.Lock()
	testModelFactory = func() models.Model { return model }
	testModelMu.Unlock()
	defer func() {
		testModelMu.Lock()
		testModelFactory = nil
		testModelMu.Unlock()
	}()

	// Create agent.
	agentBody := mustJSON(t, map[string]any{
		"name":         "structured-agent",
		"instructions": "You are a test agent.",
		"provider":     "test-structured",
		"model":        "fake-model",
	})
	resp, err := http.Post(ts.URL+"/agents", "application/json", bytes.NewReader(agentBody))
	if err != nil {
		t.Fatalf("create agent failed: %v", err)
	}
	var agent store.Agent
	decodeJSON(t, resp, &agent)

	// Create session.
	sessBody := mustJSON(t, map[string]any{"working_directory": "/tmp"})
	resp, err = http.Post(ts.URL+"/sessions", "application/json", bytes.NewReader(sessBody))
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	var sess store.Session
	decodeJSON(t, resp, &sess)

	// Stream message with output_schema.
	msgBody := mustJSON(t, map[string]any{
		"agent_id": agent.ID,
		"message":  "give me a name",
		"output_schema": map[string]any{
			"name": "ContactInfo",
			"schema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
				},
				"required": []any{"name"},
			},
		},
	})

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/sessions/"+sess.ID+"/message/stream", bytes.NewReader(msgBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	scanner := makeSSEScanner(resp)
	var found bool
	for scanner.Scan() {
		ev := scanner.Event()
		if ev.Type == "structured_output" {
			found = true
			payload, ok := ev.Data["data"].(map[string]any)
			if !ok {
				t.Fatalf("expected data payload map, got %T", ev.Data["data"])
			}
			parsed, ok := payload["parsed"].(map[string]any)
			if !ok {
				t.Fatalf("expected parsed object, got %T", payload["parsed"])
			}
			if parsed["name"] != "Alice" {
				t.Errorf("expected parsed.name = Alice, got %v", parsed["name"])
			}
			if payload["schema"] != "ContactInfo" {
				t.Errorf("expected schema = ContactInfo, got %v", payload["schema"])
			}
			if rawJSON, ok := payload["raw_json"].(string); !ok || rawJSON != `{"name":"Alice"}` {
				t.Errorf("expected raw_json = {\"name\":\"Alice\"}, got %v", payload["raw_json"])
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("sse scanner error: %v", err)
	}
	if !found {
		t.Fatal("expected structured_output event in SSE stream")
	}
}

func TestPostMessageWithoutOutputSchemaUnchanged(t *testing.T) {
	ts, _ := setupTestServer(t)

	model := looptest.NewRecordingModel(looptest.Reply("hello"))
	model.SetInfo(models.ModelInfo{
		Provider:     "test-structured",
		ID:           "fake-model",
		Capabilities: models.ModelCapabilities{StructuredOutput: true},
	})

	testModelMu.Lock()
	testModelFactory = func() models.Model { return model }
	testModelMu.Unlock()
	defer func() {
		testModelMu.Lock()
		testModelFactory = nil
		testModelMu.Unlock()
	}()

	agentBody := mustJSON(t, map[string]any{
		"name":         "normal-agent",
		"instructions": "You are a test agent.",
		"provider":     "test-structured",
		"model":        "fake-model",
	})
	resp, err := http.Post(ts.URL+"/agents", "application/json", bytes.NewReader(agentBody))
	if err != nil {
		t.Fatalf("create agent failed: %v", err)
	}
	var agent store.Agent
	decodeJSON(t, resp, &agent)

	sessBody := mustJSON(t, map[string]any{"working_directory": "/tmp"})
	resp, err = http.Post(ts.URL+"/sessions", "application/json", bytes.NewReader(sessBody))
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	var sess store.Session
	decodeJSON(t, resp, &sess)

	msgBody := mustJSON(t, map[string]any{
		"agent_id": agent.ID,
		"message":  "hi",
	})

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/sessions/"+sess.ID+"/message/stream", bytes.NewReader(msgBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	scanner := makeSSEScanner(resp)
	var foundDone bool
	for scanner.Scan() {
		if scanner.Event().Type == "done" {
			foundDone = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("sse scanner error: %v", err)
	}
	if !foundDone {
		t.Fatal("expected done event in SSE stream")
	}
}

// sseEvent is a parsed SSE event.
type sseEvent struct {
	Type string
	Data map[string]any
}

// sseScanner reads text/event-stream responses line-by-line and yields
// parsed {event,data} pairs.
type sseScanner struct {
	scanner *bufio.Scanner
	current sseEvent
	err     error
}

func makeSSEScanner(resp *http.Response) *sseScanner {
	return &sseScanner{scanner: bufio.NewScanner(resp.Body)}
}

func (s *sseScanner) Scan() bool {
	if !s.scanner.Scan() {
		return false
	}
	line := s.scanner.Text()
	if !strings.HasPrefix(line, "event: ") {
		return s.Scan()
	}
	s.current.Type = strings.TrimPrefix(line, "event: ")
	if !s.scanner.Scan() {
		s.err = fmt.Errorf("expected data line after event: %s", s.current.Type)
		return false
	}
	dataLine := s.scanner.Text()
	if !strings.HasPrefix(dataLine, "data: ") {
		s.err = fmt.Errorf("expected data: prefix, got %q", dataLine)
		return false
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(strings.TrimPrefix(dataLine, "data: ")), &envelope); err != nil {
		s.err = fmt.Errorf("unmarshal envelope: %w", err)
		return false
	}
	s.current.Data = envelope
	return true
}

func (s *sseScanner) Event() sseEvent { return s.current }
func (s *sseScanner) Err() error      { return s.err }
