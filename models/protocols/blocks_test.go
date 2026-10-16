package protocols

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/route"
)

func parseBlocks(t *testing.T, protocol route.Protocol, frames ...string) (*models.Message, []models.StreamPart) {
	t.Helper()
	parser := protocol.NewParser(models.ModelInfo{})
	var events []models.StreamPart
	for _, frame := range frames {
		parts, err := parser.Step(route.Frame{Data: frame})
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, parts...)
	}
	return parser.Message(), events
}

func TestAnthropicPreservesContentBlocks(t *testing.T) {
	message, events := parseBlocks(t, Anthropic{},
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"first"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-1"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":"before"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"call","name":"lookup","input":{}}}`,
		`{"type":"content_block_stop","index":2}`,
		`{"type":"content_block_start","index":3,"content_block":{"type":"thinking","thinking":"","signature":"sig-2"}}`,
		`{"type":"content_block_stop","index":3}`,
		`{"type":"content_block_start","index":4,"content_block":{"type":"text","text":"after"}}`,
		`{"type":"content_block_delta","index":4,"delta":{"type":"text_delta","text":"!"}}`,
		`{"type":"content_block_stop","index":4}`,
		`{"type":"message_stop"}`,
	)
	want := models.Content{
		models.ReasoningPart{Reasoning: "first", Encrypted: "sig-1"},
		models.TextPart{Text: "before"},
		models.ToolCallPart{CallID: "call", Name: "lookup", Input: map[string]any{}},
		models.ReasoningPart{Encrypted: "sig-2"},
		models.TextPart{Text: "after!"},
	}
	if !reflect.DeepEqual(message.Content, want) {
		t.Fatalf("content = %#v, want %#v", message.Content, want)
	}
	starts, ends := map[string]int{}, map[string]int{}
	for _, event := range events {
		switch p := event.(type) {
		case models.TextStartPart:
			starts[p.ID]++
		case models.ReasoningStartPart:
			starts[p.ID]++
		case models.TextEndPart:
			ends[p.ID]++
		case models.ReasoningEndPart:
			ends[p.ID]++
		}
	}
	for _, id := range []string{"reasoning-0", "text-1", "reasoning-3", "text-4"} {
		if starts[id] != 1 || ends[id] != 1 {
			t.Errorf("block %s: starts=%d ends=%d", id, starts[id], ends[id])
		}
	}
	blocks := anthropicBlocks(message.Content)
	for i, kind := range []string{"thinking", "text", "tool_use", "thinking", "text"} {
		if blocks[i].(map[string]any)["type"] != kind {
			t.Fatalf("replay block %d = %#v", i, blocks[i])
		}
	}
}

func TestResponsesPreservesItemsAndReplayOrder(t *testing.T) {
	message, events := parseBlocks(t, Responses{},
		`{"type":"response.output_item.added","item":{"type":"reasoning","id":"rs_1"}}`,
		`{"type":"response.reasoning_summary_text.delta","item_id":"rs_1","delta":"first"}`,
		`{"type":"response.output_item.done","item":{"type":"reasoning","id":"rs_1","encrypted_content":"encrypted-1"}}`,
		`{"type":"response.content_part.added","item_id":"msg_1","content_index":0,"part":{"type":"output_text","text":""}}`,
		`{"type":"response.output_text.delta","item_id":"msg_1","content_index":0,"delta":"before"}`,
		`{"type":"response.content_part.done","item_id":"msg_1","content_index":0,"part":{"type":"output_text"}}`,
		`{"type":"response.output_item.added","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup"}}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"x\":"}`,
		`{"type":"response.output_item.added","item":{"type":"function_call","id":"fc_2","call_id":"call_2","name":"lookup"}}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_2","delta":"{}"}`,
		`{"type":"response.output_item.done","item":{"type":"function_call","id":"fc_2","call_id":"call_2","name":"lookup","arguments":"{}"}}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"1}"}`,
		`{"type":"response.output_item.done","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup","arguments":"{\"x\":1}"}}`,
		`{"type":"response.output_item.added","item":{"type":"reasoning","id":"rs_2"}}`,
		`{"type":"response.output_item.done","item":{"type":"reasoning","id":"rs_2","encrypted_content":"encrypted-2"}}`,
		`{"type":"response.output_text.delta","item_id":"msg_2","content_index":0,"delta":"after"}`,
		`{"type":"response.output_text.delta","item_id":"msg_2","content_index":1,"delta":"another block"}`,
		`{"type":"response.completed"}`,
	)
	want := models.Content{
		models.ReasoningPart{Reasoning: "first", Encrypted: "encrypted-1", ProviderMetadata: models.Meta{"openai": map[string]any{"item_id": "rs_1", "reasoning_encrypted_content": "encrypted-1"}}},
		models.TextPart{Text: "before"},
		models.ToolCallPart{CallID: "call_1", Name: "lookup", Input: map[string]any{"x": float64(1)}},
		models.ToolCallPart{CallID: "call_2", Name: "lookup", Input: map[string]any{}},
		models.ReasoningPart{Encrypted: "encrypted-2", ProviderMetadata: models.Meta{"openai": map[string]any{"item_id": "rs_2", "reasoning_encrypted_content": "encrypted-2"}}},
		models.TextPart{Text: "after"},
		models.TextPart{Text: "another block"},
	}
	if !reflect.DeepEqual(message.Content, want) {
		t.Fatalf("content = %#v, want %#v", message.Content, want)
	}
	var toolStarts []string
	arguments := map[string]string{}
	for _, event := range events {
		switch p := event.(type) {
		case models.ToolInputStartPart:
			toolStarts = append(toolStarts, p.ID)
		case models.ToolInputDeltaPart:
			arguments[p.ID] += p.Delta
		}
	}
	if !reflect.DeepEqual(toolStarts, []string{"call_1", "call_2"}) || arguments["call_1"] != `{"x":1}` || arguments["call_2"] != `{}` {
		t.Fatalf("tool starts = %v, arguments = %v", toolStarts, arguments)
	}
	body, err := (Responses{}).Body(models.ModelInfo{}, models.Request{Messages: []models.Message{*message}})
	if err != nil {
		t.Fatal(err)
	}
	input := body["input"].([]any)
	if len(input) != 6 {
		t.Fatalf("replay = %#v", input)
	}
	for i, kind := range []string{"reasoning", "assistant", "function_call", "function_call", "reasoning", "assistant"} {
		item := input[i].(map[string]any)
		if item["type"] != kind && item["role"] != kind {
			t.Errorf("replay item %d = %#v, want %s", i, item, kind)
		}
	}
	if len(input[5].(map[string]any)["content"].([]any)) != 2 {
		t.Fatal("replay merged separate text blocks")
	}
}

func TestGeminiKeepsSignaturesOnTheirBlocks(t *testing.T) {
	message, _ := parseBlocks(t, Gemini{},
		`{"candidates":[{"content":{"parts":[{"text":"think","thought":true,"thoughtSignature":"reason-signature"}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"text":"before","thoughtSignature":"text-signature"}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"text":"!"}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"text":"?"}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup","args":{}},"thoughtSignature":"tool-signature"},{"text":"after"}]},"finishReason":"STOP"}]}`,
	)
	if len(message.Content) != 5 {
		t.Fatalf("content = %#v", message.Content)
	}
	if p := message.Content[0].(models.ReasoningPart); p.Encrypted != "reason-signature" || p.Reasoning != "think" {
		t.Fatalf("reasoning = %#v", p)
	}
	if p := message.Content[1].(models.TextPart); p.Text != "before" {
		t.Fatalf("text = %#v", p)
	}
	body, err := (Gemini{}).Body(models.ModelInfo{}, models.Request{Messages: []models.Message{*message}})
	if err != nil {
		t.Fatal(err)
	}
	parts := body["contents"].([]any)[0].(map[string]any)["parts"].([]any)
	for i, signature := range map[int]string{0: "reason-signature", 1: "text-signature", 3: "tool-signature"} {
		if parts[i].(map[string]any)["thoughtSignature"] != signature {
			t.Errorf("part %d = %#v, want signature %s", i, parts[i], signature)
		}
	}
	if parts[2].(map[string]any)["text"] != "!?" || parts[2].(map[string]any)["thoughtSignature"] != nil || parts[4].(map[string]any)["thoughtSignature"] != nil {
		t.Fatal("signature leaked to later text")
	}
}

func TestGeminiSignedPartBoundaries(t *testing.T) {
	for _, tt := range []struct {
		name, parts string
	}{
		{"trailing signature", `[{"text":"answer"},{"text":"","thoughtSignature":"sig"}]`},
		{"adjacent signatures", `[{"text":"first","thoughtSignature":"sig-1"},{"text":"second","thoughtSignature":"sig-2"}]`},
		{"signed then unsigned", `[{"text":"signed","thoughtSignature":"sig"},{"text":"unsigned"}]`},
	} {
		for _, thought := range []bool{false, true} {
			for _, split := range []bool{false, true} {
				t.Run(tt.name+"/thought="+encodeJSON(thought)+"/split="+encodeJSON(split), func(t *testing.T) {
					var want []any
					if err := json.Unmarshal([]byte(tt.parts), &want); err != nil {
						t.Fatal(err)
					}
					if thought {
						for _, part := range want {
							part.(map[string]any)["thought"] = true
							if part.(map[string]any)["thoughtSignature"] == nil {
								part.(map[string]any)["thoughtSignature"] = ""
							}
						}
					}
					frame := func(parts []any) string {
						return `{"candidates":[{"content":{"parts":` + encodeJSON(parts) + `}}]}`
					}
					frames := []string{frame(want)}
					if split {
						frames = nil
						for _, part := range want {
							frames = append(frames, frame([]any{part}))
						}
					}
					frames = append(frames, `{"candidates":[{"finishReason":"STOP"}]}`)
					message, events := parseBlocks(t, Gemini{}, frames...)
					body, err := (Gemini{}).Body(models.ModelInfo{}, models.Request{Messages: []models.Message{*message}})
					if err != nil {
						t.Fatal(err)
					}
					got := body["contents"].([]any)[0].(map[string]any)["parts"]
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("replay = %#v, want %#v", got, want)
					}
					starts, ends := 0, 0
					for _, event := range events {
						switch event.(type) {
						case models.TextStartPart, models.ReasoningStartPart:
							starts++
						case models.TextEndPart, models.ReasoningEndPart:
							ends++
						}
					}
					if starts != len(want) || ends != len(want) {
						t.Fatalf("starts=%d ends=%d, want %d each", starts, ends, len(want))
					}
				})
			}
		}
	}
}

func TestAnthropicReplayOmitsEmptyText(t *testing.T) {
	for _, text := range []string{"", " \n\t"} {
		t.Run(encodeJSON(text), func(t *testing.T) {
			message, _ := parseBlocks(t, Anthropic{},
				`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":`+encodeJSON(text)+`}}`,
				`{"type":"content_block_stop","index":0}`,
				`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call","name":"lookup","input":{}}}`,
				`{"type":"content_block_stop","index":1}`,
				`{"type":"message_stop"}`,
			)
			body, err := (Anthropic{}).Body(models.ModelInfo{}, models.Request{Messages: []models.Message{*message}})
			if err != nil {
				t.Fatal(err)
			}
			blocks := body["messages"].([]any)[0].(map[string]any)["content"].([]any)
			if len(blocks) != 1 || blocks[0].(map[string]any)["type"] != "tool_use" {
				t.Fatalf("replay = %#v", blocks)
			}
			if len(message.Content) != 2 || message.Content[0].(models.TextPart).Text != text {
				t.Fatalf("durable content changed: %#v", message.Content)
			}
		})
	}
}

func TestAnthropicRedactedBlockReplay(t *testing.T) {
	message, _ := parseBlocks(t, Anthropic{},
		`{"type":"content_block_start","index":0,"content_block":{"type":"redacted_thinking","data":"opaque"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_stop"}`,
	)
	if len(message.Content) != 1 {
		t.Fatalf("content = %#v", message.Content)
	}
	want := []any{map[string]any{"type": "redacted_thinking", "data": "opaque"}}
	if got := anthropicBlocks(message.Content); !reflect.DeepEqual(got, want) {
		t.Fatalf("replay = %#v", got)
	}
}

func TestChatPreservesAlternatingContent(t *testing.T) {
	message, _ := parseBlocks(t, Chat{},
		`{"choices":[{"delta":{"reasoning_content":"first"}}]}`,
		`{"choices":[{"delta":{"content":"answer"}}]}`,
		`{"choices":[{"delta":{"reasoning_content":"second"}}]}`,
		`{"choices":[{"delta":{"content":"more"},"finish_reason":"stop"}]}`,
		`[DONE]`,
	)
	want := models.Content{
		models.ReasoningPart{Reasoning: "first"}, models.TextPart{Text: "answer"},
		models.ReasoningPart{Reasoning: "second"}, models.TextPart{Text: "more"},
	}
	if !reflect.DeepEqual(message.Content, want) {
		t.Fatalf("content = %#v", message.Content)
	}
}

func TestChatReservesToolBlockBeforeArguments(t *testing.T) {
	message, _ := parseBlocks(t, Chat{},
		`{"choices":[{"delta":{"content":"before"}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call","function":{"name":"lookup","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{"content":"after"}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`,
		`[DONE]`,
	)
	want := models.Content{
		models.TextPart{Text: "before"},
		models.ToolCallPart{CallID: "call", Name: "lookup", Input: map[string]any{}},
		models.TextPart{Text: "after"},
	}
	if !reflect.DeepEqual(message.Content, want) {
		t.Fatalf("content = %#v", message.Content)
	}
}

func TestChatToolIndexOrderAcrossFrames(t *testing.T) {
	message, _ := parseBlocks(t, Chat{},
		`{"choices":[{"delta":{"content":"before"}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"second","function":{"name":"b","arguments":"{}"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"first","function":{"name":"a","arguments":"{}"}}]}}]}`,
		`{"choices":[{"delta":{"content":"after"},"finish_reason":"tool_calls"}]}`,
		`[DONE]`,
	)
	if len(message.Content) != 4 || message.Content[0].(models.TextPart).Text != "before" || message.Content[3].(models.TextPart).Text != "after" {
		t.Fatalf("content = %#v", message.Content)
	}
	calls := toolCalls(message.Content)
	if calls[0].CallID != "first" || calls[1].CallID != "second" {
		t.Fatalf("tool order = %#v", calls)
	}
}
