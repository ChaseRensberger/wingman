package ollama

import (
	"errors"
	"testing"

	"github.com/chaserensberger/wingman/models"
)

func drainFinish(t *testing.T, out *models.EventStream[models.StreamPart, *models.Message]) *models.Message {
	t.Helper()
	for range out.Iter() {
	}
	msg, _ := out.Final()
	return msg
}

func wantOrigin(t *testing.T, got *models.MessageOrigin, modelID string) {
	t.Helper()
	if got == nil {
		t.Fatal("Origin is nil; expected to be stamped on assembled message")
	}
	if got.Provider != "ollama" {
		t.Errorf("Origin.Provider = %q, want ollama", got.Provider)
	}
	// API intentionally empty for ollama; see Client.origin().
	if got.API != "" {
		t.Errorf("Origin.API = %q, want empty (ollama is single-provider)", got.API)
	}
	if got.ModelID != modelID {
		t.Errorf("Origin.ModelID = %q, want %q", got.ModelID, modelID)
	}
}

func TestStreamParser_StampsOriginAndFinishReason_Normal(t *testing.T) {
	out := models.NewEventStream[models.StreamPart, *models.Message](16)
	origin := &models.MessageOrigin{Provider: "ollama", ModelID: "llama-test"}
	p := newStreamParser(out, origin)
	p.doneReason = "stop"
	p.terminateNormal()

	msg := drainFinish(t, out)
	if msg == nil {
		t.Fatal("Final returned nil message")
	}
	wantOrigin(t, msg.Origin, "llama-test")
	if msg.FinishReason != models.FinishReasonStop {
		t.Errorf("msg.FinishReason = %q, want stop", msg.FinishReason)
	}
}

func TestStreamParser_StampsOriginAndFinishReason_Error(t *testing.T) {
	out := models.NewEventStream[models.StreamPart, *models.Message](16)
	origin := &models.MessageOrigin{Provider: "ollama", ModelID: "llama-test"}
	p := newStreamParser(out, origin)
	p.terminateError(errors.New("boom"))

	msg := drainFinish(t, out)
	if msg == nil {
		t.Fatal("Final returned nil message")
	}
	wantOrigin(t, msg.Origin, "llama-test")
	if msg.FinishReason != models.FinishReasonError {
		t.Errorf("msg.FinishReason = %q, want error", msg.FinishReason)
	}
}

func TestStreamParser_StampsOriginAndFinishReason_Aborted(t *testing.T) {
	out := models.NewEventStream[models.StreamPart, *models.Message](16)
	origin := &models.MessageOrigin{Provider: "ollama", ModelID: "llama-test"}
	p := newStreamParser(out, origin)
	p.terminateAborted()

	msg := drainFinish(t, out)
	if msg == nil {
		t.Fatal("Final returned nil message")
	}
	wantOrigin(t, msg.Origin, "llama-test")
	if msg.FinishReason != models.FinishReasonAborted {
		t.Errorf("msg.FinishReason = %q, want aborted", msg.FinishReason)
	}
}

func TestClient_InfoLeavesAPIEmpty(t *testing.T) {
	c := &Client{model: "llama-some-unknown-id"}
	info := c.Info()
	// Ollama deliberately does not stamp API; see model.go API doc.
	if info.API != "" {
		t.Errorf("Info().API = %q, want empty (ollama is single-provider)", info.API)
	}
	if info.Provider != "ollama" {
		t.Errorf("Info().Provider = %q, want ollama", info.Provider)
	}
}
