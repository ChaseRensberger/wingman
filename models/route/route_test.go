package route

import (
	"context"
	"net/http"
	"testing"

	"github.com/chaserensberger/wingman/models"
)

type testProtocol struct{}

func (testProtocol) API() models.API { return models.APIOpenAICompletions }
func (testProtocol) Prepare(context.Context, ModelRef, models.Request) (*PreparedBody, error) {
	h := make(http.Header)
	h.Set("X-Protocol", "yes")
	return &PreparedBody{Body: []byte(`{"ok":true}`), Headers: h, Meta: map[string]any{"protocol": "test"}}, nil
}
func (testProtocol) ParseStream(context.Context, ModelRef, *http.Response, *models.EventStream[models.StreamPart, *models.Message]) {
}
func (testProtocol) CountTokens(context.Context, ModelRef, []models.Message) (int, error) {
	return 0, nil
}

func TestRoutePrepareComposesEndpointHeadersAndAuth(t *testing.T) {
	refHeaders := make(http.Header)
	refHeaders.Set("X-Model", "yes")
	r := Route{
		ID:       "test-route",
		Protocol: testProtocol{},
		Endpoint: Path("/chat/completions"),
		Auth:     Bearer("test-token"),
	}
	prepared, err := r.Prepare(context.Background(), ModelRef{
		Provider: "test",
		ModelID:  "model",
		BaseURL:  "https://example.com/v1/",
		Headers:  refHeaders,
	}, models.Request{})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if prepared.Method != http.MethodPost {
		t.Fatalf("method = %q, want POST", prepared.Method)
	}
	if prepared.URL != "https://example.com/v1/chat/completions" {
		t.Fatalf("url = %q", prepared.URL)
	}
	if got := prepared.Headers.Get("Authorization"); got != "Bearer test-token" {
		t.Fatalf("authorization = %q", got)
	}
	if got := prepared.Headers.Get("X-Model"); got != "yes" {
		t.Fatalf("model header = %q", got)
	}
	if got := prepared.Headers.Get("X-Protocol"); got != "yes" {
		t.Fatalf("protocol header = %q", got)
	}
	if string(prepared.Body) != `{"ok":true}` {
		t.Fatalf("body = %s", prepared.Body)
	}
}
