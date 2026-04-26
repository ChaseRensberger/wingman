package ollama

import (
	"errors"
	"testing"

	"github.com/chaserensberger/wingman/wingmodels"
)

func drainFinish(t *testing.T, out *wingmodels.EventStream[wingmodels.StreamPart, *wingmodels.Message]) *wingmodels.Message {
	t.Helper()
	for range out.Iter() {
	}
	msg, _ := out.Final()
	return msg
}

func wantOrigin(t *testing.T, got *wingmodels.MessageOrigin, modelID string) {
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
	out := wingmodels.NewEventStream[wingmodels.StreamPart, *wingmodels.Message](16)
	origin := &wingmodels.MessageOrigin{Provider: "ollama", ModelID: "llama-test"}
	p := newStreamParser(out, origin)
	p.doneReason = "stop"
	p.terminateNormal()

	msg := drainFinish(t, out)
	if msg == nil {
		t.Fatal("Final returned nil message")
	}
	wantOrigin(t, msg.Origin, "llama-test")
	if msg.FinishReason != wingmodels.FinishReasonStop {
		t.Errorf("msg.FinishReason = %q, want stop", msg.FinishReason)
	}
}

func TestStreamParser_StampsOriginAndFinishReason_Error(t *testing.T) {
	out := wingmodels.NewEventStream[wingmodels.StreamPart, *wingmodels.Message](16)
	origin := &wingmodels.MessageOrigin{Provider: "ollama", ModelID: "llama-test"}
	p := newStreamParser(out, origin)
	p.terminateError(errors.New("boom"))

	msg := drainFinish(t, out)
	if msg == nil {
		t.Fatal("Final returned nil message")
	}
	wantOrigin(t, msg.Origin, "llama-test")
	if msg.FinishReason != wingmodels.FinishReasonError {
		t.Errorf("msg.FinishReason = %q, want error", msg.FinishReason)
	}
}

func TestStreamParser_StampsOriginAndFinishReason_Aborted(t *testing.T) {
	out := wingmodels.NewEventStream[wingmodels.StreamPart, *wingmodels.Message](16)
	origin := &wingmodels.MessageOrigin{Provider: "ollama", ModelID: "llama-test"}
	p := newStreamParser(out, origin)
	p.terminateAborted()

	msg := drainFinish(t, out)
	if msg == nil {
		t.Fatal("Final returned nil message")
	}
	wantOrigin(t, msg.Origin, "llama-test")
	if msg.FinishReason != wingmodels.FinishReasonAborted {
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
