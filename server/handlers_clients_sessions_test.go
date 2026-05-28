package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestCreateSessionDefaultsToWingmanClient(t *testing.T) {
	st := memory.NewStore()
	srv := New(Config{Store: st})

	req := httptest.NewRequest(http.MethodPost, "/sessions/", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}
	var sess store.Session
	if err := json.NewDecoder(rec.Body).Decode(&sess); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	if sess.ClientID != store.DefaultClientID {
		t.Fatalf("expected default client %q, got %q", store.DefaultClientID, sess.ClientID)
	}
}

func TestListSessionsDefaultsToWingmanClientScope(t *testing.T) {
	st := memory.NewStore()
	srv := New(Config{Store: st})

	defaultSess := &store.Session{Title: "default", ClientID: store.DefaultClientID}
	if _, err := st.EnsureDefaultClient(); err != nil {
		t.Fatalf("ensure default client: %v", err)
	}
	if err := st.CreateSession(defaultSess); err != nil {
		t.Fatalf("create default session: %v", err)
	}
	other, err := st.CreateClient("other")
	if err != nil {
		t.Fatalf("create other client: %v", err)
	}
	if err := st.CreateSession(&store.Session{Title: "other", ClientID: other.ID}); err != nil {
		t.Fatalf("create other session: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/sessions/", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var sessions []store.Session
	if err := json.NewDecoder(rec.Body).Decode(&sessions); err != nil {
		t.Fatalf("decode sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != defaultSess.ID {
		t.Fatalf("expected only default session, got %#v", sessions)
	}
}

func TestCreateClientRejectsDuplicateName(t *testing.T) {
	st := memory.NewStore()
	srv := New(Config{Store: st})

	for _, body := range []string{`{"name":"my-app"}`, `{"name":"MY-APP"}`} {
		req := httptest.NewRequest(http.MethodPost, "/clients/", bytes.NewBufferString(body))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if body == `{"name":"my-app"}` && rec.Code != http.StatusCreated {
			t.Fatalf("expected first create status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
		}
		if body == `{"name":"MY-APP"}` && rec.Code != http.StatusConflict {
			t.Fatalf("expected duplicate status %d, got %d: %s", http.StatusConflict, rec.Code, rec.Body.String())
		}
	}
}

func TestCreateClientRejectsWingmanName(t *testing.T) {
	st := memory.NewStore()
	srv := New(Config{Store: st})

	req := httptest.NewRequest(http.MethodPost, "/clients/", bytes.NewBufferString(`{"name":"Wingman"}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d: %s", http.StatusConflict, rec.Code, rec.Body.String())
	}
}

func TestListWorkspacesDoesNotCreateDefault(t *testing.T) {
	st := memory.NewStore()
	srv := New(Config{Store: st})

	req := httptest.NewRequest(http.MethodGet, "/workspaces/", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var workspaces []store.Workspace
	if err := json.NewDecoder(rec.Body).Decode(&workspaces); err != nil {
		t.Fatalf("decode workspaces: %v", err)
	}
	if len(workspaces) != 0 {
		t.Fatalf("expected no workspaces, got %#v", workspaces)
	}
}

func TestCreateWorkspaceRejectsDuplicateName(t *testing.T) {
	st := memory.NewStore()
	srv := New(Config{Store: st})

	for _, body := range []string{`{"name":"Wingman"}`, `{"name":"wingman"}`} {
		req := httptest.NewRequest(http.MethodPost, "/workspaces/", bytes.NewBufferString(body))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if body == `{"name":"Wingman"}` && rec.Code != http.StatusCreated {
			t.Fatalf("expected first create status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
		}
		if body == `{"name":"wingman"}` && rec.Code != http.StatusConflict {
			t.Fatalf("expected duplicate status %d, got %d: %s", http.StatusConflict, rec.Code, rec.Body.String())
		}
	}
}

func TestMessageSessionRejectsWrongClient(t *testing.T) {
	st := memory.NewStore()
	srv := New(Config{Store: st})

	if _, err := st.EnsureDefaultClient(); err != nil {
		t.Fatalf("ensure default client: %v", err)
	}
	sess := &store.Session{Title: "default", ClientID: store.DefaultClientID}
	if err := st.CreateSession(sess); err != nil {
		t.Fatalf("create session: %v", err)
	}
	other, err := st.CreateClient("other")
	if err != nil {
		t.Fatalf("create other client: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/sessions/"+sess.ID+"/message", bytes.NewBufferString(`{"agent_id":"agt_missing","message":"hello"}`))
	req.Header.Set("X-Wingman-Client", other.ID)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d: %s", http.StatusForbidden, rec.Code, rec.Body.String())
	}
}
