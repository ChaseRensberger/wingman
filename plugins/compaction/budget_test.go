package compaction

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/models"
)

type budgetClient struct {
	testClient
	requests  []models.Request
	truncated bool
}

func (c *budgetClient) Prepare(_ context.Context, req models.Request) (*models.PreparedRequest, error) {
	return &models.PreparedRequest{Body: map[string]any{"system": req.System, "messages": req.Messages, "tools": req.Tools}}, nil
}

func (c *budgetClient) Generate(_ context.Context, req models.Request) (*models.Message, error) {
	c.requests = append(c.requests, req)
	text := c.summary
	if text == "" {
		text = "summary"
	}
	result := &models.Message{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: text}}}
	if c.truncated {
		result.FinishReason = models.FinishReasonMaxTokens
	}
	return result, nil
}

func TestSummaryRequestBudgetAndCallLimit(t *testing.T) {
	for _, tooLarge := range []bool{false, true} {
		client := &budgetClient{}
		text := strings.Repeat("界", 8000)
		if tooLarge {
			text = strings.Repeat(text, 30)
		}
		_, err := summarizeBounded(t.Context(), client, models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{ContextWindow: 2048}, "Summarize.", "", []string{text}, 128)
		if (err != nil) != tooLarge {
			t.Fatalf("tooLarge=%v err=%v", tooLarge, err)
		}
		if len(client.requests) < 2 || len(client.requests) > 8 || (tooLarge && len(client.requests) != 8) {
			t.Fatalf("calls=%d", len(client.requests))
		}
		var consumed strings.Builder
		for i, req := range client.requests {
			if approxRequestTokens(t.Context(), client, req)+req.MaxOutputTokens > 2048 {
				t.Fatal("summary request exceeds budget")
			}
			prompt := req.Messages[0].Content[0].(models.TextPart).Text
			if !utf8.ValidString(prompt) {
				t.Fatal("split UTF-8")
			}
			_, chunk, _ := strings.Cut(prompt, "The following is the conversation history:\n\n")
			consumed.WriteString(chunk)
			if i > 0 && !strings.Contains(prompt, "<previous-summary>\nsummary") {
				t.Fatal("lost running summary")
			}
		}
		if !tooLarge && consumed.String() != text {
			t.Fatal("summary input lost or duplicated")
		}
	}
}

func TestCompactionRejectsUnusableBudgets(t *testing.T) {
	for _, name := range []string{"overhead", "checkpoint", "truncated"} {
		t.Run(name, func(t *testing.T) {
			client := &budgetClient{}
			msgs := []models.Message{{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: strings.Repeat("old", 1000)}}}, {Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "recent"}}}}
			info := run.TransformHistoryInfo{Messages: msgs, Client: client, Model: models.ModelRef{Provider: "test", ID: "model"}, ModelInfo: models.ModelInfo{ContextWindow: 4096, MaxOutput: 256}}
			if name == "overhead" {
				info.Request.System = strings.Repeat("system", 4000)
			}
			if name == "checkpoint" {
				client.summary = strings.Repeat("summary", 5000)
			}
			if name == "truncated" {
				client.truncated = true
			}
			emitted := false
			info.Sink = run.SinkFunc(func(run.Event) { emitted = true })
			_, err := New(WithKeepTail(1)).compact(t.Context(), info, true)
			if err == nil || emitted {
				t.Fatalf("err=%v emitted=%v", err, emitted)
			}
			if name == "overhead" && len(client.requests) != 0 {
				t.Fatal("called model with impossible overhead")
			}
		})
	}
}

func TestOverflowCompactionShrinksCheckpoint(t *testing.T) {
	client := &budgetClient{}
	msgs := []models.Message{{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: strings.Repeat("old ", 1000)}}}, {Role: models.RoleUser, Content: models.Content{models.TextPart{Text: strings.Repeat("recent ", 1000)}}}}
	info := run.TransformHistoryInfo{ContextOverflow: true, Messages: msgs, Client: client, Model: models.ModelRef{Provider: "test", ID: "model"}, ModelInfo: models.ModelInfo{ContextWindow: 8192, MaxOutput: 256}, Request: models.Request{Messages: msgs}}
	out, err := New(WithKeepTail(1)).transformHistory(t.Context(), info)
	if err != nil {
		t.Fatal(err)
	}
	req := info.Request
	req.Messages = modelFacingMessages(out)
	if approxRequestTokens(t.Context(), client, req) > approxRequestTokens(t.Context(), client, info.Request)*3/4 {
		t.Fatal("overflow checkpoint did not shrink")
	}
	marker, _ := markerInMessage(out[len(out)-1])
	if marker.Reason != "overflow" {
		t.Fatal("missing overflow reason")
	}
}

func TestOverflowRecompactsRejectedCheckpoint(t *testing.T) {
	for _, summaryOnly := range []bool{false, true} {
		client := &budgetClient{}
		previous := MarkerPart{Version: 1, Summary: "existing summary", Recent: strings.Repeat("recent conversation ", 1000), OriginalCount: 20}
		if summaryOnly {
			previous.Summary, previous.Recent = previous.Recent, ""
		}
		part, err := newMarkerPart(previous)
		if err != nil {
			t.Fatal(err)
		}
		messages := []models.Message{{Role: models.RoleUser, Content: models.Content{part}}, {Role: models.RoleAssistant, State: models.MessageStateFailed}}
		info := run.TransformHistoryInfo{ContextOverflow: true, Messages: messages, Client: client, Model: models.ModelRef{Provider: "test", ID: "model"}, ModelInfo: models.ModelInfo{ContextWindow: 32768, MaxOutput: 4096}, Request: models.Request{Messages: messages}}
		out, err := New().transformHistory(t.Context(), info)
		if err != nil {
			t.Fatal(err)
		}
		marker, ok := markerInMessage(out[len(out)-1])
		if !ok || marker.Reason != "overflow" || marker.OriginalCount != previous.OriginalCount {
			t.Fatalf("marker=%#v", marker)
		}
		prompt := client.requests[0].Messages[0].Content[0].(models.TextPart).Text
		if !strings.Contains(prompt, previous.Summary) || !strings.Contains(prompt, previous.Recent) {
			t.Fatal("lost prior checkpoint content")
		}
		check := info.Request
		check.Messages = modelFacingMessages(out)
		before := info.Request
		before.Messages = modelFacingMessages(messages)
		if approxRequestTokens(t.Context(), client, check) >= approxRequestTokens(t.Context(), client, before) {
			t.Fatal("checkpoint did not shrink")
		}
	}
}
