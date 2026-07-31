package session

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
	"github.com/chaserensberger/wingman/tool"
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
	sess := New(WithID(stored.ID), WithRunID("run_message"), WithStore(data))
	if _, err := data.AdmitSessionRun(context.Background(), store.SessionRun{ID: "run_message", SessionID: stored.ID}); err != nil {
		t.Fatal(err)
	}
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
	if len(messages) != 1 || messages[0].ID != message.ID || messages[0].RunID != "run_message" || messages[0].Revision != message.Revision || messages[0].State != string(message.State) || len(messages[0].Parts) != 2 || messages[0].Parts[1].ID != models.PartID(message.Content[1]) {
		t.Fatalf("stored messages = %#v", messages)
	}
}

func TestRecoverRunMessagesReconcilesOnlyTargetRunIdempotently(t *testing.T) {
	data := memory.NewStore()
	ctx := context.Background()
	if err := data.CreateSession(&store.Session{ID: "ses_recover"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"run_target", "run_other"} {
		if _, err := data.AdmitSessionRun(ctx, store.SessionRun{ID: id, SessionID: "ses_recover"}); err != nil {
			t.Fatal(err)
		}
	}
	created := time.Now().Add(-time.Minute).UTC()
	partial := store.StoredMessage{ID: "msg_partial", SessionID: "ses_recover", RunID: "run_target", Idx: 0, Role: "assistant", Revision: 3, State: "in_progress", CreatedAt: created, UpdatedAt: created, Parts: []store.StoredPart{{ID: "part_text", MessageID: "msg_partial", Sequence: 0, Kind: "text", PayloadJSON: []byte(`{"type":"text","id":"part_text","text":"partial"}`), CreatedAt: created, UpdatedAt: created}}}
	pending := store.StoredMessage{ID: "msg_tool", SessionID: "ses_recover", RunID: "run_target", Idx: 1, Role: "assistant", Revision: 1, State: "completed", CreatedAt: created, UpdatedAt: created, Parts: []store.StoredPart{{ID: "part_tool", MessageID: "msg_tool", Sequence: 0, Kind: "tool", PayloadJSON: []byte(`{"type":"tool","id":"part_tool","call_id":"call","name":"bash","state":"pending","input_raw":"{bad"}`), CreatedAt: created, UpdatedAt: created}}}
	other := store.StoredMessage{ID: "msg_other", SessionID: "ses_recover", RunID: "run_other", Idx: 2, Role: "assistant", Revision: 2, State: "in_progress", CreatedAt: created, UpdatedAt: created, Parts: []store.StoredPart{{ID: "part_opaque", MessageID: "msg_other", Sequence: 0, Kind: "plugin", PayloadJSON: []byte(`{"type":"plugin","value":{"x":1}}`), CreatedAt: created, UpdatedAt: created}}}
	for _, message := range []store.StoredMessage{partial, pending, other} {
		if err := data.SaveMessage(ctx, message); err != nil {
			t.Fatal(err)
		}
	}
	if err := RecoverRunMessages(ctx, data, "ses_recover", "run_target"); err != nil {
		t.Fatal(err)
	}
	messages, err := data.ListMessages(ctx, "ses_recover")
	if err != nil {
		t.Fatal(err)
	}
	if messages[0].State != "failed" || messages[0].Revision != 4 || messages[0].CreatedAt != created {
		t.Fatalf("partial = %#v", messages[0])
	}
	toolMessage, err := StoredMessageToModel(messages[1])
	if err != nil {
		t.Fatal(err)
	}
	toolPart := toolMessage.Content[0].(models.ToolPart)
	if messages[1].Revision != 2 || toolPart.State != models.ToolStateError || toolPart.Error != "Tool execution interrupted" || toolPart.InputRaw != "{bad" || toolPart.CompletedAt == 0 {
		t.Fatalf("pending tool = %#v", toolPart)
	}
	if messages[2].State != "in_progress" || messages[2].Revision != 2 || string(messages[2].Parts[0].PayloadJSON) != string(other.Parts[0].PayloadJSON) {
		t.Fatalf("other run message = %#v", messages[2])
	}
	updated := messages[0].UpdatedAt
	if err := RecoverRunMessages(ctx, data, "ses_recover", "run_target"); err != nil {
		t.Fatal(err)
	}
	messages, err = data.ListMessages(ctx, "ses_recover")
	if err != nil {
		t.Fatal(err)
	}
	if messages[0].Revision != 4 || !messages[0].UpdatedAt.Equal(updated) || messages[1].Revision != 2 {
		t.Fatalf("second recovery changed messages: %#v", messages)
	}
}

func TestRecoverRunMessagesProjectsToolUse(t *testing.T) {
	data := memory.NewStore()
	ctx := context.Background()
	if err := data.CreateSession(&store.Session{ID: "ses_tool_recover"}); err != nil {
		t.Fatal(err)
	}
	if _, err := data.AdmitSessionRun(ctx, store.SessionRun{ID: "run_tool_recover", SessionID: "ses_tool_recover"}); err != nil {
		t.Fatal(err)
	}
	message := store.StoredMessage{ID: "msg_tool_recover", SessionID: "ses_tool_recover", RunID: "run_tool_recover", Idx: 0, Role: "assistant", Revision: 1, State: "completed", Parts: []store.StoredPart{{ID: "part_tool_recover", MessageID: "msg_tool_recover", Sequence: 0, Kind: "tool", PayloadJSON: []byte(`{"type":"tool","id":"part_tool_recover","call_id":"call","name":"bash","state":"running","input_raw":"raw","provider_metadata":{"keep":true}}`)}}}
	if err := data.SaveMessage(ctx, message); err != nil {
		t.Fatal(err)
	}
	started, completed := time.Now().Add(-time.Second).UTC(), time.Now().UTC()
	use := store.ToolUse{ID: "tlu_recover", SessionID: "ses_tool_recover", RunID: "run_tool_recover", PartID: "part_tool_recover", Status: store.ToolUseStatusProposed, InputJSON: []byte(`{"command":"pwd"}`)}
	if err := data.SaveToolUse(ctx, use); err != nil {
		t.Fatal(err)
	}
	use.Status = store.ToolUseStatusAuthorized
	if err := data.SaveToolUse(ctx, use); err != nil {
		t.Fatal(err)
	}
	use.Status = store.ToolUseStatusStarted
	use.StartedAt = started
	if err := data.SaveToolUse(ctx, use); err != nil {
		t.Fatal(err)
	}
	use.Status = store.ToolUseStatusCompleted
	use.Output = "/tmp"
	use.MetadataJSON = []byte(`{"source":"store"}`)
	use.CompletedAt = completed
	if err := data.SaveToolUse(ctx, use); err != nil {
		t.Fatal(err)
	}
	if err := RecoverRunMessages(ctx, data, "ses_tool_recover", "run_tool_recover"); err != nil {
		t.Fatal(err)
	}
	messages, err := data.ListMessages(ctx, "ses_tool_recover")
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := StoredMessageToModel(messages[0])
	if err != nil {
		t.Fatal(err)
	}
	part := recovered.Content[0].(models.ToolPart)
	if messages[0].Revision != 2 || part.ToolUseID != "tlu_recover" || part.State != models.ToolStateCompleted || part.Input["command"] != "pwd" || part.Output != "/tmp" || part.Metadata["source"] != "store" || part.InputRaw != "raw" || part.ProviderMetadata["keep"] != true || part.StartedAt != started.UnixMilli() || part.CompletedAt != completed.UnixMilli() {
		t.Fatalf("recovered part = %#v", part)
	}
}

func TestRecoverRunMessagesProjectsFailedToolUse(t *testing.T) {
	data := memory.NewStore()
	ctx := context.Background()
	if err := data.CreateSession(&store.Session{ID: "ses_failed_tool_recover"}); err != nil {
		t.Fatal(err)
	}
	if _, err := data.AdmitSessionRun(ctx, store.SessionRun{ID: "run_failed_tool_recover", SessionID: "ses_failed_tool_recover"}); err != nil {
		t.Fatal(err)
	}
	message := store.StoredMessage{ID: "msg_failed_tool", SessionID: "ses_failed_tool_recover", RunID: "run_failed_tool_recover", Idx: 0, Role: "assistant", Revision: 1, State: "completed", Parts: []store.StoredPart{{ID: "part_failed_tool", MessageID: "msg_failed_tool", Sequence: 0, Kind: "tool", PayloadJSON: []byte(`{"type":"tool","id":"part_failed_tool","call_id":"call","name":"bash","state":"completed","output":"stale"}`)}}}
	if err := data.SaveMessage(ctx, message); err != nil {
		t.Fatal(err)
	}
	use := store.ToolUse{ID: "tlu_failed", SessionID: "ses_failed_tool_recover", RunID: "run_failed_tool_recover", PartID: "part_failed_tool", Status: store.ToolUseStatusProposed}
	if err := data.SaveToolUse(ctx, use); err != nil {
		t.Fatal(err)
	}
	use.Status, use.ErrorMessage, use.Output = store.ToolUseStatusFailed, "tool failed", "failure output"
	if err := data.SaveToolUse(ctx, use); err != nil {
		t.Fatal(err)
	}
	if err := RecoverRunMessages(ctx, data, "ses_failed_tool_recover", "run_failed_tool_recover"); err != nil {
		t.Fatal(err)
	}
	messages, err := data.ListMessages(ctx, "ses_failed_tool_recover")
	if err != nil {
		t.Fatal(err)
	}
	part, err := StoredMessageToModel(messages[0])
	if err != nil {
		t.Fatal(err)
	}
	toolPart := part.Content[0].(models.ToolPart)
	if toolPart.State != models.ToolStateError || toolPart.Error != "tool failed" || toolPart.Output != "failure output" {
		t.Fatalf("failed tool = %#v", toolPart)
	}
}

func TestRecoverRunMessagesSaveFailureLeavesOtherRunsUntouched(t *testing.T) {
	data := memory.NewStore()
	ctx := context.Background()
	if err := data.CreateSession(&store.Session{ID: "ses_recover_failure"}); err != nil {
		t.Fatal(err)
	}
	for _, runID := range []string{"run_recover_failure", "run_recover_other"} {
		if _, err := data.AdmitSessionRun(ctx, store.SessionRun{ID: runID, SessionID: "ses_recover_failure"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, message := range []store.StoredMessage{
		{ID: "msg_recover_failure", SessionID: "ses_recover_failure", RunID: "run_recover_failure", Idx: 0, Role: "assistant", State: "in_progress"},
		{ID: "msg_recover_other", SessionID: "ses_recover_failure", RunID: "run_recover_other", Idx: 1, Role: "assistant", State: "in_progress"},
	} {
		if err := data.SaveMessage(ctx, message); err != nil {
			t.Fatal(err)
		}
	}
	failing := &failingSaveStore{Store: data, failAfter: 0}
	if err := RecoverRunMessages(ctx, failing, "ses_recover_failure", "run_recover_failure"); !errors.Is(err, errSaveMessage) {
		t.Fatalf("err = %v, want save failure", err)
	}
	messages, err := data.ListMessages(ctx, "ses_recover_failure")
	if err != nil {
		t.Fatal(err)
	}
	if messages[0].State != "in_progress" || messages[1].State != "in_progress" || messages[1].Revision != 1 {
		t.Fatalf("messages after failed recovery = %#v", messages)
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

func TestRunPersistsToolUseLifecycleAndHydratesToolUseID(t *testing.T) {
	data := memory.NewStore()
	stored := &store.Session{ID: "ses_tool_lifecycle"}
	if err := data.CreateSession(stored); err != nil {
		t.Fatal(err)
	}
	if _, err := data.AdmitSessionRun(context.Background(), store.SessionRun{ID: "run_tool_lifecycle", SessionID: stored.ID, Message: "hello"}); err != nil {
		t.Fatal(err)
	}
	client := &sequencedToolClient{messages: []models.Message{
		{Role: models.RoleAssistant, Content: models.Content{models.ToolCallPart{CallID: "call_1", Name: "test", Input: map[string]any{"original": true}}}},
		{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "done"}}},
	}}
	sess := New(
		WithID(stored.ID), WithRunID("run_tool_lifecycle"), WithStore(data), WithClient(client),
		WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}),
		WithTools(tool.NewFuncTool("test", "test", tool.Definition{Name: "test", InputSchema: tool.InputSchema{Type: "object"}, OutputSchema: map[string]any{"type": "object", "required": []any{"count"}}}, func(_ context.Context, inv tool.Invocation) (tool.Result, error) {
			if inv.Input["rewritten"] != true {
				t.Fatalf("tool input = %#v", inv.Input)
			}
			return tool.Result{Text: "model-visible output", Structured: map[string]any{"count": float64(1)}, Metadata: map[string]any{"source": "tool"}}, nil
		})),
		WithPlugin(beforeToolRewritePlugin{}),
	)
	if _, err := sess.Run(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	uses, err := data.ListToolUses(context.Background(), stored.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(uses) != 1 {
		t.Fatalf("tool uses = %#v", uses)
	}
	use := uses[0]
	if use.ID == "" || use.RunID != "run_tool_lifecycle" || use.ModelCallID == "" || use.AssistantMessageID == "" || use.PartID == "" || use.CallID != "call_1" || use.Step != 1 || use.Ordinal != 1 || use.Status != store.ToolUseStatusCompleted || use.ProposedAt.IsZero() || use.AuthorizedAt.IsZero() || use.StartedAt.IsZero() || use.CompletedAt.IsZero() {
		t.Fatalf("tool use = %#v", use)
	}
	var input, structured, metadata map[string]any
	if err := json.Unmarshal(use.InputJSON, &input); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(use.MetadataJSON, &metadata); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(use.StructuredJSON, &structured); err != nil {
		t.Fatal(err)
	}
	if input["rewritten"] != true || use.Output != "model-visible output" || structured["count"] != float64(1) || metadata["source"] != "tool" {
		t.Fatalf("tool use payload = %#v, input=%#v, structured=%#v, metadata=%#v", use, input, structured, metadata)
	}
	hydrated := New(WithID(stored.ID), WithStore(data))
	if err := hydrated.hydrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	part := hydrated.History()[1].Content[0].(models.ToolPart)
	if part.ToolUseID != use.ID || models.PartID(part) != use.PartID || part.Structured.(map[string]any)["count"] != float64(1) {
		t.Fatalf("hydrated tool part = %#v, use = %#v", part, use)
	}
}

func TestToolUseRecorderMapsFailureAndDecline(t *testing.T) {
	for _, status := range []run.ToolUseStatus{run.ToolUseStatusFailed, run.ToolUseStatusDeclined} {
		t.Run(string(status), func(t *testing.T) {
			data := memory.NewStore()
			if err := data.CreateSession(&store.Session{ID: "ses_status"}); err != nil {
				t.Fatal(err)
			}
			recorder := &toolUseRecorder{store: data, sessionID: "ses_status", runID: ""}
			id, err := recorder.Propose(context.Background(), run.ToolUseProposeInfo{Step: 1, Ordinal: 1, CallID: "call", Name: "test", Args: map[string]any{}})
			if err != nil {
				t.Fatal(err)
			}
			if err := recorder.Finish(context.Background(), run.ToolUseFinishInfo{Step: 1, Ordinal: 1, ToolUseID: id, CallID: "call", Name: "test", Args: map[string]any{}, Status: status, ErrorMessage: "terminal"}); err != nil {
				t.Fatal(err)
			}
			uses, err := data.ListToolUses(context.Background(), "ses_status")
			if err != nil || len(uses) != 1 || uses[0].Status != string(status) || uses[0].ErrorMessage != "terminal" {
				t.Fatalf("uses=%#v err=%v", uses, err)
			}
		})
	}
}

func TestRunPersistsParallelToolUsesWithDistinctIDsAndOrdinals(t *testing.T) {
	data := memory.NewStore()
	stored := &store.Session{ID: "ses_parallel_tools"}
	if err := data.CreateSession(stored); err != nil {
		t.Fatal(err)
	}
	client := &sequencedToolClient{messages: []models.Message{
		{Role: models.RoleAssistant, Content: models.Content{
			models.ToolCallPart{CallID: "call_1", Name: "test", Input: map[string]any{"n": float64(1)}},
			models.ToolCallPart{CallID: "call_2", Name: "test", Input: map[string]any{"n": float64(2)}},
		}},
		{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "done"}}},
	}}
	var mu sync.Mutex
	executed := 0
	sess := New(WithID(stored.ID), WithStore(data), WithClient(client), WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}), WithTools(tool.NewFuncTool("test", "test", tool.Definition{Name: "test", InputSchema: tool.InputSchema{Type: "object"}}, func(context.Context, tool.Invocation) (tool.Result, error) {
		mu.Lock()
		executed++
		mu.Unlock()
		return tool.Result{}, nil
	})))
	if _, err := sess.Run(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	uses, err := data.ListToolUses(context.Background(), stored.ID)
	if err != nil {
		t.Fatal(err)
	}
	if executed != 2 || len(uses) != 2 || uses[0].ID == uses[1].ID || uses[0].Ordinal != 1 || uses[1].Ordinal != 2 {
		t.Fatalf("executed=%d uses=%#v", executed, uses)
	}
}

func TestToolUseStartPersistenceFailurePreventsExecution(t *testing.T) {
	data := memory.NewStore()
	stored := &store.Session{ID: "ses_start_failure"}
	if err := data.CreateSession(stored); err != nil {
		t.Fatal(err)
	}
	client := &sequencedToolClient{messages: []models.Message{{Role: models.RoleAssistant, Content: models.Content{models.ToolCallPart{CallID: "call", Name: "test", Input: map[string]any{}}}}}}
	failing := &failingToolUseStore{Store: data, failStatus: store.ToolUseStatusStarted}
	executed := 0
	sess := New(WithID(stored.ID), WithStore(failing), WithClient(client), WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}), WithTools(tool.NewFuncTool("test", "test", tool.Definition{Name: "test", InputSchema: tool.InputSchema{Type: "object"}}, func(context.Context, tool.Invocation) (tool.Result, error) {
		executed++
		return tool.Result{}, nil
	})))
	if _, err := sess.Run(context.Background(), "hello"); err == nil || !errors.Is(err, errSaveToolUse) || executed != 0 {
		t.Fatalf("err=%v executed=%d", err, executed)
	}
}

func TestToolUseTerminalPersistenceFailureDoesNotReexecute(t *testing.T) {
	data := memory.NewStore()
	stored := &store.Session{ID: "ses_finish_failure"}
	if err := data.CreateSession(stored); err != nil {
		t.Fatal(err)
	}
	client := &sequencedToolClient{messages: []models.Message{{Role: models.RoleAssistant, Content: models.Content{models.ToolCallPart{CallID: "call", Name: "test", Input: map[string]any{}}}}}}
	failing := &failingToolUseStore{Store: data, failStatus: store.ToolUseStatusCompleted}
	executed := 0
	sess := New(WithID(stored.ID), WithStore(failing), WithClient(client), WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}), WithTools(tool.NewFuncTool("test", "test", tool.Definition{Name: "test", InputSchema: tool.InputSchema{Type: "object"}}, func(context.Context, tool.Invocation) (tool.Result, error) {
		executed++
		return tool.Result{}, nil
	})))
	if _, err := sess.Run(context.Background(), "hello"); err == nil || !errors.Is(err, errSaveToolUse) || executed != 1 {
		t.Fatalf("err=%v executed=%d", err, executed)
	}
}

type requestCaptureClient struct {
	request models.Request
}

type beforeToolRewritePlugin struct{}

func (beforeToolRewritePlugin) Name() string { return "before-tool-rewrite" }

func (beforeToolRewritePlugin) Install(reg *plugin.Registry) error {
	reg.RegisterBeforeToolCall(func(context.Context, run.ToolCall) (map[string]any, error) {
		return map[string]any{"rewritten": true}, nil
	})
	return nil
}

type sequencedToolClient struct {
	mu       sync.Mutex
	messages []models.Message
	next     int
}

func (c *sequencedToolClient) Prepare(context.Context, models.Request) (*models.PreparedRequest, error) {
	return nil, errors.New("unexpected Prepare")
}

func (c *sequencedToolClient) Generate(context.Context, models.Request) (*models.Message, error) {
	return nil, errors.New("unexpected Generate")
}

func (c *sequencedToolClient) Stream(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	c.mu.Lock()
	if c.next >= len(c.messages) {
		c.mu.Unlock()
		return nil, errors.New("unexpected Stream")
	}
	message := c.messages[c.next]
	c.next++
	c.mu.Unlock()
	stream := models.NewEventStream[models.StreamPart, *models.Message](0)
	stream.Close(&message, nil)
	return stream, nil
}

var errSaveToolUse = errors.New("save tool use failed")

type failingToolUseStore struct {
	store.Store
	failStatus string
}

func (s *failingToolUseStore) SaveToolUse(ctx context.Context, use store.ToolUse) error {
	if use.Status == s.failStatus {
		return errSaveToolUse
	}
	return s.Store.SaveToolUse(ctx, use)
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
