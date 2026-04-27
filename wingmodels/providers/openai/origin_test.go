package openai

import (
	"errors"
	"testing"

	"github.com/chaserensberger/wingman/wingmodels"
)

func drainFinish(t *testing.T, out *wingmodels.EventStream[wingmodels.StreamPart, *wingmodels.Message]) (wingmodels.FinishPart, *wingmodels.Message) {
	t.Helper()
	var fp wingmodels.FinishPart
	for part := range out.Iter() {
		if v, ok := part.(wingmodels.FinishPart); ok {
			fp = v
		}
	}
	msg, _ := out.Final()
	return fp, msg
}

func wantOrigin(t *testing.T, got *wingmodels.MessageOrigin, modelID string) {
	t.Helper()
	if got == nil {
		t.Fatal("Origin is nil; expected to be stamped on assembled message")
	}
	if got.Provider != "openai" {
		t.Errorf("Origin.Provider = %q, want openai", got.Provider)
	}
	if got.API != wingmodels.APIOpenAIResponses {
		t.Errorf("Origin.API = %q, want %q", got.API, wingmodels.APIOpenAIResponses)
	}
	if got.ModelID != modelID {
		t.Errorf("Origin.ModelID = %q, want %q", got.ModelID, modelID)
	}
}

func TestStreamParser_StampsOriginAndFinishReason_Normal(t *testing.T) {
	out := wingmodels.NewEventStream[wingmodels.StreamPart, *wingmodels.Message](16)
	origin := &wingmodels.MessageOrigin{
		Provider: "openai",
		API:      wingmodels.APIOpenAIResponses,
		ModelID:  "gpt-test",
	}
	p := newStreamParser(out, origin)
	p.stopReason = "completed"
	p.terminateNormal()

	fp, msg := drainFinish(t, out)
	if fp.Reason != wingmodels.FinishReasonStop {
		t.Errorf("FinishPart.Reason = %q, want stop", fp.Reason)
	}
	if msg == nil {
		t.Fatal("Final returned nil message")
	}
	wantOrigin(t, msg.Origin, "gpt-test")
	if msg.FinishReason != wingmodels.FinishReasonStop {
		t.Errorf("msg.FinishReason = %q, want stop", msg.FinishReason)
	}
}

func TestStreamParser_StampsOriginAndFinishReason_Error(t *testing.T) {
	out := wingmodels.NewEventStream[wingmodels.StreamPart, *wingmodels.Message](16)
	origin := &wingmodels.MessageOrigin{
		Provider: "openai",
		API:      wingmodels.APIOpenAIResponses,
		ModelID:  "gpt-test",
	}
	p := newStreamParser(out, origin)
	p.terminate(wingmodels.FinishReasonError, errors.New("boom"))

	_, msg := drainFinish(t, out)
	if msg == nil {
		t.Fatal("Final returned nil message")
	}
	wantOrigin(t, msg.Origin, "gpt-test")
	if msg.FinishReason != wingmodels.FinishReasonError {
		t.Errorf("msg.FinishReason = %q, want error", msg.FinishReason)
	}
}

func TestStreamParser_StampsOriginAndFinishReason_Aborted(t *testing.T) {
	out := wingmodels.NewEventStream[wingmodels.StreamPart, *wingmodels.Message](16)
	origin := &wingmodels.MessageOrigin{
		Provider: "openai",
		API:      wingmodels.APIOpenAIResponses,
		ModelID:  "gpt-test",
	}
	p := newStreamParser(out, origin)
	p.terminateAborted()

	_, msg := drainFinish(t, out)
	if msg == nil {
		t.Fatal("Final returned nil message")
	}
	wantOrigin(t, msg.Origin, "gpt-test")
	if msg.FinishReason != wingmodels.FinishReasonAborted {
		t.Errorf("msg.FinishReason = %q, want aborted", msg.FinishReason)
	}
}

func TestClient_InfoStampsAPI(t *testing.T) {
	c := &Client{model: "gpt-some-unknown-id", baseURL: defaultBaseURL}
	info := c.Info()
	if info.API != wingmodels.APIOpenAIResponses {
		t.Errorf("Info().API = %q, want %q", info.API, wingmodels.APIOpenAIResponses)
	}
	if info.Provider != "openai" {
		t.Errorf("Info().Provider = %q, want openai", info.Provider)
	}
}
