package protocols

import (
	"context"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/route"
)

func TestProtocolsRejectIncompleteToolsAndMissingTerminal(t *testing.T) {
	for _, tt := range []struct {
		name     string
		protocol route.Protocol
		body     string
	}{
		{"chat done without finish", Chat{}, "data: [DONE]\n\n"},
		{"chat partial then done", Chat{}, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\ndata: [DONE]\n\n"},
		{"responses tool", Responses{}, "data: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"fc_1\",\"delta\":\"{\"}\n\ndata: {\"type\":\"response.completed\"}\n\n"},
		{"chat tool", Chat{}, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\"}}]}}]}\n\ndata: [DONE]\n\n"},
		{"anthropic tool", Anthropic{}, "data: {\"type\":\"content_block_start\",\"content_block\":{\"type\":\"tool_use\",\"id\":\"call_1\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n"},
		{"anthropic delta without stop", Anthropic{}, "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n"},
		{"gemini tool without finish", Gemini{}, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"functionCall\":{\"name\":\"lookup\",\"args\":{}}}]}}]}\n\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stream, err := streamModel(tt.protocol, tt.body).Stream(context.Background(), models.Request{})
			if err != nil {
				t.Fatal(err)
			}
			for part := range stream.Iter() {
				if _, ok := part.(models.FinishPart); ok {
					t.Fatal("invalid stream emitted finish")
				}
			}
			msg, err := stream.Final()
			if err == nil || msg.FinishReason != "" {
				t.Fatalf("message = %#v, error = %v", msg, err)
			}
		})
	}
}

func TestProtocolsMapFinishReasons(t *testing.T) {
	for _, tt := range []struct {
		name     string
		protocol route.Protocol
		event    string
		reason   models.FinishReason
	}{
		{"responses limit", Responses{}, `{"type":"response.incomplete","response":{"incomplete_details":{"reason":"max_output_tokens"}}}`, models.FinishReasonMaxTokens},
		{"chat limit", Chat{}, `{"choices":[{"finish_reason":"length"}]}`, models.FinishReasonMaxTokens},
		{"chat blocked", Chat{}, `{"choices":[{"finish_reason":"content_filter"}]}`, models.FinishReasonBlocked},
		{"anthropic limit", Anthropic{}, "{\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"max_tokens\"}}\n\ndata: {\"type\":\"message_stop\"}", models.FinishReasonMaxTokens},
		{"gemini limit", Gemini{}, `{"candidates":[{"finishReason":"MAX_TOKENS"}]}`, models.FinishReasonMaxTokens},
		{"gemini blocked prompt", Gemini{}, `{"promptFeedback":{"blockReason":"SAFETY"}}`, models.FinishReasonBlocked},
	} {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := streamModel(tt.protocol, "data: "+tt.event+"\n\n").Generate(context.Background(), models.Request{})
			if err != nil || msg.FinishReason != tt.reason {
				t.Fatalf("message = %#v, error = %v", msg, err)
			}
		})
	}
}

func TestProtocolParsersOwnResponseState(t *testing.T) {
	for _, tt := range []struct {
		protocol route.Protocol
		delta    string
	}{
		{Responses{}, `{"type":"response.output_text.delta","delta":"hello"}`},
		{Chat{}, `{"choices":[{"delta":{"content":"hello"}}]}`},
		{Anthropic{}, `{"type":"content_block_delta","delta":{"text":"hello"}}`},
		{Gemini{}, `{"candidates":[{"content":{"parts":[{"text":"hello"}]}}]}`},
	} {
		t.Run(tt.protocol.ID(), func(t *testing.T) {
			first, second := tt.protocol.NewParser(models.ModelInfo{}), tt.protocol.NewParser(models.ModelInfo{})
			if _, err := first.Step(route.Frame{Data: tt.delta}); err != nil {
				t.Fatal(err)
			}
			if joinText(first.Message().Content) != "hello" || len(second.Message().Content) != 0 {
				t.Fatal("responses share state")
			}
			if _, err := second.Step(route.Frame{Data: tt.delta}); err != nil {
				t.Fatal(err)
			}
			if joinText(first.Message().Content) != "hello" || joinText(second.Message().Content) != "hello" {
				t.Fatal("interleaved responses share state")
			}
		})
	}
}
