package protocols

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/route"
)

func TestNumericProviderErrorCodes(t *testing.T) {
	for _, tt := range []struct {
		code     string
		category models.ErrorCategory
		retry    bool
	}{
		{"400", models.ErrorInvalidRequest, false},
		{"401", models.ErrorAuthentication, false},
		{"402", models.ErrorQuota, false},
		{"403", models.ErrorAuthorization, false},
		{"408", models.ErrorTimeout, true},
		{"429", models.ErrorRateLimit, true},
		{"500", models.ErrorUnavailable, true},
		{"502", models.ErrorUnavailable, true},
		{"503", models.ErrorUnavailable, true},
		{"504", models.ErrorTimeout, true},
		{"777", models.ErrorProvider, true},
	} {
		for _, wireCode := range []string{tt.code, `"` + tt.code + `"`} {
			for _, shape := range []struct {
				name     string
				protocol route.Protocol
				data     string
			}{
				{"chat", Chat{}, `{"error":{"code":` + wireCode + `,"message":"private provider detail"}}`},
				{"responses nested", Responses{}, `{"type":"response.failed","response":{"error":{"code":` + wireCode + `,"message":"private provider detail"}}}`},
				{"responses top level", Responses{}, `{"type":"error","code":` + wireCode + `,"message":"private provider detail"}`},
			} {
				t.Run(shape.name+"/"+wireCode, func(t *testing.T) {
					stream, err := streamModel(shape.protocol, "data: "+shape.data+"\n\n").Stream(context.Background(), models.Request{})
					if err != nil {
						t.Fatal(err)
					}
					for part := range stream.Iter() {
						if _, ok := part.(models.FinishPart); ok {
							t.Fatal("failed stream emitted finish")
						}
						if p, ok := part.(models.ErrorPart); ok && strings.Contains(p.Error, "private") {
							t.Fatal("raw error leaked")
						}
					}
					_, err = stream.Final()
					var failure *models.ProviderError
					if !errors.As(err, &failure) || failure.Category != tt.category || failure.Retryable != tt.retry {
						t.Fatalf("error = %#v", err)
					}
					if failure.Status != 200 || failure.RequestID != "req_stream" || failure.Cause == nil || failure.Cause.Error() != shape.data {
						t.Fatalf("context lost: %#v", failure)
					}
					if strings.Contains(failure.Error(), "private") {
						t.Fatal("raw error leaked")
					}
				})
			}
		}
	}
}

func TestChatRetainsNonEmptyUsage(t *testing.T) {
	for _, tt := range []struct {
		name   string
		events []string
		want   int
	}{
		{"choice with empty top-level", []string{`{"choices":[{"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15}}],"usage":{}}`}, 15},
		{"prior with empty top-level", []string{`{"usage":{"total_tokens":15}}`, `{"usage":{}}`}, 15},
		{"prior with empty choice", []string{`{"usage":{"total_tokens":15}}`, `{"choices":[{"usage":{}}]}`}, 15},
		{"top-level precedence", []string{`{"choices":[{"usage":{"total_tokens":15}}],"usage":{"total_tokens":20}}`}, 20},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := "data: " + strings.Join(tt.events, "\n\ndata: ") + "\n\ndata: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
			msg, err := streamModel(Chat{}, body).Generate(context.Background(), models.Request{})
			if err != nil {
				t.Fatal(err)
			}
			if msg.Usage == nil || msg.Usage.TotalTokens != tt.want {
				t.Fatalf("usage = %#v, want %d", msg.Usage, tt.want)
			}
		})
	}
}

func TestEmptySSEEventsDoNotFailCompletion(t *testing.T) {
	for _, tt := range []struct {
		protocol route.Protocol
		terminal string
	}{
		{Responses{}, `{"type":"response.completed"}`},
		{Chat{}, `{"choices":[{"finish_reason":"stop"}]}`},
		{Anthropic{}, `{"type":"message_stop"}`},
		{Gemini{}, `{"candidates":[{"finishReason":"STOP"}]}`},
	} {
		t.Run(tt.protocol.ID(), func(t *testing.T) {
			for _, empty := range []string{"data:\n\n", "data: \r\n\r\n"} {
				msg, err := streamModel(tt.protocol, empty+"data: "+tt.terminal+"\n\n").Generate(context.Background(), models.Request{})
				if err != nil || msg.FinishReason != models.FinishReasonStop {
					t.Fatalf("message = %#v, error = %v", msg, err)
				}
			}
		})
	}
}
