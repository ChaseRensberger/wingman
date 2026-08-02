package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestDiagnosticsReportsBoundedOperationalState(t *testing.T) {
	data := memory.NewStore()
	if err := data.CreateSession(&store.Session{ID: "ses_diagnostics"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"run_diagnostics_one", "run_diagnostics_two"} {
		if _, err := data.AdmitSessionRun(context.Background(), store.SessionRun{ID: id, SessionID: "ses_diagnostics"}); err != nil {
			t.Fatal(err)
		}
	}
	server := New(Config{Store: data, Credential: "secret"})
	subscription, unsubscribe := server.events.subscribe("ses_diagnostics")
	defer unsubscribe()
	for i := 0; i < cap(subscription.events)+1; i++ {
		server.events.publish(store.SessionEvent{SessionID: "ses_diagnostics"})
	}

	response := httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/diagnostics", "secret"))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var diagnostics api.DiagnosticsResponse
	if err := json.NewDecoder(response.Body).Decode(&diagnostics); err != nil {
		t.Fatal(err)
	}
	if diagnostics.QueuedRuns != 2 || diagnostics.ActiveRuns != 0 || diagnostics.EventSubscribers != 1 || diagnostics.SubscriberOverflows != 1 || diagnostics.SubscriberClosures != 0 || diagnostics.SubscriberBacklog != 256 || diagnostics.SubscriberMaxBacklog != 256 || diagnostics.PluginsRunning != 0 || diagnostics.PluginsDegraded != 0 || diagnostics.PluginsFailed != 0 || diagnostics.PluginLoadErrors != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
}
