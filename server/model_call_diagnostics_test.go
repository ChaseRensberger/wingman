package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestModelCallDiagnosticsRequestPaths(t *testing.T) {
	frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/console/" {
			t.Errorf("API request reached Console proxy: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte("Console development frontend"))
	}))
	defer frontend.Close()
	for _, devURL := range []string{"", frontend.URL} {
		name := "production"
		if devURL != "" {
			name = "proxied development"
		}
		t.Run(name, func(t *testing.T) {
			data := memory.NewStore()
			owner, err := data.EnsureDefaultClient()
			if err != nil {
				t.Fatal(err)
			}
			if err := data.CreateSession(&store.Session{ID: "ses_diagnostic", ClientID: owner.ID}); err != nil {
				t.Fatal(err)
			}
			trace := models.CallTrace{Version: "1", Failure: &models.CallDiagnostic{
				Stage: "stream", HTTPStatus: 200, Code: "unsupported_model", Message: "Use the Responses API",
				Body: `{"error":{"message":"Use the Responses API","unknown":"preserved"}}`, BodyKind: "event",
				ResponseHeaders: map[string][]string{"x-request-id": {"req_diagnostic"}},
				Transport:       &models.TransportDiagnostic{Kind: "http", Operation: "read", Phase: "receive", Delivery: "accepted", Recovery: "fail"},
				Causes:          []models.DiagnosticCause{{Type: "*net.OpError", Message: "read: connection reset", Code: "ECONNRESET"}},
			}}
			encoded, err := json.Marshal(trace)
			if err != nil {
				t.Fatal(err)
			}
			if err := data.UpsertModelCall(context.Background(), store.ModelCall{ID: "mcl_diagnostic", SessionID: "ses_diagnostic", Status: store.ModelCallStatusFailed, Trace: encoded}); err != nil {
				t.Fatal(err)
			}
			s := New(Config{Store: data, ConsoleDevURL: devURL, Username: "diagnostic", Password: "test-password"})
			t.Cleanup(func() { _ = s.Close(t.Context()) })
			for _, authenticated := range []bool{false, true} {
				request := httptest.NewRequest(http.MethodGet, "/sessions/ses_diagnostic/model-calls", nil)
				if authenticated {
					request.SetBasicAuth("diagnostic", "test-password")
				}
				response := httptest.NewRecorder()
				s.ServeHTTP(response, request)
				if !authenticated {
					if response.Code != http.StatusUnauthorized {
						t.Fatalf("unauthenticated status = %d", response.Code)
					}
					continue
				}
				if response.Code != http.StatusOK {
					t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
				}
				var calls []api.ModelCall
				if err := json.Unmarshal(response.Body.Bytes(), &calls); err != nil || len(calls) != 1 {
					t.Fatalf("calls = %#v, error = %v", calls, err)
				}
				var got models.CallTrace
				if err := json.Unmarshal(calls[0].Trace, &got); err != nil || !reflect.DeepEqual(got.Failure, trace.Failure) {
					t.Fatalf("trace = %#v, error = %v", got, err)
				}
			}
			if devURL != "" {
				request := httptest.NewRequest(http.MethodGet, "/console/", nil)
				request.SetBasicAuth("diagnostic", "test-password")
				response := httptest.NewRecorder()
				s.ServeHTTP(response, request)
				if response.Code != http.StatusOK || response.Body.String() != "Console development frontend" {
					t.Fatalf("Console proxy status = %d, body = %s", response.Code, response.Body.String())
				}
			}
		})
	}
}
