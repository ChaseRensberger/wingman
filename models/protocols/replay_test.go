package protocols

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/route"
)

func replayModel(protocol route.Protocol) *route.Model {
	api := models.API(protocol.ID())
	if _, ok := protocol.(Chat); ok {
		api = models.APIOpenAICompletions
	}
	return &route.Model{Info_: models.ModelInfo{Provider: "test", ID: "model", API: api, Capabilities: models.ModelCapabilities{Images: true}}, Route: route.Route{Protocol: protocol}}
}

func TestReplayReasoningOrigin(t *testing.T) {
	for _, protocol := range []route.Protocol{Anthropic{}, Responses{}, Gemini{}} {
		for _, change := range []string{"same", "provider", "api", "model", "unknown"} {
			t.Run(protocol.ID()+"/"+change, func(t *testing.T) {
				model := replayModel(protocol)
				origin := &models.MessageOrigin{Provider: model.Info_.Provider, API: model.Info_.API, ModelID: model.Info_.ID}
				switch change {
				case "provider":
					origin.Provider = "other"
				case "api":
					origin.API = "other"
				case "model":
					origin.ModelID = "other"
				case "unknown":
					origin = nil
				}
				metadata := models.Meta{"google": map[string]any{"thought_signature": "private-signature"}, "openai": map[string]any{"item_id": "rs_1", "reasoning_encrypted_content": "private-signature"}}
				history := []models.Message{{Role: models.RoleAssistant, Origin: origin, Content: models.Content{
					models.ReasoningPart{Reasoning: "private-reasoning", Encrypted: "private-signature", ProviderMetadata: metadata},
					models.TextPart{Text: "visible-answer", ProviderMetadata: metadata},
					models.ToolPart{CallID: "call_1", Name: "lookup", State: models.ToolStateCompleted, Input: map[string]any{}, Output: "result", ProviderMetadata: metadata},
				}}}
				before := encodeJSON(history)
				prepared, err := model.Prepare(t.Context(), models.Request{Messages: history})
				if err != nil {
					t.Fatal(err)
				}
				wire := encodeJSON(prepared.Body)
				for _, secret := range []string{"private-reasoning", "private-signature"} {
					if strings.Contains(wire, secret) != (change == "same") {
						t.Fatalf("%s replay: %s", secret, wire)
					}
				}
				if !strings.Contains(wire, "visible-answer") || !strings.Contains(wire, "result") || encodeJSON(history) != before {
					t.Fatalf("lost visible content or mutated history: %s", wire)
				}
				if _, google := protocol.(Gemini); google && change != "same" && !strings.Contains(wire, "skip_thought_signature_validator") {
					t.Fatalf("foreign Gemini tool call has no replay marker: %s", wire)
				}
			})
		}
	}
}

func TestReplayToolCallIDs(t *testing.T) {
	for _, protocol := range []route.Protocol{Anthropic{}, Responses{}, Chat{}, Gemini{}} {
		t.Run(protocol.ID(), func(t *testing.T) {
			model := replayModel(protocol)
			var history []models.Message
			for _, id := range []string{"call_1", "call_1", "a/b", "a?b", strings.Repeat("x", 90), ""} {
				history = append(history, models.Message{Role: models.RoleAssistant, Content: models.Content{
					models.ToolPart{CallID: id, Name: "lookup", State: models.ToolStateCompleted, Input: map[string]any{}, Output: "result"},
				}})
			}
			before := encodeJSON(history)
			prepared, err := model.Prepare(t.Context(), models.Request{Messages: history})
			if err != nil {
				t.Fatal(err)
			}
			calls, results := replayToolIDs(prepared.Body)
			if len(calls) != len(history) || !reflect.DeepEqual(calls, results) {
				t.Fatalf("call/result pairing = %v / %v", calls, results)
			}
			seen := map[string]bool{}
			for _, id := range calls {
				if !regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`).MatchString(id) || seen[id] {
					t.Fatalf("invalid or duplicate ID %q in %v", id, calls)
				}
				seen[id] = true
			}
			again, err := model.Prepare(t.Context(), models.Request{Messages: history})
			if err != nil || !reflect.DeepEqual(prepared.Body, again.Body) || encodeJSON(history) != before {
				t.Fatalf("unstable replay or mutated history: %v", err)
			}
		})
	}
}

func replayToolIDs(body map[string]any) (calls, results []string) {
	var visit func(any)
	visit = func(value any) {
		switch v := value.(type) {
		case []any:
			for _, child := range v {
				visit(child)
			}
		case map[string]any:
			switch v["type"] {
			case "tool_use":
				calls = append(calls, v["id"].(string))
			case "tool_result":
				results = append(results, v["tool_use_id"].(string))
			case "function_call":
				calls = append(calls, v["call_id"].(string))
			case "function_call_output":
				results = append(results, v["call_id"].(string))
			case "function":
				calls = append(calls, v["id"].(string))
			}
			if v["role"] == "tool" {
				results = append(results, v["tool_call_id"].(string))
			}
			for _, key := range []string{"functionCall", "functionResponse"} {
				if function, ok := v[key].(map[string]any); ok {
					id, _ := function["id"].(string)
					if key == "functionCall" {
						calls = append(calls, id)
					} else {
						results = append(results, id)
					}
				}
			}
			for _, key := range []string{"messages", "input", "contents", "content", "parts", "tool_calls"} {
				visit(v[key])
			}
		}
	}
	visit(body)
	return
}

func TestReplayToolImages(t *testing.T) {
	for _, protocol := range []route.Protocol{Anthropic{}, Responses{}, Chat{}, Gemini{}} {
		for _, failed := range []bool{false, true} {
			for _, vision := range []bool{false, true} {
				t.Run(protocol.ID()+"/error="+encodeJSON(failed)+"/vision="+encodeJSON(vision), func(t *testing.T) {
					model := replayModel(protocol)
					model.Info_.Capabilities.Images = vision
					state := models.ToolStateCompleted
					if failed {
						state = models.ToolStateError
					}
					history := []models.Message{{Role: models.RoleAssistant, Content: models.Content{
						models.ToolPart{CallID: "call_1", Name: "capture", State: state, Input: map[string]any{}, OutputParts: models.Content{models.TextPart{Text: "tool-output"}, models.ImagePart{Base64: "aW1hZ2U=", MediaType: "image/png"}}, Structured: map[string]any{"ok": false}},
						models.ToolPart{CallID: "call_2", Name: "other", State: models.ToolStateCompleted, Input: map[string]any{}, Output: "second-result"},
					}}}
					before := encodeJSON(history)
					prepared, err := model.Prepare(t.Context(), models.Request{Messages: history})
					if err != nil {
						t.Fatal(err)
					}
					wire := encodeJSON(prepared.Body)
					if strings.Contains(wire, "aW1hZ2U=") != vision || (!vision && !strings.Contains(wire, "image omitted")) {
						t.Fatalf("incorrect image replay: %s", wire)
					}
					if !strings.Contains(wire, "tool-output") || !strings.Contains(wire, "second-result") || !strings.Contains(wire, "ok") || encodeJSON(history) != before {
						t.Fatalf("lost tool content or mutated history: %s", wire)
					}
					if _, anthropic := protocol.(Anthropic); anthropic && failed && vision {
						content := prepared.Body["messages"].([]any)[1].(map[string]any)["content"].([]any)
						if strings.Contains(encodeJSON(content[0]), "aW1hZ2U=") || content[1].(map[string]any)["type"] != "tool_result" || !strings.Contains(encodeJSON(content[2:]), "aW1hZ2U=") {
							t.Fatalf("error images must follow all tool results: %#v", content)
						}
					}
					if vision {
						switch protocol.(type) {
						case Chat:
							messages := prepared.Body["messages"].([]any)
							if len(messages) != 4 || messages[2].(map[string]any)["role"] != "tool" || messages[3].(map[string]any)["role"] != "user" {
								t.Fatalf("images interrupted tool results: %#v", messages)
							}
						case Gemini:
							contents := prepared.Body["contents"].([]any)
							if len(contents) != 3 || len(contents[1].(map[string]any)["parts"].([]any)) != 2 || !strings.Contains(encodeJSON(contents[2]), "aW1hZ2U=") {
								t.Fatalf("images interrupted function responses: %#v", contents)
							}
						case Responses:
							input := prepared.Body["input"].([]any)
							output := input[2].(map[string]any)["output"].([]any)
							if len(output) != 3 || output[0].(map[string]any)["type"] != "input_text" || output[1].(map[string]any)["type"] != "input_image" || output[2].(map[string]any)["type"] != "input_text" {
								t.Fatalf("tool content order changed: %#v", output)
							}
						}
					}
				})
			}
		}
	}
}

func TestReplayDuplicateIDsInOneBatch(t *testing.T) {
	for _, identified := range []bool{false, true} {
		t.Run(encodeJSON(identified), func(t *testing.T) {
			var content models.Content
			for _, output := range []string{"first", "second"} {
				part := models.ToolPart{CallID: "duplicate", Name: output, State: models.ToolStateCompleted, Input: map[string]any{}, Output: output}
				if identified {
					part.ID = output
					part.ToolUseID = "use_" + output
				}
				content = append(content, part)
			}
			model := replayModel(Responses{})
			prepared, err := model.Prepare(t.Context(), models.Request{Messages: []models.Message{{Role: models.RoleAssistant, Content: content}}})
			if err != nil {
				t.Fatal(err)
			}
			calls, results := replayToolIDs(prepared.Body)
			if len(calls) != 2 || calls[0] == calls[1] || !reflect.DeepEqual(calls, results) {
				t.Fatalf("call/result pairing = %v / %v", calls, results)
			}
		})
	}
}

func TestReplayReservedIDsAndChatLimits(t *testing.T) {
	for _, id := range []string{"model", "mistral-large"} {
		t.Run(id, func(t *testing.T) {
			model := replayModel(Chat{})
			model.Info_.ID = id
			content := models.Content{}
			for _, callID := range []string{"invalid/id", "w00000001", strings.Repeat("x", 41), "abcdefghi"} {
				content = append(content, models.ToolPart{CallID: callID, Name: "lookup", State: models.ToolStateCompleted, Input: map[string]any{}, Output: "result"})
			}
			prepared, err := model.Prepare(t.Context(), models.Request{Messages: []models.Message{{Role: models.RoleAssistant, Content: content}}})
			if err != nil {
				t.Fatal(err)
			}
			calls, results := replayToolIDs(prepared.Body)
			if !reflect.DeepEqual(calls, results) || len(calls) != 4 || calls[0] == calls[1] || calls[1] != "w00000001" || calls[3] != "abcdefghi" {
				t.Fatalf("call/result pairing = %v / %v", calls, results)
			}
			for _, callID := range calls {
				if len(callID) > 40 || (id == "mistral-large" && !regexp.MustCompile(`^[a-zA-Z0-9]{9}$`).MatchString(callID)) {
					t.Fatalf("invalid ID %q", callID)
				}
			}
		})
	}
}

func TestReplayUserImages(t *testing.T) {
	for _, protocol := range []route.Protocol{Anthropic{}, Responses{}, Chat{}, Gemini{}} {
		for _, image := range []models.ImagePart{
			{Base64: "aW1hZ2U=", MediaType: "image/png"},
			{URL: "data:image/png;base64,aW1hZ2U="},
			{URL: "https://example.com/image.png", MediaType: "image/png"},
		} {
			for _, vision := range []bool{false, true} {
				t.Run(protocol.ID()+"/"+image.URL+"/vision="+encodeJSON(vision), func(t *testing.T) {
					model := replayModel(protocol)
					model.Info_.Capabilities.Images = vision
					history := []models.Message{{Role: models.RoleUser, Content: models.Content{image}}}
					before := encodeJSON(history)
					prepared, err := model.Prepare(t.Context(), models.Request{Messages: history})
					if err != nil {
						t.Fatal(err)
					}
					wire := encodeJSON(prepared.Body)
					present := strings.Contains(wire, "aW1hZ2U=") || strings.Contains(wire, "https://example.com/image.png")
					if present != vision || (!vision && !strings.Contains(wire, "image omitted")) || encodeJSON(history) != before {
						t.Fatalf("incorrect image replay or history mutation: %s", wire)
					}
				})
			}
		}
	}
}

func TestReplayDeepSeekReasoning(t *testing.T) {
	model := replayModel(Chat{})
	model.Info_.Provider = "deepseek"
	for _, same := range []bool{false, true} {
		t.Run(encodeJSON(same), func(t *testing.T) {
			origin := &models.MessageOrigin{Provider: "other", API: model.Info_.API, ModelID: model.Info_.ID}
			if same {
				origin.Provider = "deepseek"
			}
			prepared, err := model.Prepare(t.Context(), models.Request{Messages: []models.Message{{Role: models.RoleAssistant, Origin: origin, Content: models.Content{
				models.ReasoningPart{Reasoning: "reasoning"},
				models.ToolPart{CallID: "call_1", Name: "lookup", State: models.ToolStateCompleted, Input: map[string]any{}, Output: "result"},
			}}}})
			if err != nil {
				t.Fatal(err)
			}
			message := prepared.Body["messages"].([]any)[0].(map[string]any)
			want := ""
			if same {
				want = "reasoning"
			}
			if message["reasoning_content"] != want {
				t.Fatalf("reasoning = %#v, want %q", message, want)
			}
		})
	}
}

func TestReplayGeminiNativeCallID(t *testing.T) {
	model := replayModel(Gemini{})
	parser := model.Route.Protocol.NewParser(model.Info_)
	_, err := parser.Step(route.Frame{Data: `{"candidates":[{"content":{"parts":[{"functionCall":{"id":"native-id","name":"lookup","args":{}},"thoughtSignature":"native-signature"}]},"finishReason":"STOP"}]}`})
	if err != nil {
		t.Fatal(err)
	}
	message := *parser.Message()
	call := message.Content[0].(models.ToolCallPart)
	if call.CallID != "native-id" {
		t.Fatalf("call = %#v", call)
	}
	message.Content = models.Content{models.ToolPart{CallID: call.CallID, Name: call.Name, State: models.ToolStateCompleted, Input: call.Input, Output: "result", ProviderMetadata: call.ProviderMetadata}}
	for _, id := range []string{"model", "gemini-2.5-pro"} {
		t.Run(id, func(t *testing.T) {
			model.Info_.ID = id
			prepared, err := model.Prepare(t.Context(), models.Request{Messages: []models.Message{message}})
			if err != nil {
				t.Fatal(err)
			}
			wire := encodeJSON(prepared.Body)
			if id == "model" {
				calls, results := replayToolIDs(prepared.Body)
				if !reflect.DeepEqual(calls, []string{"native-id"}) || !reflect.DeepEqual(calls, results) || !strings.Contains(wire, "native-signature") {
					t.Fatalf("native replay = %s", wire)
				}
			} else if strings.Contains(wire, "native-id") || strings.Contains(wire, "thoughtSignature") {
				t.Fatalf("legacy replay includes incompatible state: %s", wire)
			}
		})
	}
}

func TestReplayDropsForeignOpaqueOnlyMessages(t *testing.T) {
	for _, protocol := range []route.Protocol{Anthropic{}, Responses{}, Chat{}, Gemini{}} {
		t.Run(protocol.ID(), func(t *testing.T) {
			model := replayModel(protocol)
			messages := []models.Message{
				{Role: models.RoleAssistant, Content: models.Content{
					models.ReasoningPart{Encrypted: "foreign", ProviderMetadata: models.Meta{"anthropic": map[string]any{"redacted_data": "opaque"}}},
					models.TextPart{ProviderMetadata: models.Meta{"google": map[string]any{"thought_signature": "foreign"}}},
				}},
				{Role: models.RoleUser, Content: models.Content{models.TextPart{Text: "continue"}}},
			}
			prepared, err := model.Prepare(t.Context(), models.Request{Messages: messages})
			if err != nil {
				t.Fatal(err)
			}
			key := "messages"
			switch protocol.(type) {
			case Responses:
				key = "input"
			case Gemini:
				key = "contents"
			}
			items := prepared.Body[key].([]any)
			if len(items) != 1 || items[0].(map[string]any)["role"] != "user" {
				t.Fatalf("empty foreign assistant replayed: %#v", items)
			}
		})
	}
}
