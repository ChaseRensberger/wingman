package session

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/plugins/compaction"
	"github.com/chaserensberger/wingman/store"
)

type overflowClient struct {
	requestCaptureClient
	data             store.Store
	calls, summaries int
	cancel           context.CancelFunc
	t                *testing.T
}

type overflowCrashStore struct {
	store.Store
	stage string
}

func (s *overflowCrashStore) UpsertModelCall(ctx context.Context, call store.ModelCall) error {
	if err := s.Store.UpsertModelCall(ctx, call); err != nil {
		return err
	}
	var trace models.CallTrace
	if err := json.Unmarshal(call.MetadataJSON, &trace); err != nil {
		return err
	}
	if s.stage == "intent" && trace.Retry != nil && trace.Retry.Reason == "context_overflow" {
		os.Exit(23)
	}
	return nil
}

func (s *overflowCrashStore) SaveMessage(ctx context.Context, message store.StoredMessage) error {
	if err := s.Store.SaveMessage(ctx, message); err != nil {
		return err
	}
	for _, part := range message.Parts {
		if part.Kind == compaction.PartType &&
			((s.stage == "checkpoint" && message.State == string(models.MessageStateCompleted)) ||
				(s.stage == "compaction-started" && message.State == string(models.MessageStateInProgress))) {
			os.Exit(23)
		}
	}
	return nil
}

type overflowCrashClient struct {
	requestCaptureClient
	stage            string
	summaries, calls int
}

func (c *overflowCrashClient) Generate(context.Context, models.Request) (*models.Message, error) {
	c.summaries++
	if c.stage == "summary" {
		os.Exit(23)
	}
	return &models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "summary"}}}, nil
}

func (c *overflowCrashClient) Stream(_ context.Context, request models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	c.calls++
	if c.stage != "resume" {
		return nil, models.ClassifyProviderFailure("test", 400, `{"error":{"code":"context_length_exceeded"}}`)
	}
	return c.requestCaptureClient.Stream(context.Background(), request)
}

func TestOverflowRecoveryAcrossProcessCrashes(t *testing.T) {
	if stage := os.Getenv("WINGMAN_OVERFLOW_CRASH_STAGE"); stage != "" {
		data, err := store.NewSQLiteStore(os.Getenv("WINGMAN_OVERFLOW_CRASH_DB"))
		if err != nil {
			t.Fatal(err)
		}
		defer data.Close()
		sess := New(WithID("ses_history"), WithRunID(os.Getenv("WINGMAN_OVERFLOW_CRASH_RUN")), WithStore(&overflowCrashStore{Store: data, stage: stage}), WithClient(&overflowCrashClient{stage: stage}), WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{ContextWindow: 8192, MaxOutput: 512}), WithPlugin(compaction.New(compaction.WithMinMessages(100))))
		_, err = sess.Run(t.Context(), strings.Repeat("history ", 1000))
		t.Fatalf("did not reach crash stage: %v", err)
	}
	for _, stage := range []string{"intent", "compaction-started", "summary", "checkpoint"} {
		t.Run(stage, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "overflow.db")
			data, err := store.NewSQLiteStore(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := data.CreateSession(&store.Session{ID: "ses_history"}); err != nil {
				t.Fatal(err)
			}
			admitted, err := data.AdmitSessionRun(t.Context(), store.SessionRun{SessionID: "ses_history", Agent: store.Agent{ID: "agt_test"}, Message: "go"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := data.ClaimNextSessionRun(t.Context(), "ses_history"); err != nil {
				t.Fatal(err)
			}
			if err := data.Close(); err != nil {
				t.Fatal(err)
			}
			command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestOverflowRecoveryAcrossProcessCrashes$")
			command.Env = append(os.Environ(), "WINGMAN_OVERFLOW_CRASH_STAGE="+stage, "WINGMAN_OVERFLOW_CRASH_DB="+path, "WINGMAN_OVERFLOW_CRASH_RUN="+admitted.Run.ID)
			output, err := command.CombinedOutput()
			var exited *exec.ExitError
			if !errors.As(err, &exited) || exited.ExitCode() != 23 {
				t.Fatalf("crash helper: %v\n%s", err, output)
			}
			data, err = store.NewSQLiteStore(path)
			if err != nil {
				t.Fatal(err)
			}
			defer data.Close()
			if err := data.InterruptActiveModelCalls(t.Context(), admitted.Run.ID, "process_interrupted", "process stopped"); err != nil {
				t.Fatal(err)
			}
			if err := RecoverRunMessages(t.Context(), data, "ses_history", admitted.Run.ID); err != nil {
				t.Fatal(err)
			}
			messages, err := data.ListMessages(t.Context(), "ses_history")
			if err != nil {
				t.Fatal(err)
			}
			for _, message := range messages {
				if message.State == string(models.MessageStateInProgress) {
					t.Fatal("recovered compaction still appears active")
				}
			}
			if _, err := data.RequeueSessionRun(t.Context(), admitted.Run.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := data.ClaimNextSessionRun(t.Context(), "ses_history"); err != nil {
				t.Fatal(err)
			}
			client := &overflowCrashClient{stage: "resume"}
			sess := New(WithID("ses_history"), WithRunID(admitted.Run.ID), WithStore(data), WithRunRecovery(), WithClient(client), WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{ContextWindow: 8192, MaxOutput: 512}), WithPlugin(compaction.New(compaction.WithMinMessages(100))))
			defer sess.Close(context.Background())
			if _, err := sess.Run(t.Context(), "go"); err != nil {
				t.Fatal(err)
			}
			wantSummaries := 1
			if stage == "checkpoint" {
				wantSummaries = 0
			}
			if client.calls != 1 || client.summaries != wantSummaries {
				t.Fatalf("calls=%d summaries=%d", client.calls, client.summaries)
			}
			calls, err := data.QueryModelCalls(t.Context(), "ses_history", store.ModelCallQuery{RunID: admitted.Run.ID})
			if err != nil || len(calls) != 2 {
				t.Fatalf("calls=%#v err=%v", calls, err)
			}
			if calls[0].Step == calls[1].Step {
				t.Fatal("retry reused the rejected step")
			}
		})
	}
}

func (c *overflowClient) Generate(context.Context, models.Request) (*models.Message, error) {
	c.summaries++
	return &models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "summary"}}}, nil
}

func (c *overflowClient) Stream(ctx context.Context, req models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	c.calls++
	if c.calls == 2 {
		messages, err := c.data.ListMessages(ctx, "ses_history")
		if err != nil {
			return nil, err
		}
		found := false
		for _, msg := range messages {
			for _, part := range msg.Parts {
				if part.Kind == compaction.PartType {
					found = true
				}
			}
		}
		if !found {
			c.t.Error("retry started before durable compaction")
		}
		if len(req.Messages) != 1 || !strings.Contains(req.Messages[0].Content[0].(models.TextPart).Text, "<conversation-checkpoint>") {
			c.t.Error("retry did not use checkpoint")
		}
		c.cancel()
		return nil, ctx.Err()
	}
	return nil, models.ClassifyProviderFailure("test", 400, `{"error":{"code":"context_length_exceeded"}}`)
}

func TestOverflowRecoveryBudgetSurvivesRestart(t *testing.T) {
	for _, backend := range []string{"memory", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			data := historyStore(t, backend)
			admitted, err := data.AdmitSessionRun(t.Context(), store.SessionRun{SessionID: "ses_history", Agent: store.Agent{ID: "agt_test"}, Message: "go"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := data.ClaimNextSessionRun(t.Context(), "ses_history"); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			client := &overflowClient{data: data, cancel: cancel, t: t}
			options := []Option{WithID("ses_history"), WithRunID(admitted.Run.ID), WithStore(data), WithClient(client), WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{ContextWindow: 8192, MaxOutput: 512}), WithPlugin(compaction.New(compaction.WithMinMessages(100)))}
			first := New(options...)
			if _, err := first.Run(ctx, strings.Repeat("history ", 1000)); !errors.Is(err, context.Canceled) {
				t.Fatalf("first run=%v", err)
			}
			_ = first.Close(context.Background())
			if client.calls != 2 || client.summaries != 1 {
				t.Fatalf("calls=%d summaries=%d", client.calls, client.summaries)
			}
			second := New(append(options, WithRunRecovery())...)
			defer second.Close(context.Background())
			if _, err := second.Run(t.Context(), "go"); err == nil {
				t.Fatal("second overflow should fail")
			}
			if client.calls != 3 || client.summaries != 1 {
				t.Fatalf("recovery reset budget: calls=%d summaries=%d", client.calls, client.summaries)
			}
		})
	}
}
