package session_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/agent/loop/looptest"
	"github.com/chaserensberger/wingman/agent/session"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
	"github.com/chaserensberger/wingman/tool"
)

func TestHydration(t *testing.T) {
	ctx := context.Background()
	st := memory.NewStore()

	sessRow := &store.Session{Title: "hydrate-test", WorkDir: "/tmp"}
	if err := st.CreateSession(sessRow); err != nil {
		t.Fatalf("create session: %v", err)
	}

	msg := store.StoredMessage{
		ID:        store.NewID(store.PrefixMessage),
		SessionID: sessRow.ID,
		Idx:       0,
		Role:      "user",
		CreatedAt: time.Now().UTC(),
	}
	if err := st.UpsertMessage(ctx, msg); err != nil {
		t.Fatalf("upsert message: %v", err)
	}
	part := store.StoredPart{
		ID:          store.NewID(store.PrefixPart),
		MessageID:   msg.ID,
		Sequence:    0,
		Kind:        "text",
		PayloadJSON: []byte(`{"type":"text","text":"hello"}`),
		CreatedAt:   time.Now().UTC(),
	}
	if err := st.UpsertPart(ctx, part); err != nil {
		t.Fatalf("upsert part: %v", err)
	}

	model := looptest.NewRecordingModel(looptest.Reply("world"))
	sess := session.New(
		session.WithID(sessRow.ID),
		session.WithModel(model),
		session.WithStore(st),
	)
	_, err := sess.Run(ctx, "hi")
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	hist := sess.History()
	if len(hist) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(hist))
	}
	if len(hist[0].Content) != 1 {
		t.Fatalf("expected 1 part in hydrated message, got %d", len(hist[0].Content))
	}
	if tp, ok := hist[0].Content[0].(models.TextPart); !ok || tp.Text != "hello" {
		t.Errorf("expected hydrated text 'hello', got %v", hist[0].Content[0])
	}
	if tp, ok := hist[2].Content[0].(models.TextPart); !ok || tp.Text != "world" {
		t.Errorf("expected assistant text 'world', got %v", hist[2].Content[0])
	}
}

func TestHydrationSessionNotFound(t *testing.T) {
	ctx := context.Background()
	st := memory.NewStore()

	model := looptest.NewRecordingModel(looptest.Reply("ok"))
	sess := session.New(
		session.WithID("ses_nonexistent"),
		session.WithModel(model),
		session.WithStore(st),
	)
	_, err := sess.Run(ctx, "hi")
	if err != nil {
		t.Fatalf("expected no error for missing session, got %v", err)
	}
	if len(sess.History()) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(sess.History()))
	}
}

func TestUserMessagePersistence(t *testing.T) {
	ctx := context.Background()
	st := memory.NewStore()
	sessRow := &store.Session{Title: "user-msg", WorkDir: "/tmp"}
	if err := st.CreateSession(sessRow); err != nil {
		t.Fatalf("create session: %v", err)
	}

	model := looptest.NewRecordingModel(looptest.Reply("ok"))
	sess := session.New(
		session.WithID(sessRow.ID),
		session.WithModel(model),
		session.WithStore(st),
	)
	_, err := sess.Run(ctx, "hello")
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	msgs, err := st.ListMessages(ctx, sessRow.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("expected first message role user, got %q", msgs[0].Role)
	}
	if len(msgs[0].Parts) != 1 {
		t.Fatalf("expected 1 part in user message, got %d", len(msgs[0].Parts))
	}
	if string(msgs[0].Parts[0].PayloadJSON) != `{"type":"text","text":"hello"}` {
		t.Errorf("unexpected payload: %s", string(msgs[0].Parts[0].PayloadJSON))
	}
}

func TestAssistantMessagePersistence(t *testing.T) {
	ctx := context.Background()
	st := memory.NewStore()
	sessRow := &store.Session{Title: "assistant-msg", WorkDir: "/tmp"}
	if err := st.CreateSession(sessRow); err != nil {
		t.Fatalf("create session: %v", err)
	}

	model := looptest.NewRecordingModel(
		looptest.ReplyWithToolCalls(
			looptest.ToolCall{Name: "echo", Args: map[string]any{"msg": "a"}},
			looptest.ToolCall{Name: "echo", Args: map[string]any{"msg": "b"}},
		),
		looptest.Reply("done"),
	)
	sess := session.New(
		session.WithID(sessRow.ID),
		session.WithModel(model),
		session.WithStore(st),
		session.WithTools(&echoTool{}),
	)
	_, err := sess.Run(ctx, "call echo")
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	msgs, err := st.ListMessages(ctx, sessRow.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}
	if msgs[1].Role != "assistant" {
		t.Errorf("expected assistant role, got %q", msgs[1].Role)
	}
	if len(msgs[1].Parts) != 2 {
		t.Fatalf("expected 2 parts in assistant message, got %d", len(msgs[1].Parts))
	}
	if msgs[1].Parts[0].Kind != "tool_call" {
		t.Errorf("expected first part kind tool_call, got %q", msgs[1].Parts[0].Kind)
	}
	if msgs[1].Parts[0].Sequence != 0 {
		t.Errorf("expected first part sequence 0, got %d", msgs[1].Parts[0].Sequence)
	}
	if msgs[1].Parts[1].Kind != "tool_call" {
		t.Errorf("expected second part kind tool_call, got %q", msgs[1].Parts[1].Kind)
	}
	if msgs[1].Parts[1].Sequence != 1 {
		t.Errorf("expected second part sequence 1, got %d", msgs[1].Parts[1].Sequence)
	}
}

func TestToolResultPersistence(t *testing.T) {
	ctx := context.Background()
	st := memory.NewStore()
	sessRow := &store.Session{Title: "tool-result", WorkDir: "/tmp"}
	if err := st.CreateSession(sessRow); err != nil {
		t.Fatalf("create session: %v", err)
	}

	model := looptest.NewRecordingModel(
		looptest.ReplyWithTool("echo", `{"msg":"ping"}`),
		looptest.Reply("done"),
	)
	sess := session.New(
		session.WithID(sessRow.ID),
		session.WithModel(model),
		session.WithStore(st),
		session.WithTools(&echoTool{}),
	)
	_, err := sess.Run(ctx, "call echo")
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	msgs, err := st.ListMessages(ctx, sessRow.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}
	if msgs[2].Role != "tool" {
		t.Errorf("expected tool role, got %q", msgs[2].Role)
	}
	if len(msgs[2].Parts) != 1 {
		t.Fatalf("expected 1 part in tool result, got %d", len(msgs[2].Parts))
	}
	if msgs[2].Parts[0].Kind != "tool_result" {
		t.Errorf("expected tool_result kind, got %q", msgs[2].Parts[0].Kind)
	}
}

func TestNilStore(t *testing.T) {
	ctx := context.Background()

	model := looptest.NewRecordingModel(looptest.Reply("ok"))
	sess := session.New(
		session.WithModel(model),
	)
	_, err := sess.Run(ctx, "hello")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(sess.History()) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(sess.History()))
	}
}

func TestErrorPropagation(t *testing.T) {
	ctx := context.Background()
	st := &errorStore{err: errors.New("upsert failed")}

	model := looptest.NewRecordingModel(looptest.Reply("ok"))
	sess := session.New(
		session.WithID("ses_test"),
		session.WithModel(model),
		session.WithStore(st),
	)
	_, err := sess.Run(ctx, "hello")
	if err == nil {
		t.Fatal("expected error from store, got nil")
	}
	if !errors.Is(err, st.err) {
		t.Errorf("expected error to wrap %v, got %v", st.err, err)
	}
}

type echoTool struct{}

func (echoTool) Name() string        { return "echo" }
func (echoTool) Description() string { return "echo tool" }
func (echoTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        "echo",
		Description: "echo tool",
		InputSchema: tool.InputSchema{
			Type: "object",
			Properties: map[string]tool.Property{
				"msg": {Type: "string"},
			},
		},
	}
}
func (echoTool) Execute(ctx context.Context, args map[string]any, workDir string) (string, error) {
	msg, _ := args["msg"].(string)
	return msg, nil
}

type errorStore struct {
	err error
}

func (s *errorStore) CreateAgent(a *store.Agent) error                     { return nil }
func (s *errorStore) GetAgent(id string) (*store.Agent, error)             { return nil, nil }
func (s *errorStore) ListAgents() ([]*store.Agent, error)                  { return nil, nil }
func (s *errorStore) UpdateAgent(a *store.Agent) error                     { return nil }
func (s *errorStore) DeleteAgent(id string) error                          { return nil }
func (s *errorStore) CreateSession(sess *store.Session) error              { return nil }
func (s *errorStore) GetSession(id string) (*store.Session, error)         { return nil, nil }
func (s *errorStore) ListSessions() ([]*store.Session, error)              { return nil, nil }
func (s *errorStore) ListSessionsByClient(clientID string) ([]*store.Session, error) {
	return nil, nil
}
func (s *errorStore) UpdateSession(sess *store.Session) error              { return nil }
func (s *errorStore) DeleteSession(id string) error                        { return nil }
func (s *errorStore) UpsertMessage(ctx context.Context, msg store.StoredMessage) error {
	return s.err
}
func (s *errorStore) UpsertPart(ctx context.Context, part store.StoredPart) error {
	return s.err
}
func (s *errorStore) ListMessages(ctx context.Context, sessionID string) ([]store.StoredMessage, error) {
	return nil, store.ErrSessionNotFound
}
func (s *errorStore) CreateClient(name string) (*store.Client, error)     { return nil, nil }
func (s *errorStore) GetClient(id string) (*store.Client, error)           { return nil, nil }
func (s *errorStore) ListClients() ([]*store.Client, error)                { return nil, nil }
func (s *errorStore) GetAuth() (*store.Auth, error)                        { return nil, nil }
func (s *errorStore) SetAuth(auth *store.Auth) error                       { return nil }
func (s *errorStore) Close() error                                         { return nil }
