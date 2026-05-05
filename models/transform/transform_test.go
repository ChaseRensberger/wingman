package transform

import (
	"reflect"
	"testing"

	"github.com/chaserensberger/wingman/models"
)

// commonTarget builds a Target identifying a stable provider/api/model used
// by most tests. Capabilities.Images defaults to true so image-related tests must
// override deliberately.
func commonTarget() Target {
	return Target{
		Provider: "anthropic",
		API:      models.APIAnthropicMessages,
		ModelID:  "claude-sonnet-4-5",
		Capabilities: models.ModelCapabilities{
			Images: true,
			Tools:  true,
		},
	}
}

// originOf builds a MessageOrigin matching the common target. Used to mark
// assistant messages as same-model so reasoning is preserved.
func originOf(t Target) *models.MessageOrigin {
	return &models.MessageOrigin{
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
	msgs := []models.Message{
		models.NewUserText("hello"),
		{
			Role: models.RoleAssistant,
			Content: models.Content{
				models.ReasoningPart{Reasoning: "thinking"},
				models.TextPart{Text: "hi"},
			},
			// Cross-model: reasoning should be dropped in the output.
			Origin: &models.MessageOrigin{
				Provider: "openai",
				API:      models.APIOpenAIResponses,
				ModelID:  "gpt-5",
			},
		},
	}
	originalLen := len(msgs[1].Content)
	_ = Apply(msgs, commonTarget())
	if len(msgs[1].Content) != originalLen {
		t.Errorf("input message Content was mutated: len changed from %d to %d", originalLen, len(msgs[1].Content))
	}
	if _, ok := msgs[1].Content[0].(models.ReasoningPart); !ok {
		t.Error("input ReasoningPart was mutated away")
	}
}

func TestDropFailedAssistantTurns(t *testing.T) {
	msgs := []models.Message{
		models.NewUserText("first"),
		{
			Role:         models.RoleAssistant,
			Content:      models.Content{models.TextPart{Text: "good answer"}},
			FinishReason: models.FinishReasonStop,
		},
		models.NewUserText("second"),
		{
			Role: models.RoleAssistant,
			Content: models.Content{
				models.TextPart{Text: "broken"},
				models.ToolCallPart{CallID: "tlu_failed", Name: "x", Input: nil},
			},
			FinishReason: models.FinishReasonError,
		},
		// Stale tool result attached to the failed assistant — must also drop.
		models.NewToolResult("tlu_failed", []models.Part{models.TextPart{Text: "?"}}, false),
		models.NewUserText("third"),
	}
	out := Apply(msgs, commonTarget())

	wantRoles := []models.Role{
		models.RoleUser,
		models.RoleAssistant,
		models.RoleUser,
		models.RoleUser,
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
	if txt := out[1].Content[0].(models.TextPart).Text; txt != "good answer" {
		t.Errorf("survivor text=%q; want 'good answer'", txt)
	}
}

func TestDropAbortedAssistantTurns(t *testing.T) {
	msgs := []models.Message{
		models.NewUserText("hi"),
		{
			Role:         models.RoleAssistant,
			Content:      models.Content{models.TextPart{Text: "partial..."}},
			FinishReason: models.FinishReasonAborted,
		},
	}
	out := Apply(msgs, commonTarget())
	if len(out) != 1 {
		t.Fatalf("len=%d; want 1", len(out))
	}
	if out[0].Role != models.RoleUser {
		t.Errorf("survivor role=%s; want user", out[0].Role)
	}
}

func TestCrossModelReasoningDropped(t *testing.T) {
	target := commonTarget()
	msgs := []models.Message{
		models.NewUserText("q"),
		{
			Role: models.RoleAssistant,
			Content: models.Content{
				models.ReasoningPart{Reasoning: "secret thoughts", Signature: "sig123"},
				models.TextPart{Text: "answer"},
			},
			Origin: &models.MessageOrigin{
				Provider: "openai",
				API:      models.APIOpenAIResponses,
				ModelID:  "gpt-5",
			},
			FinishReason: models.FinishReasonStop,
		},
	}
	out := Apply(msgs, target)
	if len(out) != 2 {
		t.Fatalf("len=%d; want 2", len(out))
	}
	for _, p := range out[1].Content {
		if _, ok := p.(models.ReasoningPart); ok {
			t.Error("cross-model ReasoningPart should have been dropped")
		}
	}
	// Text should remain.
	if len(out[1].Content) != 1 {
		t.Fatalf("assistant content len=%d; want 1", len(out[1].Content))
	}
	if tp, ok := out[1].Content[0].(models.TextPart); !ok || tp.Text != "answer" {
		t.Errorf("expected TextPart{answer}, got %#v", out[1].Content[0])
	}
}

func TestSameModelReasoningPreserved(t *testing.T) {
	target := commonTarget()
	msgs := []models.Message{
		models.NewUserText("q"),
		{
			Role: models.RoleAssistant,
			Content: models.Content{
				models.ReasoningPart{Reasoning: "still here", Signature: "sig"},
				models.TextPart{Text: "answer"},
			},
			Origin:       originOf(target),
			FinishReason: models.FinishReasonStop,
		},
	}
	out := Apply(msgs, target)
	if len(out[1].Content) != 2 {
		t.Fatalf("same-model content len=%d; want 2 (reasoning+text)", len(out[1].Content))
	}
	if _, ok := out[1].Content[0].(models.ReasoningPart); !ok {
		t.Error("same-model ReasoningPart should be preserved")
	}
}

func TestImageDowngradeOnNonVisionTarget(t *testing.T) {
	target := commonTarget()
	target.Capabilities.Images = false
	msgs := []models.Message{
		{
			Role: models.RoleUser,
			Content: models.Content{
				models.TextPart{Text: "look at these:"},
				models.ImagePart{Data: "a", MimeType: "image/png"},
				models.ImagePart{Data: "b", MimeType: "image/png"},
				models.ImagePart{Data: "c", MimeType: "image/png"},
				models.TextPart{Text: "what do you see?"},
			},
		},
	}
	out := Apply(msgs, target)
	want := models.Content{
		models.TextPart{Text: "look at these:"},
		models.TextPart{Text: placeholderUserImage},
		models.TextPart{Text: "what do you see?"},
	}
	if !reflect.DeepEqual(out[0].Content, want) {
		t.Errorf("downgrade mismatch:\n got=%#v\nwant=%#v", out[0].Content, want)
	}
}

func TestImageDowngradeOnToolResult(t *testing.T) {
	target := commonTarget()
	target.Capabilities.Images = false
	msgs := []models.Message{
		models.NewUserText("show"),
		{
			Role: models.RoleAssistant,
			Content: models.Content{
				models.ToolCallPart{CallID: "tlu_1", Name: "screenshot", Input: map[string]any{}},
			},
			FinishReason: models.FinishReasonToolCalls,
		},
		{
			Role: models.RoleTool,
			Content: models.Content{
				models.ToolResultPart{
					CallID: "tlu_1",
					Output: []models.Part{
						models.ImagePart{Data: "x", MimeType: "image/png"},
					},
				},
			},
		},
	}
	out := Apply(msgs, target)
	tr := out[2].Content[0].(models.ToolResultPart)
	if len(tr.Output) != 1 {
		t.Fatalf("output len=%d; want 1", len(tr.Output))
	}
	if tp, ok := tr.Output[0].(models.TextPart); !ok || tp.Text != placeholderToolImage {
		t.Errorf("tool image not downgraded: got %#v", tr.Output[0])
	}
}

func TestImagePreservedWhenSupported(t *testing.T) {
	target := commonTarget() // Capabilities.Images = true
	msgs := []models.Message{
		{
			Role: models.RoleUser,
			Content: models.Content{
				models.ImagePart{Data: "a", MimeType: "image/png"},
			},
		},
	}
	out := Apply(msgs, target)
	if _, ok := out[0].Content[0].(models.ImagePart); !ok {
		t.Errorf("ImagePart should be preserved on vision-capable target; got %#v", out[0].Content[0])
	}
}

func TestOrphanToolCallSynthesizesErrorResult(t *testing.T) {
	msgs := []models.Message{
		models.NewUserText("do work"),
		{
			Role: models.RoleAssistant,
			Content: models.Content{
				models.ToolCallPart{CallID: "tlu_orphan", Name: "search", Input: map[string]any{"q": "x"}},
			},
			FinishReason: models.FinishReasonToolCalls,
		},
		// No tool result; conversation ends.
	}
	out := Apply(msgs, commonTarget())
	if len(out) != 3 {
		t.Fatalf("len=%d; want 3 (user, assistant, synthetic tool); got=%+v", len(out), out)
	}
	if out[2].Role != models.RoleTool {
		t.Errorf("synthetic message role=%s; want tool", out[2].Role)
	}
	tr, ok := out[2].Content[0].(models.ToolResultPart)
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
	msgs := []models.Message{
		models.NewUserText("q"),
		{
			Role: models.RoleAssistant,
			Content: models.Content{
				models.ToolCallPart{CallID: "tlu_a", Name: "f", Input: nil},
				models.ToolCallPart{CallID: "tlu_b", Name: "f", Input: nil},
			},
			FinishReason: models.FinishReasonToolCalls,
		},
		models.NewToolResult("tlu_a", []models.Part{models.TextPart{Text: "ok"}}, false),
		// tlu_b has no result; should get a synthetic one before user turn.
		models.NewUserText("next"),
	}
	out := Apply(msgs, commonTarget())
	// Expect: user, assistant, tool(tlu_a real), tool(tlu_b synth), user
	if len(out) != 5 {
		t.Fatalf("len=%d; want 5; got=%+v", len(out), out)
	}
	// Find the synthetic.
	var synth *models.ToolResultPart
	for i := range out {
		if out[i].Role != models.RoleTool {
			continue
		}
		tr := out[i].Content[0].(models.ToolResultPart)
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
	msgs := []models.Message{
		models.NewUserText("hello"),
		{
			Role: models.RoleAssistant,
			// Cross-model + only-reasoning means content drops to empty.
			Content: models.Content{
				models.ReasoningPart{Reasoning: "thinking"},
			},
			Origin: &models.MessageOrigin{
				Provider: "openai",
				API:      models.APIOpenAIResponses,
				ModelID:  "gpt-5",
			},
			FinishReason: models.FinishReasonStop,
		},
	}
	out := Apply(msgs, target)
	if len(out) != 1 {
		t.Fatalf("len=%d; want 1 (assistant elided); got=%+v", len(out), out)
	}
	if out[0].Role != models.RoleUser {
		t.Errorf("survivor role=%s; want user", out[0].Role)
	}
}

func TestNormalizeToolCallIDAppliedCrossModel(t *testing.T) {
	target := commonTarget()
	target.NormalizeToolCallID = func(id string) string {
		return "norm_" + id
	}
	msgs := []models.Message{
		models.NewUserText("q"),
		{
			Role: models.RoleAssistant,
			Content: models.Content{
				models.ToolCallPart{CallID: "raw_xyz", Name: "f", Input: nil},
			},
			Origin: &models.MessageOrigin{
				Provider: "openai",
				API:      models.APIOpenAIResponses,
				ModelID:  "gpt-5",
			},
			FinishReason: models.FinishReasonToolCalls,
		},
		models.NewToolResult("raw_xyz", []models.Part{models.TextPart{Text: "ok"}}, false),
	}
	out := Apply(msgs, target)
	tc := out[1].Content[0].(models.ToolCallPart)
	tr := out[2].Content[0].(models.ToolResultPart)
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
	msgs := []models.Message{
		{
			Role: models.RoleAssistant,
			Content: models.Content{
				models.ToolCallPart{CallID: "tlu_native", Name: "f", Input: nil},
			},
			Origin:       originOf(target),
			FinishReason: models.FinishReasonToolCalls,
		},
		models.NewToolResult("tlu_native", []models.Part{models.TextPart{Text: "ok"}}, false),
	}
	out := Apply(msgs, target)
	if called {
		t.Error("NormalizeToolCallID should not be invoked for same-model assistant messages")
	}
	tc := out[0].Content[0].(models.ToolCallPart)
	if tc.CallID != "tlu_native" {
		t.Errorf("CallID changed unexpectedly: got %q", tc.CallID)
	}
}

func TestMessageWithNoOriginIsTreatedAsCrossModel(t *testing.T) {
	// Older sessions / synthetic messages won't have Origin. SameModel
	// returns false on nil, so they should be treated as cross-model
	// (lossy transforms apply).
	target := commonTarget()
	msgs := []models.Message{
		{
			Role: models.RoleAssistant,
			Content: models.Content{
				models.ReasoningPart{Reasoning: "?"},
				models.TextPart{Text: "answer"},
			},
			// Origin: nil
			FinishReason: models.FinishReasonStop,
		},
	}
	out := Apply(msgs, target)
	for _, p := range out[0].Content {
		if _, ok := p.(models.ReasoningPart); ok {
			t.Error("ReasoningPart should be dropped when Origin is nil (cross-model fallback)")
		}
	}
}
