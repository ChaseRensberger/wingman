package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
	"github.com/danielgtaylor/huma/v2"
)

func TestModelInputRequestPaths(t *testing.T) {
	frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/console/" {
			t.Errorf("API sent to frontend: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte("console frontend"))
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
			other, err := data.CreateClient("Other")
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"ses_input", "ses_other"} {
				if err := data.CreateSession(&store.Session{ID: id, ClientID: owner.ID}); err != nil {
					t.Fatal(err)
				}
			}
			hash, blobs, err := models.SnapshotCallInput(models.Request{System: "Historical system prompt", Messages: []models.Message{models.NewUserText("transformed input")}, Instructions: []models.InstructionSnapshot{{Kind: "directory", Path: "/project/AGENTS.md", SHA256: "recorded", Content: "Historical guidance", ToolUseID: "tlu_read", ResolvedPath: "/project/file.go"}}})
			if err != nil {
				t.Fatal(err)
			}
			trace, _ := json.Marshal(models.CallTrace{Version: "1", InputHash: hash})
			if err := data.UpsertModelCall(t.Context(), store.ModelCall{ID: "mcl_input", SessionID: "ses_input", Trace: trace, MetadataJSON: trace, InputBlobs: blobs}); err != nil {
				t.Fatal(err)
			}
			if err := data.UpsertModelCall(t.Context(), store.ModelCall{ID: "mcl_old", SessionID: "ses_input"}); err != nil {
				t.Fatal(err)
			}
			s := New(Config{Store: data, ConsoleDevURL: devURL, Username: "input", Password: "test-password"})
			t.Cleanup(func() { _ = s.Close(t.Context()) })
			for _, tc := range []struct {
				path, client string
				auth         bool
				status       int
			}{
				{"/sessions/ses_input/model-calls/mcl_input/input", "", false, 401},
				{"/sessions/ses_input/model-calls/mcl_input/input", owner.ID, true, 200},
				{"/sessions/ses_input/model-calls/mcl_input/input", other.ID, true, 403},
				{"/sessions/ses_other/model-calls/mcl_input/input", owner.ID, true, 404},
				{"/sessions/ses_input/model-calls/missing/input", owner.ID, true, 404},
				{"/sessions/ses_input/model-calls/mcl_old/input", owner.ID, true, 200},
			} {
				request := httptest.NewRequest(http.MethodGet, tc.path, nil)
				if tc.auth {
					request.SetBasicAuth("input", "test-password")
				}
				if tc.client != "" {
					request.Header.Set("X-Wingman-Client", tc.client)
				}
				response := httptest.NewRecorder()
				s.ServeHTTP(response, request)
				if response.Code != tc.status {
					t.Fatalf("%s client=%s status=%d body=%s", tc.path, tc.client, response.Code, response.Body.String())
				}
				if tc.status != 200 {
					continue
				}
				var got api.ModelCallInputResponse
				if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(tc.path, "mcl_old") {
					if got.Input != nil {
						t.Fatal("invented historical input")
					}
					var body any
					if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
						t.Fatal(err)
					}
					validation := &huma.ValidateResult{}
					huma.Validate(s.protocol.OpenAPI().Components.Schemas, schemaFor(s.protocol, api.ModelCallInputResponse{}), &huma.PathBuffer{}, huma.ModeReadFromServer, body, validation)
					if len(validation.Errors) != 0 {
						t.Fatalf("older-call response violates schema: %v", validation.Errors)
					}
				} else if got.Input == nil || got.Input.System != "Historical system prompt" || got.Input.Messages[0].Content[0].(models.TextPart).Text != "transformed input" || len(got.Input.Instructions) != 1 || got.Input.Instructions[0].Content != "Historical guidance" || got.Input.Instructions[0].ToolUseID != "tlu_read" {
					t.Fatalf("input = %#v", got)
				}
			}
			if devURL != "" {
				request := httptest.NewRequest(http.MethodGet, "/console/", nil)
				request.SetBasicAuth("input", "test-password")
				response := httptest.NewRecorder()
				s.ServeHTTP(response, request)
				if response.Code != 200 || response.Body.String() != "console frontend" {
					t.Fatalf("frontend = %d %s", response.Code, response.Body.String())
				}
			}
		})
	}
}
