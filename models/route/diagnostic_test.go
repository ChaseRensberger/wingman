package route_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/models/protocols"
	"github.com/chaserensberger/wingman/models/route"
	"github.com/chaserensberger/wingman/tool"
)

func TestFailureDiagnostics(t *testing.T) {
	for _, tt := range []struct {
		name     string
		protocol route.Protocol
		status   int
		body     string
		stage    string
		code     string
		output   bool
	}{
		{"responses stream", protocols.Responses{}, 200, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial output\"}\n\ndata: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"model_not_supported\",\"message\":\"Model luna cannot use this endpoint\",\"param\":\"model\"},\"output\":\"private response body\"}}\n\n", "stream", "model_not_supported", true},
		{"chat stream", protocols.Chat{}, 200, "data: {\"error\":{\"code\":\"model_not_supported\",\"message\":\"Model luna cannot use this endpoint\",\"param\":\"model\"}}\n\n", "stream", "model_not_supported", false},
		{"HTTP rejection", protocols.Responses{}, 400, `{"error":{"code":"model_not_supported","message":"Model luna cannot use this endpoint","param":"model"},"request":"private request body"}`, "response", "model_not_supported", false},
		{"anthropic stream", protocols.Anthropic{}, 200, "data: {\"type\":\"error\",\"error\":{\"type\":\"invalid_request_error\",\"message\":\"Model luna cannot use this endpoint\"}}\n\n", "stream", "", false},
		{"gemini stream", protocols.Gemini{}, 200, "data: {\"error\":{\"code\":400,\"status\":\"INVALID_ARGUMENT\",\"message\":\"Model luna cannot use this endpoint\"}}\n\n", "stream", "400", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			model := diagnosticModel(tt.protocol, tt.status, strings.NewReader(tt.body))
			_, err := model.Generate(context.Background(), models.Request{Generation: models.Generation{MaxTokens: 123}})
			var failure *models.ProviderError
			if !errors.As(err, &failure) || failure.Diagnostic == nil {
				t.Fatalf("missing diagnostic: %v", err)
			}
			d := failure.Diagnostic
			if d.Stage != tt.stage || d.Code != tt.code || d.Message != "Model luna cannot use this endpoint" || d.OutputStarted != tt.output {
				t.Fatalf("diagnostic = %#v", d)
			}
			if d.Model != "luna" || d.Protocol != tt.protocol.ID() || d.Route != "test-route" || d.HTTPStatus != tt.status || d.RequestID != "req_diagnostic" || d.Endpoint != "https://provider.test/responses" {
				t.Fatalf("missing request context: %#v", d)
			}
			if len(d.Settings) == 0 {
				t.Fatal("missing effective settings")
			}
			wantBody := tt.body
			if tt.stage == "stream" {
				wantBody = strings.TrimSpace(strings.SplitN(tt.body[strings.LastIndex(tt.body, "data: ")+len("data: "):], "\n", 2)[0])
			}
			if d.Body != wantBody || d.ResponseHeaders["x-request-id"][0] != "req_diagnostic" || d.Transport == nil || d.Transport.Delivery != map[bool]string{true: "accepted", false: "rejected"}[tt.stage == "stream"] {
				t.Fatalf("lost native evidence or HTTP context: %#v", d)
			}
			encoded, _ := json.Marshal(d)
			if !strings.Contains(string(encoded), "cannot use this endpoint") || strings.Contains(err.Error(), "cannot use this endpoint") || d.Body == "" {
				t.Fatalf("missing evidence or native detail in public error: %s, %v", encoded, err)
			}
		})
	}
}

func diagnosticModel(protocol route.Protocol, status int, body io.Reader) *route.Model {
	return &route.Model{
		Info_: models.ModelInfo{Provider: "openai", ID: "luna"},
		Route: route.Route{
			ID: "test-route", Protocol: protocol, Framing: route.SSE,
			Endpoint: route.Endpoint{BaseURL: "https://provider.test", Path: "/responses"},
			Transport: route.HTTP{Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Header: http.Header{"X-Request-Id": {"req_diagnostic"}}, Body: io.NopCloser(body)}, nil
			})}},
		},
	}
}

func TestFailureDiagnosticRedactsCredentials(t *testing.T) {
	for _, status := range []int{200, 400} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			message := "Unsupported input: private prompt; private system; private tool result; auth-secret-value; query-secret-value; header-secret-value; sk-other-secret. Use the Responses API."
			body, _ := json.Marshal(map[string]any{"error": map[string]any{"code": "auth-secret-value", "type": "query-secret-value", "param": "header-secret-value", "message": message}, "input": "private prompt"})
			if status == 200 {
				body = []byte("data: " + string(body) + "\n\n")
			}
			model := diagnosticModel(protocols.Chat{}, status, strings.NewReader(string(body)))
			model.Route.Auth = route.BearerAuth("auth-secret-value")
			model.Route.Endpoint.Query = map[string]string{"key": "query-secret-value"}
			req := models.Request{
				System:   "private system",
				Messages: []models.Message{models.NewUserText("private prompt"), {Role: models.RoleTool, Content: models.Content{models.ToolResultPart{CallID: "call", Output: []models.Part{models.TextPart{Text: "private tool result"}}}}}},
				HTTP:     models.HTTPOptions{Headers: map[string]string{"x-private": "header-secret-value"}},
			}
			_, err := model.Generate(context.Background(), req)
			var failure *models.ProviderError
			if !errors.As(err, &failure) || failure.Diagnostic == nil {
				t.Fatalf("missing diagnostic: %v", err)
			}
			d := failure.Diagnostic
			encoded, _ := json.Marshal(d)
			for _, secret := range []string{"auth-secret-value", "query-secret-value", "header-secret-value", "sk-other-secret"} {
				if strings.Contains(string(encoded), secret) {
					t.Fatalf("diagnostic exposed %q: %s", secret, encoded)
				}
			}
			if !d.Redacted || !strings.Contains(d.Message, "Use the Responses API") || d.Code != "[redacted]" || d.Type != "[redacted]" || d.Param != "[redacted]" || d.Endpoint != "https://provider.test/responses" {
				t.Fatalf("diagnostic = %#v", d)
			}
		})
	}
}

func TestFailureDiagnosticLimits(t *testing.T) {
	for _, tt := range []struct {
		name      string
		body      string
		truncated bool
		omitted   bool
	}{
		{"non JSON", "<html>private proxy response</html>", false, false},
		{"oversized body", `{"error":{"message":"` + strings.Repeat("x", 70<<10) + `"}}`, true, false},
		{"long field", `{"error":{"code":"` + strings.Repeat("x", 2049) + `"}}`, true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := diagnosticModel(protocols.Responses{}, 400, strings.NewReader(tt.body)).Generate(context.Background(), models.Request{})
			var failure *models.ProviderError
			if !errors.As(err, &failure) || failure.Diagnostic == nil {
				t.Fatalf("missing diagnostic: %v", err)
			}
			d := failure.Diagnostic
			if d.Truncated != tt.truncated || d.DetailOmitted != tt.omitted || len(d.Code) > 2048 {
				t.Fatalf("truncated = %v, omitted = %v, code bytes = %d", d.Truncated, d.DetailOmitted, len(d.Code))
			}
			if d.Body == "" || len(d.Body) > 64<<10 || !strings.HasPrefix(tt.body, d.Body) {
				t.Fatal("missing or incorrect body prefix")
			}
		})
	}
}

type brokenDiagnosticStream struct{}

func (brokenDiagnosticStream) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestBrokenStreamDiagnostic(t *testing.T) {
	partial := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"private partial output\"}\n\n"
	for _, tail := range []io.Reader{strings.NewReader(""), brokenDiagnosticStream{}} {
		_, err := diagnosticModel(protocols.Responses{}, 200, io.MultiReader(strings.NewReader(partial), tail)).Generate(context.Background(), models.Request{})
		var failure *models.ProviderError
		if !errors.As(err, &failure) || failure.Diagnostic == nil {
			t.Fatalf("missing diagnostic: %v", err)
		}
		d := failure.Diagnostic
		if !d.OutputStarted || d.Stage != "stream" || d.HTTPStatus != 200 || d.RequestID != "req_diagnostic" {
			t.Fatalf("diagnostic = %#v", d)
		}
	}
}

func TestDiagnosticPreservesNativeMessagesWithInputEchoes(t *testing.T) {
	for _, status := range []int{200, 400} {
		for _, tc := range []struct {
			name     string
			messages []models.Message
			message  string
		}{
			{"partial prompt", []models.Message{models.NewUserText("Please inspect vault-secret-791 and summarize the result.")}, "Input validation failed near 'vault-secret-791'."},
			{"serialized argument", []models.Message{{Role: models.RoleAssistant, Content: models.Content{models.ToolPart{CallID: "call_1", Name: "inspect", State: models.ToolStateCompleted, Input: map[string]any{"token": "vault-secret-791"}, Output: "done"}}}}, "Invalid token: vault-secret-791"},
		} {
			t.Run(http.StatusText(status)+"/"+tc.name, func(t *testing.T) {
				body, _ := json.Marshal(map[string]any{"type": "error", "error": map[string]any{"code": "invalid_request_error", "message": tc.message}})
				if status == 200 {
					body = []byte("data: " + string(body) + "\n\n")
				}
				_, err := diagnosticModel(protocols.Responses{}, status, strings.NewReader(string(body))).Generate(context.Background(), models.Request{Messages: tc.messages})
				var failure *models.ProviderError
				if !errors.As(err, &failure) || failure.Diagnostic == nil {
					t.Fatalf("missing diagnostic: %v", err)
				}
				encoded, err := json.Marshal(failure.Diagnostic)
				if err != nil {
					t.Fatal(err)
				}
				if failure.Diagnostic.Message != tc.message || !strings.Contains(failure.Diagnostic.Body, "vault-secret-791") || failure.Diagnostic.DetailOmitted || failure.Diagnostic.Code != "invalid_request_error" {
					t.Fatalf("diagnostic = %s", encoded)
				}
			})
		}
	}
}

func TestDiagnosticPreservesIdentifiersWithWebfetchSchema(t *testing.T) {
	definition := tool.NewWebFetchTool().Definition()
	encoded, err := json.Marshal(definition.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(encoded, &schema); err != nil {
		t.Fatal(err)
	}
	req := models.Request{Tools: []models.ToolDef{{Name: definition.Name, Description: definition.Description, InputSchema: schema}}}
	for _, param := range []string{"text.format", "messages[0].content[0].text"} {
		t.Run(param, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{"type": "error", "error": map[string]any{"code": "context_length_exceeded", "type": "invalid_request_error", "param": param}})
			_, err := diagnosticModel(protocols.Responses{}, 200, strings.NewReader("data: "+string(body)+"\n\n")).Generate(context.Background(), req)
			var failure *models.ProviderError
			if !errors.As(err, &failure) || failure.Diagnostic == nil {
				t.Fatalf("missing diagnostic: %v", err)
			}
			d := failure.Diagnostic
			if d.Code != "context_length_exceeded" || d.Type != "invalid_request_error" || d.Param != param || d.Redacted || d.DetailOmitted {
				t.Fatalf("diagnostic = %#v", d)
			}
		})
	}
}

func TestDiagnosticPreservesUnexpectedProviderFields(t *testing.T) {
	body := `{"error":{"code":"Input: private prompt","type":"Some provider prose","param":"input['private prompt']"}}`
	_, err := diagnosticModel(protocols.Responses{}, 400, strings.NewReader(body)).Generate(context.Background(), models.Request{})
	var failure *models.ProviderError
	if !errors.As(err, &failure) || failure.Diagnostic == nil {
		t.Fatalf("missing diagnostic: %v", err)
	}
	d := failure.Diagnostic
	if d.Code != "Input: private prompt" || d.Type != "Some provider prose" || d.Param != "input['private prompt']" || d.Body != body {
		t.Fatalf("diagnostic = %#v", d)
	}
}
