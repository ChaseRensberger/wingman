package run

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/chaserensberger/wingman/models"
)

func TestTransformHistoryAdoptsReturnedHistory(t *testing.T) {
	original := []models.Message{{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "original"}}}}
	replacement := []models.Message{{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "replacement"}}}}
	metadataOnly := []models.Message{original[0]}
	metadataOnly[0].Metadata = models.Meta{"reviewed": true}
	for _, tt := range []struct {
		name    string
		output  []models.Message
		want    []models.Message
		changed bool
	}{
		{name: "same length replacement", output: replacement, want: replacement, changed: true},
		{name: "metadata only", output: metadataOnly, want: metadataOnly, changed: true},
		{name: "nil", want: original},
		{name: "empty", output: []models.Message{}, want: []models.Message{}, changed: true},
		{name: "unchanged", output: original, want: original},
		{name: "equal copy", output: append([]models.Message(nil), original...), want: original},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var events []ContextTransformedEvent
			contextCalls, requests := 0, 0
			client := &modelCallTestClient{stream: func(_ context.Context, req models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
				requests++
				if !reflect.DeepEqual(req.Messages, tt.want) {
					t.Fatalf("request history = %#v, want %#v", req.Messages, tt.want)
				}
				return completedStream(models.Message{Role: models.RoleAssistant}), nil
			}}
			result, err := Run(t.Context(), Config{
				Client: client, Model: testModel, Messages: original,
				Hooks: Hooks{
					TransformHistory: func(_ context.Context, info TransformHistoryInfo) ([]models.Message, error) {
						if !reflect.DeepEqual(info.Messages, original) || !reflect.DeepEqual(info.Request.Messages, original) {
							t.Fatalf("hook input = %#v", info)
						}
						return tt.output, nil
					},
					TransformContext: func(_ context.Context, info TransformContextInfo) ([]models.Message, error) {
						contextCalls++
						if len(info.Messages) != len(tt.want) || (len(tt.want) > 0 && !reflect.DeepEqual(info.Messages, tt.want)) {
							t.Fatalf("context history = %#v, want %#v", info.Messages, tt.want)
						}
						return nil, nil
					},
				},
				Sink: SinkFunc(func(event Event) {
					if event, ok := event.(ContextTransformedEvent); ok {
						events = append(events, event)
					}
				}),
			})
			if err != nil {
				t.Fatal(err)
			}
			if requests != 1 || contextCalls != 1 || len(result.Messages) != len(tt.want)+1 || !reflect.DeepEqual(result.Messages[:len(tt.want)], tt.want) {
				t.Fatalf("requests=%d context calls=%d result=%#v", requests, contextCalls, result)
			}
			if (len(events) == 1) != tt.changed || len(events) > 1 {
				t.Fatalf("events = %#v, changed = %v", events, tt.changed)
			}
			if tt.changed {
				event := events[0]
				if event.Step != 1 || event.Phase != "before_step" || event.OriginalCount != len(original) || event.NewCount != len(tt.want) {
					t.Fatalf("event = %#v", event)
				}
				if len(tt.want) == 0 {
					if event.Head != nil {
						t.Fatalf("empty transform head = %#v", event.Head)
					}
				} else if event.Head == nil || !reflect.DeepEqual(*event.Head, tt.want[0]) {
					t.Fatalf("replacement head = %#v", event.Head)
				}
			}
			if original[0].Content[0].(models.TextPart).Text != "original" {
				t.Fatal("transform mutated Config.Messages")
			}
		})
	}
}

func TestTransformHistoryReplacementSurvivesNextStep(t *testing.T) {
	requests, hooks := 0, 0
	replacement := models.Message{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "replacement"}}}
	client := &modelCallTestClient{stream: func(_ context.Context, req models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
		requests++
		if !reflect.DeepEqual(req.Messages[0], replacement) {
			t.Fatalf("step %d history = %#v", requests, req.Messages)
		}
		message := models.Message{Role: models.RoleAssistant}
		if requests == 1 {
			message.Content = models.Content{models.ToolCallPart{CallID: "call_1", Name: "unknown", Input: map[string]any{}}}
		}
		return completedStream(message), nil
	}}
	result, err := Run(t.Context(), Config{
		Client: client, Model: testModel,
		Messages: []models.Message{{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "original"}}}},
		Hooks: Hooks{TransformHistory: func(_ context.Context, info TransformHistoryInfo) ([]models.Message, error) {
			hooks++
			if info.Step == 1 {
				return []models.Message{replacement}, nil
			}
			if !reflect.DeepEqual(info.Messages[0], replacement) {
				t.Fatalf("later hook history = %#v", info.Messages)
			}
			return nil, nil
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || hooks != 2 || !reflect.DeepEqual(result.Messages[0], replacement) {
		t.Fatalf("requests=%d hooks=%d result=%#v", requests, hooks, result)
	}
}

func TestTransformHistoryErrorRetainsHistory(t *testing.T) {
	failure := errors.New("transform failed")
	original := []models.Message{{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "original"}}}}
	result, err := Run(t.Context(), Config{
		Client: &modelCallTestClient{stream: func(context.Context, models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
			t.Fatal("provider dispatched after transform error")
			return nil, nil
		}},
		Model: testModel, Messages: original,
		Hooks: Hooks{TransformHistory: func(context.Context, TransformHistoryInfo) ([]models.Message, error) {
			return []models.Message{}, failure
		}},
	})
	if !errors.Is(err, failure) || !reflect.DeepEqual(result.Messages, original) || result.StopReason != StopReasonError {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}
