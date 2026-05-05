package openai

import (
	"errors"
	"testing"

	"github.com/chaserensberger/wingman/models"
)

func drainFinish(t *testing.T, out *models.EventStream[models.StreamPart, *models.Message]) (models.FinishPart, *models.Message) {
	t.Helper()
	var fp models.FinishPart
	for part := range out.Iter() {
		if v, ok := part.(models.FinishPart); ok {
			fp = v
		}
	}
	msg, _ := out.Final()
	return fp, msg
}

func wantOrigin(t *testing.T, got *models.MessageOrigin, modelID string) {
	t.Helper()
	if got == nil {
		t.Fatal("Origin is nil; expected to be stamped on assembled message")
	}
	if got.Provider != "openai" {
		t.Errorf("Origin.Provider = %q, want openai", got.Provider)
	}
	if got.API != models.APIOpenAIResponses {
		t.Errorf("Origin.API = %q, want %q", got.API, models.APIOpenAIResponses)
	}
	if got.ModelID != modelID {
		t.Errorf("Origin.ModelID = %q, want %q", got.ModelID, modelID)
	}
}

func TestStreamParser_StampsOriginAndFinishReason_Normal(t *testing.T) {
	out := models.NewEventStream[models.StreamPart, *models.Message](16)
	origin := &models.MessageOrigin{
		Provider: "openai",
		API:      models.APIOpenAIResponses,
		ModelID:  "gpt-test",
	}
	p := newStreamParser(out, origin)
	p.stopReason = "completed"
	p.terminateNormal()

	fp, msg := drainFinish(t, out)
	if fp.Reason != models.FinishReasonStop {
		t.Errorf("FinishPart.Reason = %q, want stop", fp.Reason)
	}
	if msg == nil {
		t.Fatal("Final returned nil message")
	}
	wantOrigin(t, msg.Origin, "gpt-test")
	if msg.FinishReason != models.FinishReasonStop {
		t.Errorf("msg.FinishReason = %q, want stop", msg.FinishReason)
	}
}

func TestStreamParser_StampsOriginAndFinishReason_Error(t *testing.T) {
	out := models.NewEventStream[models.StreamPart, *models.Message](16)
	origin := &models.MessageOrigin{
		Provider: "openai",
		API:      models.APIOpenAIResponses,
		ModelID:  "gpt-test",
	}
	p := newStreamParser(out, origin)
	p.terminate(models.FinishReasonError, errors.New("boom"))

	_, msg := drainFinish(t, out)
	if msg == nil {
		t.Fatal("Final returned nil message")
	}
	wantOrigin(t, msg.Origin, "gpt-test")
	if msg.FinishReason != models.FinishReasonError {
		t.Errorf("msg.FinishReason = %q, want error", msg.FinishReason)
	}
}

func TestStreamParser_StampsOriginAndFinishReason_Aborted(t *testing.T) {
	out := models.NewEventStream[models.StreamPart, *models.Message](16)
	origin := &models.MessageOrigin{
		Provider: "openai",
		API:      models.APIOpenAIResponses,
		ModelID:  "gpt-test",
	}
	p := newStreamParser(out, origin)
	p.terminateAborted()

	_, msg := drainFinish(t, out)
	if msg == nil {
		t.Fatal("Final returned nil message")
	}
	wantOrigin(t, msg.Origin, "gpt-test")
	if msg.FinishReason != models.FinishReasonAborted {
		t.Errorf("msg.FinishReason = %q, want aborted", msg.FinishReason)
	}
}

func TestClient_InfoStampsAPI(t *testing.T) {
	c := &Client{model: "gpt-some-unknown-id", baseURL: defaultBaseURL}
	info := c.Info()
	if info.API != models.APIOpenAIResponses {
		t.Errorf("Info().API = %q, want %q", info.API, models.APIOpenAIResponses)
	}
	if info.Provider != "openai" {
		t.Errorf("Info().Provider = %q, want openai", info.Provider)
	}
}
