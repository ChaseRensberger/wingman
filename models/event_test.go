package models

import (
	"reflect"
	"testing"
)

func TestStreamPartRoundTrip(t *testing.T) {
	cases := []StreamPart{
		StreamStartPart{Warnings: []Warning{{Type: "unsupported_setting", Message: "top_k ignored", Setting: "top_k"}}},
		TextStartPart{ID: "txt_1"},
		TextDeltaPart{ID: "txt_1", Delta: "hello"},
		TextEndPart{ID: "txt_1"},
		ReasoningStartPart{ID: "r_1"},
		ReasoningDeltaPart{ID: "r_1", Delta: "thinking..."},
		ReasoningEndPart{ID: "r_1"},
		ToolInputStartPart{ID: "call_1", ToolName: "bash"},
		ToolInputDeltaPart{ID: "call_1", Delta: `{"cmd":`},
		ToolInputEndPart{ID: "call_1"},
		ToolCallPart_{ID: "call_1", ToolName: "bash", Input: map[string]any{"cmd": "ls"}},
		ResponseMetadataPart{ResponseMetadata: ResponseMetadata{ID: "msg_abc", ModelID: "claude-sonnet-4"}},
		FinishPart{
			Reason: FinishReasonStop,
			Usage:  Usage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150},
			Message: &Message{
				Role:    RoleAssistant,
				Content: Content{TextPart{Text: "hello"}},
			},
		},
		ErrorPart{Message: "rate limited", Code: "rate_limited"},
	}
	for _, in := range cases {
		raw, err := MarshalStreamPart(in)
		if err != nil {
			t.Fatalf("marshal %T: %v", in, err)
		}
		out, err := UnmarshalStreamPart(raw)
		if err != nil {
			t.Fatalf("unmarshal %T (%s): %v", in, raw, err)
		}
		if !reflect.DeepEqual(in, out) {
			t.Errorf("%T: in=%#v out=%#v raw=%s", in, in, out, raw)
		}
	}
}
