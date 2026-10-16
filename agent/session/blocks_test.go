package session

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/protocols"
	"github.com/chaserensberger/wingman/models/route"
	"github.com/chaserensberger/wingman/store"
)

type blockTransport func(context.Context, route.Route, *models.PreparedRequest) (*http.Response, error)

func (f blockTransport) Open(ctx context.Context, r route.Route, req *models.PreparedRequest) (*http.Response, error) {
	return f(ctx, r, req)
}

func TestProviderBlocksSurviveStreamingAndHydration(t *testing.T) {
	frames := []string{
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"first"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"signature-1"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":"before"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"call_1","name":"lookup","input":{}}}`,
		`{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"key\":1}"}}`,
		`{"type":"content_block_stop","index":2}`,
		`{"type":"content_block_start","index":3,"content_block":{"type":"thinking","thinking":"","signature":"signature-2"}}`,
		`{"type":"content_block_stop","index":3}`,
		`{"type":"content_block_start","index":4,"content_block":{"type":"text","text":"after"}}`,
		`{"type":"content_block_delta","index":4,"delta":{"type":"text_delta","text":"!"}}`,
		`{"type":"content_block_stop","index":4}`,
	}
	for _, backend := range []string{"memory", "sqlite"} {
		for _, interrupted := range []bool{false, true} {
			name := backend + "/completed"
			if interrupted {
				name = backend + "/interrupted"
			}
			t.Run(name, func(t *testing.T) {
				data := historyStore(t, backend)
				if err := data.CreateSession(&store.Session{ID: "ses_blocks"}); err != nil {
					t.Fatal(err)
				}
				body := "data: " + strings.Join(frames, "\n\ndata: ") + "\n\n"
				if !interrupted {
					body += "data: {\"type\":\"message_stop\"}\n\n"
				}
				requests := 0
				model := &route.Model{
					Info_: models.ModelInfo{Provider: "anthropic", ID: "test", API: models.APIAnthropicMessages},
					Route: route.Route{Protocol: protocols.Anthropic{}, Framing: route.SSE, Transport: blockTransport(func(context.Context, route.Route, *models.PreparedRequest) (*http.Response, error) {
						requests++
						response := body
						if requests > 1 {
							response = "data: {\"type\":\"message_stop\"}\n\n"
						}
						return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(response))}, nil
					})},
				}
				sess := New(WithID("ses_blocks"), WithStore(data), WithClient(model), WithModelRef(models.ModelRef{Provider: "anthropic", ID: "test"}, model.Info_))
				t.Cleanup(func() { _ = sess.Close(context.Background()) })
				stream, err := sess.RunStream(t.Context(), "hello")
				if err != nil {
					t.Fatal(err)
				}
				var ids []string
				liveText := map[string]string{}
				liveSignatures := map[string]string{}
				for stream.Next() {
					event, ok := stream.Event().Data.(run.StreamPartEvent)
					if !ok || event.Step != 1 {
						continue
					}
					switch part := event.Part.(type) {
					case models.TextStartPart, models.ReasoningStartPart, models.ToolInputStartPart:
						ids = append(ids, event.PartID)
					case models.TextDeltaPart:
						liveText[event.PartID] += part.Delta
					case models.ReasoningDeltaPart:
						if part.Encrypted != "" {
							liveSignatures[event.PartID] = part.Encrypted
						}
					}
				}
				if (stream.Err() != nil) != interrupted {
					t.Fatalf("stream error = %v, interrupted = %v", stream.Err(), interrupted)
				}
				message := sess.History()[1]
				if len(message.Content) != 5 || len(ids) != 5 {
					t.Fatalf("content = %#v, streamed IDs = %v", message.Content, ids)
				}
				for i, kind := range []string{"reasoning", "text", "tool", "reasoning", "text"} {
					if part := message.Content[i]; part.Type() != kind || ids[i] == "" || models.PartID(part) != ids[i] {
						t.Fatalf("part %d = %#v, streamed ID = %q", i, part, ids[i])
					}
				}
				for i, signature := range map[int]string{0: "signature-1", 3: "signature-2"} {
					if part := message.Content[i].(models.ReasoningPart); part.Encrypted != signature || liveSignatures[part.ID] != signature {
						t.Fatalf("reasoning = %#v, streamed signatures = %v", part, liveSignatures)
					}
				}
				for i, text := range map[int]string{1: "before", 4: "after!"} {
					if part := message.Content[i].(models.TextPart); part.Text != text || liveText[part.ID] != text {
						t.Fatalf("text = %#v, streamed text = %v", part, liveText)
					}
				}
				if part := message.Content[2].(models.ToolPart); part.InputRaw != `{"key":1}` {
					t.Fatalf("tool = %#v", part)
				}
				reopened := New(WithID("ses_blocks"), WithStore(data))
				if err := reopened.hydrate(t.Context()); err != nil {
					t.Fatal(err)
				}
				reloaded := reopened.History()[1]
				if !reflect.DeepEqual(message.Content, reloaded.Content) {
					t.Fatalf("reloaded = %#v, want %#v", reloaded.Content, message.Content)
				}
				before, err := model.Prepare(t.Context(), models.Request{Messages: []models.Message{message}})
				if err != nil {
					t.Fatal(err)
				}
				after, err := model.Prepare(t.Context(), models.Request{Messages: []models.Message{reloaded}})
				if err != nil || !reflect.DeepEqual(before.Body, after.Body) {
					t.Fatalf("replay changed after hydration: %v", err)
				}
			})
		}
	}
}
