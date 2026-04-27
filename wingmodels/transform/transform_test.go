package transform

import (
	"reflect"
	"testing"

	"github.com/chaserensberger/wingman/wingmodels"
)

// commonTarget builds a Target identifying a stable provider/api/model used
// by most tests. Capabilities.Images defaults to true so image-related tests must
// override deliberately.
func commonTarget() Target {
	return Target{
		Provider: "anthropic",
		API:      wingmodels.APIAnthropicMessages,
		ModelID:  "claude-sonnet-4-5",
		Capabilities: wingmodels.ModelCapabilities{
			Images: true,
			Tools:  true,
		},
	}
}

// originOf builds a MessageOrigin matching the common target. Used to mark
// assistant messages as same-model so reasoning is preserved.
func originOf(t Target) *wingmodels.MessageOrigin {
	return &wingmodels.MessageOrigin{
		Provider: t.Provider,
		API:      t.API,
		ModelID:  t.ModelID,
	}
}

func TestApplyEmptyInputReturnsEmptySlice(t *testing.T) {
	out := Apply(nil, commonTarget())
	if out == nil {
		t.Fatal("Apply(nil) returned nil; want non-nil empty slice")
	}
	if len(out) != 0 {
		t.Errorf("len=%d; want 0", len(out))
	}
}

func TestApplyDoesNotMutateInput(t *testing.T) {
	msgs := []wingmodels.Message{
		wingmodels.NewUserText("hello"),
		{
			Role: wingmodels.RoleAssistant,
			Content: wingmodels.Content{
				wingmodels.ReasoningPart{Reasoning: "thinking"},
				wingmodels.TextPart{Text: "hi"},
			},
			// Cross-model: reasoning should be dropped in the output.
			Origin: &wingmodels.MessageOrigin{
				Provider: "openai",
				API:      wingmodels.APIOpenAIResponses,
				ModelID:  "gpt-5",
			},
		},
	}
	originalLen := len(msgs[1].Content)
	_ = Apply(msgs, commonTarget())
	if len(msgs[1].Content) != originalLen {
		t.Errorf("input message Content was mutated: len changed from %d to %d", originalLen, len(msgs[1].Content))
	}
	if _, ok := msgs[1].Content[0].(wingmodels.ReasoningPart); !ok {
		t.Error("input ReasoningPart was mutated away")
	}
}

func TestDropFailedAssistantTurns(t *testing.T) {
	msgs := []wingmodels.Message{
		wingmodels.NewUserText("first"),
		{
			Role:         wingmodels.RoleAssistant,
			Content:      wingmodels.Content{wingmodels.TextPart{Text: "good answer"}},
			FinishReason: wingmodels.FinishReasonStop,
		},
		wingmodels.NewUserText("second"),
		{
			Role: wingmodels.RoleAssistant,
			Content: wingmodels.Content{
				wingmodels.TextPart{Text: "broken"},
				wingmodels.ToolCallPart{CallID: "tlu_failed", Name: "x", Input: nil},
			},
			FinishReason: wingmodels.FinishReasonError,
		},
		// Stale tool result attached to the failed assistant — must also drop.
		wingmodels.NewToolResult("tlu_failed", []wingmodels.Part{wingmodels.TextPart{Text: "?"}}, false),
		wingmodels.NewUserText("third"),
	}
	out := Apply(msgs, commonTarget())

	wantRoles := []wingmodels.Role{
		wingmodels.RoleUser,
		wingmodels.RoleAssistant,
		wingmodels.RoleUser,
		wingmodels.RoleUser,
	}
	if len(out) != len(wantRoles) {
		t.Fatalf("len=%d; want %d. got=%+v", len(out), len(wantRoles), out)
	}
	for i, r := range wantRoles {
		if out[i].Role != r {
			t.Errorf("out[%d].Role=%s; want %s", i, out[i].Role, r)
		}
	}
	// Sanity: surviving assistant is the "good answer" turn.
	if txt := out[1].Content[0].(wingmodels.TextPart).Text; txt != "good answer" {
		t.Errorf("survivor text=%q; want 'good answer'", txt)
	}
}

func TestDropAbortedAssistantTurns(t *testing.T) {
	msgs := []wingmodels.Message{
		wingmodels.NewUserText("hi"),
		{
			Role:         wingmodels.RoleAssistant,
			Content:      wingmodels.Content{wingmodels.TextPart{Text: "partial..."}},
			FinishReason: wingmodels.FinishReasonAborted,
		},
	}
	out := Apply(msgs, commonTarget())
	if len(out) != 1 {
		t.Fatalf("len=%d; want 1", len(out))
	}
	if out[0].Role != wingmodels.RoleUser {
		t.Errorf("survivor role=%s; want user", out[0].Role)
	}
}

func TestCrossModelReasoningDropped(t *testing.T) {
	target := commonTarget()
	msgs := []wingmodels.Message{
		wingmodels.NewUserText("q"),
		{
			Role: wingmodels.RoleAssistant,
			Content: wingmodels.Content{
				wingmodels.ReasoningPart{Reasoning: "secret thoughts", Signature: "sig123"},
				wingmodels.TextPart{Text: "answer"},
			},
			Origin: &wingmodels.MessageOrigin{
				Provider: "openai",
				API:      wingmodels.APIOpenAIResponses,
				ModelID:  "gpt-5",
			},
			FinishReason: wingmodels.FinishReasonStop,
		},
	}
	out := Apply(msgs, target)
	if len(out) != 2 {
		t.Fatalf("len=%d; want 2", len(out))
	}
	for _, p := range out[1].Content {
		if _, ok := p.(wingmodels.ReasoningPart); ok {
			t.Error("cross-model ReasoningPart should have been dropped")
		}
	}
	// Text should remain.
	if len(out[1].Content) != 1 {
		t.Fatalf("assistant content len=%d; want 1", len(out[1].Content))
	}
	if tp, ok := out[1].Content[0].(wingmodels.TextPart); !ok || tp.Text != "answer" {
		t.Errorf("expected TextPart{answer}, got %#v", out[1].Content[0])
	}
}

func TestSameModelReasoningPreserved(t *testing.T) {
	target := commonTarget()
	msgs := []wingmodels.Message{
		wingmodels.NewUserText("q"),
		{
			Role: wingmodels.RoleAssistant,
			Content: wingmodels.Content{
				wingmodels.ReasoningPart{Reasoning: "still here", Signature: "sig"},
				wingmodels.TextPart{Text: "answer"},
			},
			Origin:       originOf(target),
			FinishReason: wingmodels.FinishReasonStop,
		},
	}
	out := Apply(msgs, target)
	if len(out[1].Content) != 2 {
		t.Fatalf("same-model content len=%d; want 2 (reasoning+text)", len(out[1].Content))
	}
	if _, ok := out[1].Content[0].(wingmodels.ReasoningPart); !ok {
		t.Error("same-model ReasoningPart should be preserved")
	}
}

func TestImageDowngradeOnNonVisionTarget(t *testing.T) {
	target := commonTarget()
	target.Capabilities.Images = false
	msgs := []wingmodels.Message{
		{
			Role: wingmodels.RoleUser,
			Content: wingmodels.Content{
				wingmodels.TextPart{Text: "look at these:"},
				wingmodels.ImagePart{Data: "a", MimeType: "image/png"},
				wingmodels.ImagePart{Data: "b", MimeType: "image/png"},
				wingmodels.ImagePart{Data: "c", MimeType: "image/png"},
				wingmodels.TextPart{Text: "what do you see?"},
			},
		},
	}
	out := Apply(msgs, target)
	want := wingmodels.Content{
		wingmodels.TextPart{Text: "look at these:"},
		wingmodels.TextPart{Text: placeholderUserImage},
		wingmodels.TextPart{Text: "what do you see?"},
	}
	if !reflect.DeepEqual(out[0].Content, want) {
		t.Errorf("downgrade mismatch:\n got=%#v\nwant=%#v", out[0].Content, want)
	}
}

func TestImageDowngradeOnToolResult(t *testing.T) {
	target := commonTarget()
	target.Capabilities.Images = false
	msgs := []wingmodels.Message{
		wingmodels.NewUserText("show"),
		{
			Role: wingmodels.RoleAssistant,
			Content: wingmodels.Content{
				wingmodels.ToolCallPart{CallID: "tlu_1", Name: "screenshot", Input: map[string]any{}},
			},
			FinishReason: wingmodels.FinishReasonToolCalls,
		},
		{
			Role: wingmodels.RoleTool,
			Content: wingmodels.Content{
				wingmodels.ToolResultPart{
					CallID: "tlu_1",
					Output: []wingmodels.Part{
						wingmodels.ImagePart{Data: "x", MimeType: "image/png"},
					},
				},
			},
		},
	}
	out := Apply(msgs, target)
	tr := out[2].Content[0].(wingmodels.ToolResultPart)
	if len(tr.Output) != 1 {
		t.Fatalf("output len=%d; want 1", len(tr.Output))
	}
	if tp, ok := tr.Output[0].(wingmodels.TextPart); !ok || tp.Text != placeholderToolImage {
		t.Errorf("tool image not downgraded: got %#v", tr.Output[0])
	}
}

func TestImagePreservedWhenSupported(t *testing.T) {
	target := commonTarget() // Capabilities.Images = true
	msgs := []wingmodels.Message{
		{
			Role: wingmodels.RoleUser,
			Content: wingmodels.Content{
				wingmodels.ImagePart{Data: "a", MimeType: "image/png"},
			},
		},
	}
	out := Apply(msgs, target)
	if _, ok := out[0].Content[0].(wingmodels.ImagePart); !ok {
		t.Errorf("ImagePart should be preserved on vision-capable target; got %#v", out[0].Content[0])
	}
}

func TestOrphanToolCallSynthesizesErrorResult(t *testing.T) {
	msgs := []wingmodels.Message{
		wingmodels.NewUserText("do work"),
		{
			Role: wingmodels.RoleAssistant,
			Content: wingmodels.Content{
				wingmodels.ToolCallPart{CallID: "tlu_orphan", Name: "search", Input: map[string]any{"q": "x"}},
			},
			FinishReason: wingmodels.FinishReasonToolCalls,
		},
		// No tool result; conversation ends.
	}
	out := Apply(msgs, commonTarget())
	if len(out) != 3 {
		t.Fatalf("len=%d; want 3 (user, assistant, synthetic tool); got=%+v", len(out), out)
	}
	if out[2].Role != wingmodels.RoleTool {
		t.Errorf("synthetic message role=%s; want tool", out[2].Role)
	}
	tr, ok := out[2].Content[0].(wingmodels.ToolResultPart)
	if !ok {
		t.Fatalf("synthetic content[0]=%#v; want ToolResultPart", out[2].Content[0])
	}
	if tr.CallID != "tlu_orphan" {
		t.Errorf("synthetic CallID=%q; want tlu_orphan", tr.CallID)
	}
	if !tr.IsError {
		t.Error("synthetic result should be marked IsError")
	}
}

func TestOrphanReconciliationSkipsResolvedCalls(t *testing.T) {
	msgs := []wingmodels.Message{
		wingmodels.NewUserText("q"),
		{
			Role: wingmodels.RoleAssistant,
			Content: wingmodels.Content{
				wingmodels.ToolCallPart{CallID: "tlu_a", Name: "f", Input: nil},
				wingmodels.ToolCallPart{CallID: "tlu_b", Name: "f", Input: nil},
			},
			FinishReason: wingmodels.FinishReasonToolCalls,
		},
		wingmodels.NewToolResult("tlu_a", []wingmodels.Part{wingmodels.TextPart{Text: "ok"}}, false),
		// tlu_b has no result; should get a synthetic one before user turn.
		wingmodels.NewUserText("next"),
	}
	out := Apply(msgs, commonTarget())
	// Expect: user, assistant, tool(tlu_a real), tool(tlu_b synth), user
	if len(out) != 5 {
		t.Fatalf("len=%d; want 5; got=%+v", len(out), out)
	}
	// Find the synthetic.
	var synth *wingmodels.ToolResultPart
	for i := range out {
		if out[i].Role != wingmodels.RoleTool {
			continue
		}
		tr := out[i].Content[0].(wingmodels.ToolResultPart)
		if tr.CallID == "tlu_b" {
			t2 := tr
			synth = &t2
		}
	}
	if synth == nil {
		t.Fatal("expected synthetic result for tlu_b")
	}
	if !synth.IsError {
		t.Error("synthetic result should be IsError")
	}
}

func TestEmptyContentMessagesElided(t *testing.T) {
	target := commonTarget()
	target.Capabilities.Images = false
	msgs := []wingmodels.Message{
		wingmodels.NewUserText("hello"),
		{
			Role: wingmodels.RoleAssistant,
			// Cross-model + only-reasoning means content drops to empty.
			Content: wingmodels.Content{
				wingmodels.ReasoningPart{Reasoning: "thinking"},
			},
			Origin: &wingmodels.MessageOrigin{
				Provider: "openai",
				API:      wingmodels.APIOpenAIResponses,
				ModelID:  "gpt-5",
			},
			FinishReason: wingmodels.FinishReasonStop,
		},
	}
	out := Apply(msgs, target)
	if len(out) != 1 {
		t.Fatalf("len=%d; want 1 (assistant elided); got=%+v", len(out), out)
	}
	if out[0].Role != wingmodels.RoleUser {
		t.Errorf("survivor role=%s; want user", out[0].Role)
	}
}

func TestNormalizeToolCallIDAppliedCrossModel(t *testing.T) {
	target := commonTarget()
	target.NormalizeToolCallID = func(id string) string {
		return "norm_" + id
	}
	msgs := []wingmodels.Message{
		wingmodels.NewUserText("q"),
		{
			Role: wingmodels.RoleAssistant,
			Content: wingmodels.Content{
				wingmodels.ToolCallPart{CallID: "raw_xyz", Name: "f", Input: nil},
			},
			Origin: &wingmodels.MessageOrigin{
				Provider: "openai",
				API:      wingmodels.APIOpenAIResponses,
				ModelID:  "gpt-5",
			},
			FinishReason: wingmodels.FinishReasonToolCalls,
		},
		wingmodels.NewToolResult("raw_xyz", []wingmodels.Part{wingmodels.TextPart{Text: "ok"}}, false),
	}
	out := Apply(msgs, target)
	tc := out[1].Content[0].(wingmodels.ToolCallPart)
	tr := out[2].Content[0].(wingmodels.ToolResultPart)
	if tc.CallID != "norm_raw_xyz" {
		t.Errorf("tool call CallID=%q; want norm_raw_xyz", tc.CallID)
	}
	if tr.CallID != "norm_raw_xyz" {
		t.Errorf("tool result CallID=%q; want norm_raw_xyz", tr.CallID)
	}
}

func TestNormalizeToolCallIDSkippedSameModel(t *testing.T) {
	target := commonTarget()
	called := false
	target.NormalizeToolCallID = func(id string) string {
		called = true
		return "should_not_be_used"
	}
	msgs := []wingmodels.Message{
		{
			Role: wingmodels.RoleAssistant,
			Content: wingmodels.Content{
				wingmodels.ToolCallPart{CallID: "tlu_native", Name: "f", Input: nil},
			},
			Origin:       originOf(target),
			FinishReason: wingmodels.FinishReasonToolCalls,
		},
		wingmodels.NewToolResult("tlu_native", []wingmodels.Part{wingmodels.TextPart{Text: "ok"}}, false),
	}
	out := Apply(msgs, target)
	if called {
		t.Error("NormalizeToolCallID should not be invoked for same-model assistant messages")
	}
	tc := out[0].Content[0].(wingmodels.ToolCallPart)
	if tc.CallID != "tlu_native" {
		t.Errorf("CallID changed unexpectedly: got %q", tc.CallID)
	}
}

func TestMessageWithNoOriginIsTreatedAsCrossModel(t *testing.T) {
	// Older sessions / synthetic messages won't have Origin. SameModel
	// returns false on nil, so they should be treated as cross-model
	// (lossy transforms apply).
	target := commonTarget()
	msgs := []wingmodels.Message{
		{
			Role: wingmodels.RoleAssistant,
			Content: wingmodels.Content{
				wingmodels.ReasoningPart{Reasoning: "?"},
				wingmodels.TextPart{Text: "answer"},
			},
			// Origin: nil
			FinishReason: wingmodels.FinishReasonStop,
		},
	}
	out := Apply(msgs, target)
	for _, p := range out[0].Content {
		if _, ok := p.(wingmodels.ReasoningPart); ok {
			t.Error("ReasoningPart should be dropped when Origin is nil (cross-model fallback)")
		}
	}
}
