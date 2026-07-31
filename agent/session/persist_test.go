package session

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestPersistModelCallStoresUsageUnavailableAndEstimatedCost(t *testing.T) {
	data := memory.NewStore()
	stored := &store.Session{ID: "ses_test"}
	if err := data.CreateSession(stored); err != nil {
		t.Fatal(err)
	}
	sess := New(WithID(stored.ID), WithStore(data))
	model := models.ModelRef{Provider: "test", ID: "model"}
	info := models.ModelInfo{InputCostPerMTok: 3, OutputCostPerMTok: 15}

	if err := sess.persistModelCall(context.Background(), "msg_zero", run.Turn{Step: 1}, model, info, "", "", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := sess.persistModelCall(context.Background(), "msg_used", run.Turn{
		Step: 2,
		Assistant: models.Message{
			Usage: &models.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000},
		},
	}, model, info, "", "", "", nil); err != nil {
		t.Fatal(err)
	}

	calls, err := data.ListModelCalls(context.Background(), stored.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("call count = %d, want 2", len(calls))
	}
	if calls[0].TotalTokens != 0 || calls[0].Cost != nil {
		t.Fatalf("usage-unavailable call = %#v, want zero usage and no cost", calls[0])
	}
	if calls[1].Cost == nil || math.Abs(*calls[1].Cost-18) > 0.000001 {
		t.Fatalf("cost = %v, want 18", calls[1].Cost)
	}
}

func TestFinalizeUnsettledToolsMarksCallsAsInterrupted(t *testing.T) {
	messages := []models.Message{{Role: models.RoleAssistant, Content: models.Content{models.ToolPart{
		CallID: "call_1",
		Name:   "bash",
		State:  models.ToolStateRunning,
		Input:  map[string]any{"command": "pwd"},
	}}}}

	finalizeUnsettledTools(messages)
	tool := messages[0].Content[0].(models.ToolPart)
	if tool.State != models.ToolStateError || tool.Error != "Tool execution interrupted" || tool.Output != "" || tool.CompletedAt == 0 {
		t.Fatalf("tool = %#v, want interrupted error", tool)
	}
}

func TestRunFinalizesInterruptedToolsBeforeRequest(t *testing.T) {
	client := &requestCaptureClient{}
	sess := New(
		WithClient(client),
		WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}),
	)
	sess.SetHistory([]models.Message{{Role: models.RoleAssistant, Content: models.Content{models.ToolPart{
		CallID: "call_1",
		Name:   "bash",
		State:  models.ToolStatePending,
		Input:  map[string]any{"command": "pwd"},
	}}}})

	if _, err := sess.Run(context.Background(), "continue"); err != nil {
		t.Fatal(err)
	}
	if len(client.request.Messages) != 2 {
		t.Fatalf("request messages = %#v", client.request.Messages)
	}
	tool := client.request.Messages[0].Content[0].(models.ToolPart)
	if tool.State != models.ToolStateError || tool.Error != "Tool execution interrupted" {
		t.Fatalf("tool = %#v, want interrupted error", tool)
	}
	if expanded := models.ExpandToolMessages(client.request.Messages); len(expanded) != 3 || expanded[1].Role != models.RoleTool {
		t.Fatalf("expanded request = %#v, want assistant call followed by tool result", expanded)
	}
}

func TestPersistModelCallStoresAgentIDTimingAndTrace(t *testing.T) {
	data := memory.NewStore()
	stored := &store.Session{ID: "ses_test"}
	if err := data.CreateSession(stored); err != nil {
		t.Fatal(err)
	}
	sess := New(WithID(stored.ID), WithStore(data), WithAgentID("agent_123"))
	model := models.ModelRef{Provider: "test", ID: "model"}
	info := models.ModelInfo{}

	turn := run.Turn{
		Step:        1,
		Assistant:   models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "hi"}}},
		StartedAt:   time.Now().Add(-time.Second),
		CompletedAt: time.Now(),
		Trace: models.CallTrace{
			Version:  "1",
			Provider: "test",
		},
	}

	if err := sess.persistModelCall(context.Background(), "msg_1", turn, model, info, "", "agent_123", "", nil); err != nil {
		t.Fatal(err)
	}

	calls, err := data.ListModelCalls(context.Background(), stored.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 {
		t.Fatalf("call count = %d, want 1", len(calls))
	}
	call := calls[0]
	if call.AgentID != "agent_123" {
		t.Fatalf("agent_id = %q, want agent_123", call.AgentID)
	}
	if call.StartedAt.IsZero() {
		t.Fatal("StartedAt is zero")
	}
	if call.CompletedAt.IsZero() {
		t.Fatal("CompletedAt is zero")
	}
	if len(call.MetadataJSON) == 0 {
		t.Fatal("MetadataJSON empty")
	}
	var trace models.CallTrace
	if err := json.Unmarshal(call.MetadataJSON, &trace); err != nil {
		t.Fatal(err)
	}
	if trace.Version != "1" {
		t.Fatalf("trace version = %q, want 1", trace.Version)
	}
}

func TestRunPersistsStartedModelCallBeforeDispatch(t *testing.T) {
	data := memory.NewStore()
	stored := &store.Session{ID: "ses_started"}
	if err := data.CreateSession(stored); err != nil {
		t.Fatal(err)
	}
	if _, err := data.AdmitSessionRun(context.Background(), store.SessionRun{ID: "run_started", SessionID: stored.ID, Message: "hello"}); err != nil {
		t.Fatal(err)
	}
	client := &blockingRequestClient{entered: make(chan struct{}), release: make(chan struct{})}
	sess := New(
		WithID(stored.ID),
		WithRunID("run_started"),
		WithStore(data),
		WithClient(client),
		WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}),
	)
	done := make(chan error, 1)
	go func() {
		_, err := sess.Run(context.Background(), "hello")
		done <- err
	}()
	select {
	case <-client.entered:
	case err := <-done:
		t.Fatalf("Run returned before provider dispatch: %v", err)
	case <-time.After(time.Second):
		t.Fatal("provider dispatch did not start")
	}
	calls, err := data.ListModelCalls(context.Background(), stored.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].Status != store.ModelCallStatusStarted || calls[0].RunID != "run_started" || calls[0].ID == "" || calls[0].AssistantMessageID == "" {
		t.Fatalf("calls before dispatch release = %#v", calls)
	}
	messages, err := data.ListMessages(context.Background(), stored.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[1].ID != calls[0].AssistantMessageID || messages[1].State != string(models.MessageStateInProgress) || messages[1].Revision != 1 {
		t.Fatalf("messages before dispatch release = %#v", messages)
	}
	callID := calls[0].ID
	close(client.release)
	if err := <-done; err == nil {
		t.Fatal("Run succeeded, want stream error")
	}
	calls, err = data.ListModelCalls(context.Background(), stored.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].ID != callID || calls[0].Status != store.ModelCallStatusFailed || calls[0].CompletedAt.IsZero() {
		t.Fatalf("settled calls = %#v", calls)
	}
}

func TestRunPersistsFailedModelCall(t *testing.T) {
	data := memory.NewStore()
	stored := &store.Session{ID: "ses_test"}
	if err := data.CreateSession(stored); err != nil {
		t.Fatal(err)
	}
	sess := New(
		WithID(stored.ID),
		WithStore(data),
		WithClient(&failingRequestClient{}),
		WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}),
	)

	if _, err := sess.Run(context.Background(), "hello"); err == nil {
		t.Fatal("Run succeeded, want stream error")
	}

	calls, err := data.ListModelCalls(context.Background(), stored.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 {
		t.Fatalf("call count = %d, want 1", len(calls))
	}
	call := calls[0]
	if call.Status != store.ModelCallStatusFailed || call.ErrorMessage != "model stream: stream failed" || call.AssistantMessageID == "" {
		t.Fatalf("call = %#v, want failed call with checkpointed assistant message", call)
	}
	if call.StartedAt.IsZero() || call.CompletedAt.IsZero() || len(call.MetadataJSON) == 0 {
		t.Fatalf("call = %#v, want timing and trace", call)
	}
}

func TestPersistMessageAtomicallyAssignsAndPreservesIdentities(t *testing.T) {
	data := memory.NewStore()
	stored := &store.Session{ID: "ses_message"}
	if err := data.CreateSession(stored); err != nil {
		t.Fatal(err)
	}
	sess := New(WithID(stored.ID), WithStore(data))
	message, err := sess.persistMessage(context.Background(), models.Message{
		ID:       "msg_user",
		Revision: 7,
		State:    models.MessageStateInProgress,
		Role:     models.RoleUser,
		Content: models.Content{
			models.TextPart{ID: "part_text", Text: "hello"},
			models.ToolResultPart{CallID: "call_1"},
		},
	}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if message.ID != "msg_user" || message.Revision != 7 || message.State != models.MessageStateInProgress || models.PartID(message.Content[0]) != "part_text" || models.PartID(message.Content[1]) == "" {
		t.Fatalf("persisted message = %#v", message)
	}
	messages, err := data.ListMessages(context.Background(), stored.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].ID != message.ID || messages[0].Revision != message.Revision || messages[0].State != string(message.State) || len(messages[0].Parts) != 2 || messages[0].Parts[1].ID != models.PartID(message.Content[1]) {
		t.Fatalf("stored messages = %#v", messages)
	}
}

func TestStoredMessageToModelRestoresIdentitiesAndOpaquePart(t *testing.T) {
	sm := store.StoredMessage{
		ID: "msg_opaque", Revision: 4, State: string(models.MessageStateFailed), Role: string(models.RoleAssistant),
		Parts: []store.StoredPart{{ID: "part_opaque", PayloadJSON: []byte(`{"type":"plugin_part","value":{"x":1}}`)}},
	}
	message, err := StoredMessageToModel(sm)
	if err != nil {
		t.Fatal(err)
	}
	if message.ID != sm.ID || message.Revision != sm.Revision || message.State != models.MessageStateFailed || models.PartID(message.Content[0]) != "part_opaque" {
		t.Fatalf("message = %#v", message)
	}
	opaque, ok := message.Content[0].(models.OpaquePart)
	if !ok || string(opaque.Raw) != string(sm.Parts[0].PayloadJSON) {
		t.Fatalf("opaque part = %#v", message.Content[0])
	}
}

func TestCheckpointPersistenceFailurePreventsProviderDispatch(t *testing.T) {
	data := memory.NewStore()
	stored := &store.Session{ID: "ses_checkpoint_failure"}
	if err := data.CreateSession(stored); err != nil {
		t.Fatal(err)
	}
	failing := &failingSaveStore{Store: data, failAfter: 1}
	client := &dispatchSpyClient{}
	sess := New(
		WithID(stored.ID),
		WithStore(failing),
		WithClient(client),
		WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}),
	)
	if _, err := sess.Run(context.Background(), "hello"); err == nil || !errors.Is(err, errSaveMessage) || client.dispatched {
		t.Fatalf("err = %v, dispatched = %v", err, client.dispatched)
	}
}

func TestAssistantCheckpointsUpdateOneMessageWithStableIdentity(t *testing.T) {
	data := memory.NewStore()
	stored := &store.Session{ID: "ses_assistant_checkpoints"}
	if err := data.CreateSession(stored); err != nil {
		t.Fatal(err)
	}
	sess := New(
		WithID(stored.ID),
		WithStore(data),
		WithClient(&metadataRequestClient{}),
		WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}),
	)
	if _, err := sess.Run(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	messages, err := data.ListMessages(context.Background(), stored.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 {
		t.Fatalf("message count = %d, want user and one assistant", len(messages))
	}
	assistant := messages[1]
	history := sess.History()
	if assistant.Idx != 1 || assistant.ID == "" || assistant.ID != history[1].ID || assistant.Revision != history[1].Revision || assistant.State != string(models.MessageStateCompleted) || models.PartID(history[1].Content[0]) == "" {
		t.Fatalf("stored assistant = %#v, history = %#v", assistant, history[1])
	}
}

func TestRunPersistsModelCallRunAndProviderIdentity(t *testing.T) {
	data := memory.NewStore()
	stored := &store.Session{ID: "ses_identity"}
	if err := data.CreateSession(stored); err != nil {
		t.Fatal(err)
	}
	if _, err := data.AdmitSessionRun(context.Background(), store.SessionRun{ID: "run_identity", SessionID: stored.ID, Message: "hello"}); err != nil {
		t.Fatal(err)
	}
	sess := New(
		WithID(stored.ID),
		WithRunID("run_identity"),
		WithStore(data),
		WithClient(&metadataRequestClient{}),
		WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{ContextWindow: 1000}),
		WithTransformHistory(func(_ context.Context, info run.TransformHistoryInfo) ([]models.Message, error) {
			info.Sink.OnEvent(run.MessageEvent{Message: models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "synthetic"}}}})
			return nil, nil
		}),
	)
	if _, err := sess.Run(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	calls, err := data.ListModelCalls(context.Background(), stored.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 {
		t.Fatalf("calls = %#v", calls)
	}
	call := calls[0]
	if call.RunID != "run_identity" || call.ProviderRequestID != "provider-request-1" || call.Status != store.ModelCallStatusCompleted || call.AssistantMessageID == "" {
		t.Fatalf("call = %#v", call)
	}
	if call.InputTokens != 4 || call.OutputTokens != 2 || call.ContextTokens != 6 {
		t.Fatalf("usage = %#v", call)
	}
	messages, err := data.ListMessages(context.Background(), stored.ID)
	if err != nil {
		t.Fatal(err)
	}
	var providerMessageID string
	for _, storedMessage := range messages {
		message, err := StoredMessageToModel(storedMessage)
		if err != nil {
			t.Fatal(err)
		}
		if message.Role == models.RoleAssistant && textOf(message) == "done" {
			providerMessageID = storedMessage.ID
		}
	}
	if providerMessageID == "" || call.AssistantMessageID != providerMessageID {
		t.Fatalf("assistant message ID = %q, provider message ID = %q", call.AssistantMessageID, providerMessageID)
	}
}

type requestCaptureClient struct {
	request models.Request
}

var errSaveMessage = errors.New("save message failed")

type failingSaveStore struct {
	store.Store
	saves     int
	failAfter int
}

func (s *failingSaveStore) SaveMessage(ctx context.Context, message store.StoredMessage) error {
	s.saves++
	if s.saves > s.failAfter {
		return errSaveMessage
	}
	return s.Store.SaveMessage(ctx, message)
}

type dispatchSpyClient struct{ dispatched bool }

func (c *dispatchSpyClient) Prepare(context.Context, models.Request) (*models.PreparedRequest, error) {
	return nil, errors.New("unexpected Prepare")
}

func (c *dispatchSpyClient) Generate(context.Context, models.Request) (*models.Message, error) {
	return nil, errors.New("unexpected Generate")
}

func (c *dispatchSpyClient) Stream(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	c.dispatched = true
	return nil, errors.New("unexpected Stream")
}

type failingRequestClient struct{}

type metadataRequestClient struct{}

func (c *metadataRequestClient) Prepare(context.Context, models.Request) (*models.PreparedRequest, error) {
	return nil, errors.New("unexpected Prepare")
}

func (c *metadataRequestClient) Generate(context.Context, models.Request) (*models.Message, error) {
	return nil, errors.New("unexpected Generate")
}

func (c *metadataRequestClient) Stream(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	message := &models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "done"}}, FinishReason: models.FinishReasonStop}
	usage := models.Usage{InputTokens: 4, OutputTokens: 2, TotalTokens: 6}
	stream := models.NewEventStream[models.StreamPart, *models.Message](2)
	stream.Push(models.ResponseMetadataPart{Meta: map[string]any{"request_id": "provider-request-1"}})
	stream.Push(models.FinishPart{Reason: models.FinishReasonStop, Usage: usage, Message: message})
	stream.Close(message, nil)
	return stream, nil
}

type blockingRequestClient struct {
	entered chan struct{}
	release chan struct{}
}

func (c *blockingRequestClient) Prepare(context.Context, models.Request) (*models.PreparedRequest, error) {
	return nil, errors.New("unexpected Prepare")
}

func (c *blockingRequestClient) Generate(context.Context, models.Request) (*models.Message, error) {
	return nil, errors.New("unexpected Generate")
}

func (c *blockingRequestClient) Stream(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	close(c.entered)
	<-c.release
	return nil, errors.New("stream failed")
}

func (c *failingRequestClient) Prepare(context.Context, models.Request) (*models.PreparedRequest, error) {
	return nil, errors.New("unexpected Prepare")
}

func (c *failingRequestClient) Generate(context.Context, models.Request) (*models.Message, error) {
	return nil, errors.New("unexpected Generate")
}

func (c *failingRequestClient) Stream(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	return nil, errors.New("stream failed")
}

func (c *requestCaptureClient) Prepare(context.Context, models.Request) (*models.PreparedRequest, error) {
	return nil, errors.New("unexpected Prepare")
}

func (c *requestCaptureClient) Generate(context.Context, models.Request) (*models.Message, error) {
	return nil, errors.New("unexpected Generate")
}

func (c *requestCaptureClient) Stream(_ context.Context, request models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	c.request = request
	stream := models.NewEventStream[models.StreamPart, *models.Message](0)
	stream.Close(&models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "done"}}}, nil)
	return stream, nil
}
