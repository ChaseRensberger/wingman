package run

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/chaserensberger/wingman/models"
)

func TestModelInputRecordsTransformedContextBeforeDispatch(t *testing.T) {
	var recorded *models.CallInput
	client := &modelCallTestClient{stream: func(_ context.Context, req models.Request) (*models.EventStream[models.StreamPart, *models.Message], error) {
		if recorded == nil || !reflect.DeepEqual(recorded.Messages, req.Messages) || recorded.System != req.System || !reflect.DeepEqual(recorded.Instructions, req.Instructions) {
			t.Fatalf("recorded input differs from dispatched context: %#v", recorded)
		}
		return completedStream(models.Message{Role: models.RoleAssistant}), nil
	}}
	_, err := Run(t.Context(), Config{
		Client: client, Model: testModel, System: "Instructions at dispatch", Messages: []models.Message{models.NewUserText("original")},
		ResolveInstructions: func(_ context.Context, system string, _ []models.Message) (string, []models.InstructionSnapshot, error) {
			return system + "\nLive guidance", []models.InstructionSnapshot{{Path: "/repo/AGENTS.md", Content: "Live guidance", SHA256: "source", ToolUseID: "tlu_read"}}, nil
		},
		Hooks: Hooks{TransformContext: func(context.Context, TransformContextInfo) ([]models.Message, error) {
			return []models.Message{models.NewUserText("transformed summary")}, nil
		}},
		ModelCallLifecycle: modelCallLifecycleFuncs{start: func(_ context.Context, info ModelCallStartInfo) (string, error) {
			if len(info.Trace.Instructions) != 1 || info.Trace.Instructions[0].Content != "" || info.Trace.Instructions[0].ToolUseID != "tlu_read" {
				t.Fatalf("trace provenance: %#v", info.Trace.Instructions)
			}
			var err error
			recorded, err = models.ResolveCallInput(info.Trace.InputHash, func(hash string) (json.RawMessage, error) { return info.InputBlobs[hash], nil })
			return "mcl_input", err
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if recorded.Messages[0].Content[0].(models.TextPart).Text != "transformed summary" {
		t.Fatal("recorded stored history instead of transformed context")
	}
}
