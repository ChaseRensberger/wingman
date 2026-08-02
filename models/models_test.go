package models

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestEventStreamPushStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stream := NewEventStream[int, struct{}](1)
	stream.BindContext(ctx)
	stream.Push(1)
	done := make(chan struct{})
	go func() { stream.Push(2); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Push remained blocked after cancellation")
	}
}

func TestProviderErrorContract(t *testing.T) {
	cause := errors.New("wire failure")
	retryAfter := time.Second
	err := &ProviderError{Category: ErrorRateLimit, Provider: "openai", Status: 429, RequestID: "req_1", Retryable: true, RetryAfter: &retryAfter, Message: "provider request failed", Metadata: map[string]string{"region": "us"}, Cause: cause}
	if !errors.Is(err, cause) {
		t.Fatal("ProviderError does not unwrap its cause")
	}
	var got *ProviderError
	if !errors.As(err, &got) || got.ProviderRequestID() != "req_1" || got.Category != ErrorRateLimit || got.RetryAfter == nil {
		t.Fatalf("ProviderError = %#v", got)
	}
}

func TestPartDecoderGenerationIsScoped(t *testing.T) {
	registry := NewPartRegistry()
	if err := registry.Register("custom", opaqueDecoder("custom")); err != nil {
		t.Fatal(err)
	}
	decoders, err := registry.Build()
	if err != nil {
		t.Fatal(err)
	}

	part, err := decoders.UnmarshalPart([]byte(`{"type":"custom","value":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := part.(OpaquePart); !ok {
		t.Fatalf("scoped part = %T", part)
	}
	part, err = UnmarshalPart([]byte(`{"type":"custom","value":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := part.(OpaquePart); !ok {
		t.Fatalf("base part = %T", part)
	}
	if err := registry.Register("later", opaqueDecoder("later")); err == nil {
		t.Fatal("Register after Build succeeded")
	}
}

func TestPartRegistryRejectsInvalidRegistrations(t *testing.T) {
	registry := NewPartRegistry()
	for _, typeName := range []string{"", " ", "text"} {
		if err := registry.Register(typeName, opaqueDecoder(typeName)); err == nil {
			t.Fatalf("Register(%q) succeeded", typeName)
		}
	}
	if err := registry.Register("custom", opaqueDecoder("custom")); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("custom", opaqueDecoder("custom")); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("duplicate error = %v", err)
	}
}

func opaqueDecoder(typeName string) PartUnmarshaler {
	return func(data []byte) (Part, error) {
		return OpaquePart{TypeName: typeName, Raw: append([]byte(nil), data...)}, nil
	}
}

func TestMessageJSONIdentityAndState(t *testing.T) {
	message := Message{ID: "msg_1", Revision: 3, State: MessageStateCompleted, Role: RoleUser, Content: Content{TextPart{ID: "part_1", Text: "hello"}}}
	b, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	var got Message
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != message.ID || got.Revision != message.Revision || got.State != message.State {
		t.Fatalf("message identity = %#v", got)
	}
}

func TestPartIDHelpersForBuiltins(t *testing.T) {
	parts := []Part{
		TextPart{}, ImagePart{}, ReasoningPart{}, ToolPart{}, ToolCallPart{}, ToolResultPart{}, OpaquePart{TypeName: "plugin"},
	}
	for _, part := range parts {
		got := WithPartID(part, "part_1")
		if PartID(got) != "part_1" {
			t.Fatalf("PartID(%T) = %q", got, PartID(got))
		}
	}
}

func TestNormalizeMessagesFoldsLegacyToolResults(t *testing.T) {
	messages := []Message{
		{Role: RoleAssistant, Content: Content{ToolCallPart{CallID: "call_1", Name: "bash", Input: map[string]any{"command": "pwd"}}}},
		{Role: RoleTool, Content: Content{ToolResultPart{CallID: "call_1", Name: "bash", Output: Content{TextPart{Text: "/tmp"}}}}},
	}

	normalized := NormalizeMessages(messages)
	if len(normalized) != 1 || normalized[0].Role != RoleAssistant {
		t.Fatalf("normalized messages = %#v", normalized)
	}
	tool, ok := normalized[0].Content[0].(ToolPart)
	if !ok || tool.State != ToolStateCompleted || tool.Output != "/tmp" {
		t.Fatalf("tool = %#v", normalized[0].Content[0])
	}
}

func TestExpandToolMessagesDerivesProviderResult(t *testing.T) {
	messages := []Message{{Role: RoleAssistant, Content: Content{ToolPart{CallID: "call_1", Name: "bash", State: ToolStateCompleted, Input: map[string]any{"command": "pwd"}, Output: "/tmp"}}}}

	expanded := ExpandToolMessages(messages)
	if len(expanded) != 2 || expanded[1].Role != RoleTool {
		t.Fatalf("expanded messages = %#v", expanded)
	}
	result, ok := expanded[1].Content[0].(ToolResultPart)
	if !ok || toolResultText(result) != "/tmp" {
		t.Fatalf("result = %#v", expanded[1].Content[0])
	}
}

func TestExpandToolMessagesPreservesOutputPartsAndStructuredResult(t *testing.T) {
	messages := []Message{{Role: RoleAssistant, Content: Content{ToolPart{
		CallID:      "call_1",
		Name:        "screenshot",
		State:       ToolStateCompleted,
		Input:       map[string]any{},
		Output:      "captured",
		OutputParts: Content{TextPart{Text: "captured"}, ImagePart{Base64: "image-data", MediaType: "image/png"}},
		Structured:  map[string]any{"width": float64(10)},
	}}}}

	expanded := ExpandToolMessages(messages)
	result := expanded[1].Content[0].(ToolResultPart)
	if len(result.Output) != 2 {
		t.Fatalf("output = %#v, want text and image", result.Output)
	}
	if _, ok := result.Output[1].(ImagePart); !ok {
		t.Fatalf("output[1] = %T, want ImagePart", result.Output[1])
	}
	if result.Structured.(map[string]any)["width"] != float64(10) {
		t.Fatalf("structured = %#v", result.Structured)
	}
}

func TestExpandToolMessagesCombinesOutputAndError(t *testing.T) {
	messages := []Message{{Role: RoleAssistant, Content: Content{ToolPart{
		CallID: "call_1",
		Name:   "bash",
		State:  ToolStateError,
		Input:  map[string]any{"command": "pwd"},
		Output: "partial",
		Error:  "failed",
	}}}}

	expanded := ExpandToolMessages(messages)
	if len(expanded) != 2 || expanded[1].Role != RoleTool {
		t.Fatalf("expanded messages = %#v", expanded)
	}
	result, ok := expanded[1].Content[0].(ToolResultPart)
	if !ok || !result.IsError {
		t.Fatalf("result = %#v, want error", result)
	}
	var text string
	for _, p := range result.Output {
		if tp, ok := p.(TextPart); ok {
			text = tp.Text
		}
	}
	if text != "partial\nfailed" {
		t.Fatalf("result text = %q, want combined output+error", text)
	}
}

func TestExpandToolMessagesOmitsUnresolvedTool(t *testing.T) {
	messages := []Message{{Role: RoleAssistant, Content: Content{ToolPart{
		CallID: "call_1",
		Name:   "bash",
		State:  ToolStatePending,
		Input:  map[string]any{"command": "pwd"},
	}}}}

	if expanded := ExpandToolMessages(messages); len(expanded) != 0 {
		t.Fatalf("expanded messages = %#v, want unresolved tool omitted", expanded)
	}
}

func TestToolPartProviderFieldsPreservedThroughNormalizationAndExpansion(t *testing.T) {
	messages := []Message{{Role: RoleAssistant, Content: Content{ToolCallPart{
		ID:               "part_1",
		CallID:           "call_1",
		Name:             "bash",
		Input:            map[string]any{"command": "pwd"},
		ProviderExecuted: true,
		ProviderMetadata: Meta{"provider_call": "abc"},
	}}}}

	normalized := NormalizeMessages(messages)
	tool := normalized[0].Content[0].(ToolPart)
	if tool.ID != "part_1" || !tool.ProviderExecuted || tool.ProviderMetadata["provider_call"] != "abc" {
		t.Fatalf("normalized tool = %#v", tool)
	}
	tool.InputRaw = `{"command":"pwd"}`
	tool.State = ToolStateCompleted
	normalized[0].Content[0] = tool

	expanded := ExpandToolMessages(normalized)
	call := expanded[0].Content[0].(ToolCallPart)
	result := expanded[1].Content[0].(ToolResultPart)
	if call.ID != tool.ID || !call.ProviderExecuted || call.ProviderMetadata["provider_call"] != "abc" {
		t.Fatalf("expanded call = %#v", call)
	}
	if result.ID != tool.ID || !result.ProviderExecuted || result.ProviderMetadata["provider_call"] != "abc" {
		t.Fatalf("expanded result = %#v", result)
	}
}

func TestToolUseIDPreservedThroughJSONNormalizationAndExpansion(t *testing.T) {
	part := ToolResultPart{ID: "part_1", ToolUseID: "tlu_1", CallID: "call_1", Name: "bash", Output: Content{TextPart{Text: "ok"}}, Structured: map[string]any{"count": float64(1)}}
	b, err := MarshalPart(part)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := UnmarshalPart(b)
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded.(ToolResultPart).ToolUseID; got != "tlu_1" {
		t.Fatalf("JSON ToolUseID = %q", got)
	}
	if got := decoded.(ToolResultPart).Structured.(map[string]any)["count"]; got != float64(1) {
		t.Fatalf("JSON structured result = %#v", decoded)
	}
	messages := NormalizeMessages([]Message{
		{Role: RoleAssistant, Content: Content{ToolCallPart{ID: "part_1", ToolUseID: "tlu_1", CallID: "call_1", Name: "bash", Input: map[string]any{}}}},
		{Role: RoleTool, Content: Content{part}},
	})
	tool := messages[0].Content[0].(ToolPart)
	if tool.ToolUseID != "tlu_1" {
		t.Fatalf("normalized ToolUseID = %q", tool.ToolUseID)
	}
	expanded := ExpandToolMessages(messages)
	if got := expanded[0].Content[0].(ToolCallPart).ToolUseID; got != "tlu_1" {
		t.Fatalf("expanded call ToolUseID = %q", got)
	}
	if got := expanded[1].Content[0].(ToolResultPart).ToolUseID; got != "tlu_1" {
		t.Fatalf("expanded result ToolUseID = %q", got)
	}
}

func TestNormalizeMessagesPreservesToolRawAndProviderFieldsOnResultReplacement(t *testing.T) {
	messages := []Message{
		{Role: RoleAssistant, Content: Content{ToolPart{CallID: "call_1", Name: "bash", State: ToolStatePending, Input: map[string]any{"command": "pwd"}, InputRaw: `{"command":"pwd"}`, ProviderExecuted: true, ProviderMetadata: Meta{"call": "metadata"}}}},
		{Role: RoleTool, Content: Content{ToolResultPart{CallID: "call_1", Name: "bash", Output: Content{TextPart{Text: "/tmp"}}}}},
	}

	normalized := NormalizeMessages(messages)
	tool := normalized[0].Content[0].(ToolPart)
	if tool.InputRaw != `{"command":"pwd"}` || !tool.ProviderExecuted || tool.ProviderMetadata["call"] != "metadata" {
		t.Fatalf("replaced tool = %#v", tool)
	}
}

func TestUnknownPartNestedJSONRoundTrip(t *testing.T) {
	input := []byte(`{"type":"future","id":"part_1","nested":{"items":[1,{"key":"value"}]},"extra":true}`)
	part, err := UnmarshalPart(input)
	if err != nil {
		t.Fatal(err)
	}
	opaque, ok := part.(OpaquePart)
	if !ok || opaque.ID != "part_1" || opaque.TypeName != "future" {
		t.Fatalf("part = %#v", part)
	}
	got, err := MarshalPart(part)
	if err != nil {
		t.Fatal(err)
	}
	var wantValue, gotValue any
	if err := json.Unmarshal(input, &wantValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatal(err)
	}
	if !jsonEqual(wantValue, gotValue) {
		t.Fatalf("round trip = %s, want %s", got, input)
	}
}

func TestWithPartIDOpaquePreservesUnknownFields(t *testing.T) {
	part, err := UnmarshalPart([]byte(`{"type":"future","nested":{"key":"value"},"extra":true}`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := MarshalPart(WithPartID(part, "part_2"))
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(got, &value); err != nil {
		t.Fatal(err)
	}
	if value["id"] != "part_2" || value["type"] != "future" || value["extra"] != true {
		t.Fatalf("opaque JSON = %s", got)
	}
	nested := value["nested"].(map[string]any)
	if nested["key"] != "value" {
		t.Fatalf("opaque JSON = %s", got)
	}
}

func jsonEqual(a, b any) bool {
	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)
	return string(aJSON) == string(bJSON)
}

func TestNewCallTraceRedactsAndShapes(t *testing.T) {
	req := Request{
		Model:  ModelRef{Provider: "openai", ID: "gpt-4", API: APIOpenAIResponses},
		System: "Current date: 2024-01-01.\nBe helpful.",
		Messages: []Message{
			{Role: RoleUser, Content: Content{TextPart{Text: "hello"}}},
			{Role: RoleAssistant, Content: Content{TextPart{Text: "hi"}, ToolCallPart{CallID: "c1", Name: "test", Input: map[string]any{}}}},
		},
		Tools: []ToolDef{
			{Name: "test", Description: "d", InputSchema: map[string]any{"type": "object"}},
		},
		Capabilities: Capabilities{Thinking: true},
	}
	lowered := LoweredOptions{ReasoningSummaryAuto: true}
	trace := NewCallTrace(req, lowered)

	if trace.Version != "1" {
		t.Fatalf("version = %q, want 1", trace.Version)
	}
	if trace.Provider != "openai" {
		t.Fatalf("provider = %q", trace.Provider)
	}
	if trace.API != APIOpenAIResponses {
		t.Fatalf("api = %q", trace.API)
	}
	if !trace.Runtime.CurrentDate {
		t.Fatal("expected current_date true")
	}
	if len(trace.Tools) != 1 || trace.Tools[0].Name != "test" {
		t.Fatal("tool trace mismatch")
	}
	if trace.Tools[0].SchemaHash == "" || trace.Tools[0].SchemaBytes == 0 {
		t.Fatal("expected schema hash and bytes")
	}
	if trace.Messages.Count != 2 {
		t.Fatalf("message count = %d", trace.Messages.Count)
	}
	if trace.Messages.ByRole["user"] != 1 || trace.Messages.ByRole["assistant"] != 1 {
		t.Fatalf("by_role = %v", trace.Messages.ByRole)
	}
	if trace.Messages.PartKinds["text"] != 2 || trace.Messages.PartKinds["tool_call"] != 1 {
		t.Fatalf("part_kinds = %v", trace.Messages.PartKinds)
	}
	if trace.System.Bytes == 0 || trace.System.SHA256 == "" {
		t.Fatal("expected system trace")
	}
	if !trace.Lowered.ReasoningSummaryAuto {
		t.Fatal("expected reasoning_summary_auto")
	}
	// Verify no message content or credentials leaked into trace fields
	if trace.Messages.Count == 0 {
		t.Fatal("structural info missing")
	}
}
