package session

import (
	"encoding/json"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
)

func TestStoredMessageToModelAppliesModelCall(t *testing.T) {
	usage := models.Usage{
		InputTokens:       100,
		OutputTokens:      20,
		TotalTokens:       120,
		ReasoningTokens:   5,
		CachedInputTokens: 10,
		CacheWriteTokens:  15,
	}
	msg := models.Message{
		Role:     models.RoleAssistant,
		Content:  models.Content{models.TextPart{Text: "hello"}},
		Metadata: models.Meta{"k": "v"},
	}
	metadata, err := marshalMessageMetadata(msg)
	if err != nil {
		t.Fatalf("marshal metadata failed: %v", err)
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal json failed: %v", err)
	}
	payloadJSON, err := models.MarshalPart(msg.Content[0])
	if err != nil {
		t.Fatalf("marshal part failed: %v", err)
	}

	got, err := StoredMessageToModel(store.StoredMessage{
		Role:         string(models.RoleAssistant),
		MetadataJSON: metadataJSON,
		Parts: []store.StoredPart{{
			Kind:        "text",
			PayloadJSON: payloadJSON,
		}},
	})
	if err != nil {
		t.Fatalf("StoredMessageToModel failed: %v", err)
	}
	ApplyModelCall(&got, store.ModelCall{
		Provider:          "openai",
		API:               string(models.APIOpenAIResponses),
		ModelID:           "gpt",
		FinishReason:      string(models.FinishReasonStop),
		InputTokens:       usage.InputTokens,
		OutputTokens:      usage.OutputTokens,
		TotalTokens:       usage.TotalTokens,
		ReasoningTokens:   usage.ReasoningTokens,
		CachedInputTokens: usage.CachedInputTokens,
		CacheWriteTokens:  usage.CacheWriteTokens,
	})
	if got.FinishReason != models.FinishReasonStop {
		t.Fatalf("FinishReason = %q, want %q", got.FinishReason, models.FinishReasonStop)
	}
	wantOrigin := models.MessageOrigin{Provider: "openai", API: models.APIOpenAIResponses, ModelID: "gpt"}
	if got.Origin == nil || *got.Origin != wantOrigin {
		t.Fatalf("Origin = %#v, want %#v", got.Origin, wantOrigin)
	}
	if got.Usage == nil || *got.Usage != usage {
		t.Fatalf("Usage = %#v, want %#v", got.Usage, usage)
	}
	if got.Metadata["k"] != "v" {
		t.Fatalf("Metadata[k] = %#v, want v", got.Metadata["k"])
	}
}
