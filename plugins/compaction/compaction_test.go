package compaction

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/models"
)

func TestTransformHistoryUsesPreparedRequestBudget(t *testing.T) {
	client := &testClient{summary: "summary", body: map[string]any{"system": strings.Repeat("x", 20000)}}
	p := New(WithMinMessages(0), WithKeepTail(1), WithReserveTokens(20))
	msgs := []models.Message{
		{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "old"}}},
		{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "recent"}}},
	}
	out, err := p.transformHistory(context.Background(), run.TransformHistoryInfo{
		Messages: msgs, Client: client, Model: models.ModelRef{Provider: "test", ID: "model"},
		ModelInfo: models.ModelInfo{ContextWindow: 4096, MaxOutput: 10},
		Request:   models.Request{Model: models.ModelRef{Provider: "test", ID: "model"}, Messages: msgs},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 || findLatestMarker(out) != 2 {
		t.Fatalf("history = %#v, want marker at physical end", out)
	}
	marker, ok := markerInMessage(out[2])
	if !ok || !strings.Contains(marker.Recent, "[User]: recent") {
		t.Fatalf("marker = %#v, want serialized recent context", marker)
	}
	if client.maxOutput != 10 {
		t.Fatalf("summary max output = %d, want 10", client.maxOutput)
	}
}

func TestTransformHistoryRejectsEmptySummary(t *testing.T) {
	client := &testClient{body: map[string]any{"messages": strings.Repeat("x", 20000)}}
	p := New(WithMinMessages(0), WithKeepTail(1), WithReserveTokens(20))
	msgs := []models.Message{
		{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "old"}}},
		{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "recent"}}},
	}
	_, err := p.transformHistory(context.Background(), run.TransformHistoryInfo{
		Messages: msgs, Client: client, Model: models.ModelRef{Provider: "test", ID: "model"},
		ModelInfo: models.ModelInfo{ContextWindow: 4096},
		Request:   models.Request{Model: models.ModelRef{Provider: "test", ID: "model"}, Messages: msgs},
	})
	if err == nil || !strings.Contains(err.Error(), "empty summary") {
		t.Fatalf("error = %v, want empty summary error", err)
	}
}

func TestTransformHistoryIgnoresOversizedReserve(t *testing.T) {
	client := &testClient{summary: "summary", body: map[string]any{"messages": "small"}}
	p := New(WithMinMessages(0), WithKeepTail(1))
	msgs := []models.Message{
		{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "old"}}},
		{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "recent"}}},
	}
	out, err := p.transformHistory(context.Background(), run.TransformHistoryInfo{
		Messages: msgs, Client: client, Model: models.ModelRef{Provider: "test", ID: "model"},
		ModelInfo: models.ModelInfo{ContextWindow: 100},
		Request:   models.Request{Model: models.ModelRef{Provider: "test", ID: "model"}, Messages: msgs},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(msgs) {
		t.Fatalf("history count = %d, want %d", len(out), len(msgs))
	}
}

func TestManualActionSummarizesCompleteShortTurn(t *testing.T) {
	usage := &models.Usage{InputTokens: 60300, OutputTokens: 1900, TotalTokens: 62200}
	client := &testClient{summary: "summary", body: map[string]any{"messages": "small"}, usage: usage}
	p := New(WithMinMessages(100))
	msgs := []models.Message{
		{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "Hello how are you?"}}},
		{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "I'm doing well."}}},
	}
	var emitted models.Message
	err := p.compactAction(context.Background(), plugin.ActionInfo{
		History: msgs, Client: client, Model: models.ModelRef{Provider: "test", ID: "model"},
		ModelInfo: models.ModelInfo{ContextWindow: 4096},
		Sink: run.SinkFunc(func(event run.Event) {
			if message, ok := event.(run.MessageEvent); ok {
				emitted = message.Message
			}
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	out := append(append([]models.Message(nil), msgs...), emitted)
	if len(out) != 3 || findLatestMarker(out) != 2 || emitted.Content == nil {
		t.Fatalf("history = %#v, emitted = %#v", out, emitted)
	}
	if !strings.Contains(client.prompt, "[User]: Hello how are you?") || !strings.Contains(client.prompt, "[Assistant]: I'm doing well.") {
		t.Fatalf("summary prompt = %q, want complete short turn", client.prompt)
	}
	marker, ok := markerInMessage(out[2])
	if !ok || marker.Recent != "" || marker.Reason != "manual" {
		t.Fatalf("marker = %#v, want manual checkpoint with no separate recent tail", marker)
	}
	if marker.Usage == nil || *marker.Usage != *usage {
		t.Fatalf("usage = %#v, want %#v", marker.Usage, usage)
	}
	facing := modelFacingMessages(out)
	if len(facing) != 1 || !strings.Contains(facing[0].Content[0].(models.TextPart).Text, "<conversation-checkpoint>") {
		t.Fatalf("model-facing history = %#v, want one checkpoint", facing)
	}
}

func TestRepeatedCompactionCarriesPreviousCheckpointState(t *testing.T) {
	p := New(WithKeepRecentTokens(1))
	part, err := newMarkerPart(MarkerPart{Version: 1, Summary: "previous summary", Recent: "[User]: prior recent"})
	if err != nil {
		t.Fatal(err)
	}
	messages := []models.Message{
		{Role: models.RoleUser, Content: models.Content{part}},
		{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "new request"}}},
		{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "new response"}}},
	}
	plan, ok := p.planContent(messages)
	if !ok {
		t.Fatal("expected compaction plan")
	}
	if plan.previousSummary != "previous summary" || len(plan.context) == 0 || plan.context[0] != "[User]: prior recent" {
		t.Fatalf("plan = %#v, want previous summary and recent context", plan)
	}
}

func TestKeepZeroTailSummarizesAllMessages(t *testing.T) {
	p := New(WithKeepTail(0))
	plan, ok := p.planContent([]models.Message{
		{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "request"}}},
		{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "response"}}},
	})
	if !ok {
		t.Fatal("expected compaction plan")
	}
	if plan.recent != "" || len(plan.context) != 1 || !strings.Contains(plan.context[0], "[Assistant]: response") {
		t.Fatalf("plan = %#v, want all messages summarized", plan)
	}
}

func TestSerializeToolOutputIsBoundedAndDoesNotEmbedImage(t *testing.T) {
	text := strings.Repeat("x", 2100)
	serialized := serializeMessage(models.Message{Role: models.RoleTool, Content: models.Content{
		models.ToolResultPart{Output: []models.Part{models.TextPart{Text: text}, models.ImagePart{Base64: "secret", MediaType: "image/png"}}},
	}})
	if strings.Contains(serialized, "secret") || !strings.Contains(serialized, "[truncated]") {
		t.Fatalf("serialized tool output = %q", serialized)
	}
}

func TestDefaultPromptRequiresCheckpointHeadings(t *testing.T) {
	prompt := buildSummaryPrompt(defaultSummaryPrompt, "", []string{"[User]: hello"})
	for _, heading := range []string{"## Objective", "## Important Details", "## Work State", "### Completed", "### Active", "### Blocked", "## Next Move", "## Relevant Files"} {
		if !strings.Contains(prompt, heading) {
			t.Fatalf("prompt missing %q", heading)
		}
	}
}

func TestCheckpointContextSurvivesMessageRoundTrip(t *testing.T) {
	usage := &models.Usage{InputTokens: 60300, OutputTokens: 1900, TotalTokens: 62200}
	part, err := newMarkerPart(MarkerPart{Version: 1, Reason: "manual", Summary: "summary", Recent: "[User]: recent", Usage: usage})
	if err != nil {
		t.Fatal(err)
	}
	messages := []models.Message{{Role: models.RoleUser, Content: models.Content{part}}}
	body, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}
	var restored []models.Message
	if err := json.Unmarshal(body, &restored); err != nil {
		t.Fatal(err)
	}
	marker, ok := markerInMessage(restored[0])
	if !ok || marker.Usage == nil || *marker.Usage != *usage {
		t.Fatalf("restored usage = %#v, want %#v", marker.Usage, usage)
	}
	facing := modelFacingMessages(restored)
	if len(facing) != 1 {
		t.Fatalf("model-facing history = %#v", facing)
	}
	text := facing[0].Content[0].(models.TextPart).Text
	if !strings.Contains(text, "<summary>\nsummary") || !strings.Contains(text, "<recent-context>\n[User]: recent") {
		t.Fatalf("checkpoint = %q", text)
	}
}

func TestSummaryOutputTokensFitsSmallContext(t *testing.T) {
	if got := summaryOutputTokens(models.ModelInfo{ContextWindow: 8}); got != 2 {
		t.Fatalf("summary output tokens = %d, want 2", got)
	}
}

type testClient struct {
	summary        string
	body           map[string]any
	maxOutput      int
	prompt         string
	usage          *models.Usage
	beforeGenerate func(context.Context) error
}

func (c *testClient) Prepare(_ context.Context, req models.Request) (*models.PreparedRequest, error) {
	if req.MaxOutputTokens > 0 || (len(req.Messages) == 1 && strings.Contains(serializeMessage(req.Messages[0]), "<conversation-checkpoint>")) {
		return &models.PreparedRequest{Body: map[string]any{"messages": req.Messages}}, nil
	}
	return &models.PreparedRequest{Body: c.body}, nil
}

func (c *testClient) Generate(ctx context.Context, req models.Request) (*models.Message, error) {
	if c.beforeGenerate != nil {
		if err := c.beforeGenerate(ctx); err != nil {
			return nil, err
		}
	}
	c.maxOutput = req.MaxOutputTokens
	if len(req.Messages) > 0 && len(req.Messages[0].Content) > 0 {
		if text, ok := req.Messages[0].Content[0].(models.TextPart); ok {
			c.prompt = text.Text
		}
	}
	return &models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: c.summary}}, Usage: c.usage}, nil
}

func TestCompactionPublishesLifecycleSnapshots(t *testing.T) {
	for _, outcome := range []string{"completed", "failed", "cancelled"} {
		t.Run(outcome, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			messages := []models.Message{{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "original context"}}}}
			var snapshots []models.Message
			client := &testClient{summary: "summary"}
			client.beforeGenerate = func(context.Context) error {
				if len(snapshots) != 1 || snapshots[0].State != models.MessageStateInProgress {
					t.Fatalf("before generate: %#v", snapshots)
				}
				pending := append(append([]models.Message(nil), messages...), snapshots[0])
				if findLatestMarker(pending) != -1 || len(modelFacingMessages(pending)) != 1 {
					t.Fatal("in-progress compaction replaced the model context")
				}
				if outcome == "cancelled" {
					cancel()
					return ctx.Err()
				}
				if outcome == "failed" {
					return errors.New("provider unavailable")
				}
				return nil
			}
			out, err := New(WithKeepTail(0)).compact(ctx, run.TransformHistoryInfo{
				Messages: messages, Client: client, Model: models.ModelRef{Provider: "test", ID: "model"}, ModelInfo: models.ModelInfo{ContextWindow: 4096},
				Sink: run.SinkFunc(func(event run.Event) {
					if message, ok := event.(run.MessageEvent); ok {
						snapshots = append(snapshots, message.Message)
					}
				}),
			}, true)
			if len(snapshots) != 2 {
				t.Fatalf("snapshots = %#v", snapshots)
			}
			first, last := snapshots[0], snapshots[1]
			if first.ID == "" || first.ID != last.ID || first.Revision != 1 || last.Revision != 2 || models.PartID(first.Content[0]) != models.PartID(last.Content[0]) {
				t.Fatalf("unstable lifecycle: %#v, %#v", first, last)
			}
			if outcome == "completed" {
				if err != nil || last.State != models.MessageStateCompleted || len(out) != 2 || out[1].Revision != last.Revision {
					t.Fatalf("out=%#v state=%s err=%v", out, last.State, err)
				}
			} else if err == nil || last.State != models.MessageStateFailed {
				t.Fatalf("state=%s err=%v", last.State, err)
			}
		})
	}
}

func (*testClient) Stream(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
	return nil, errors.New("unexpected Stream")
}
