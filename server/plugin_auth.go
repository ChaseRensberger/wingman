package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/chaserensberger/wingman/plugin/protocol"
	"github.com/chaserensberger/wingman/pluginhost"
	"github.com/chaserensberger/wingman/store"
)

type pluginAuthRequest struct {
	Plugin           string          `json:"plugin,omitempty"`
	Method           string          `json:"method,omitempty"`
	Operation        string          `json:"operation" enum:"start,status,complete,cancel"`
	Attempt          string          `json:"attempt,omitempty"`
	Input            json.RawMessage `json:"input,omitempty"`
	WorkingDirectory string          `json:"working_directory,omitempty"`
}

type pluginAuthStatus struct {
	ID           string `json:"id"`
	Plugin       string `json:"plugin"`
	Method       string `json:"method"`
	Integration  string `json:"integration"`
	Status       string `json:"status" enum:"pending,connected,cancelled,failed"`
	URL          string `json:"url,omitempty"`
	Instructions string `json:"instructions,omitempty"`
}

type pluginAuthAttempt struct {
	mu         sync.Mutex
	owner      string
	status     pluginAuthStatus
	state      json.RawMessage
	activation *pluginhost.Activation
	cancel     context.CancelFunc
	release    func()
	closed     bool
	workCtx    context.Context
	workCancel context.CancelFunc
	done       chan struct{}
}

func (s *Server) handlePluginAuthMethods(w http.ResponseWriter, r *http.Request) {
	a, release, err := s.requestPluginActivation(r, r.URL.Query().Get("working_directory"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer release()
	methods := map[string][]protocol.AuthMethod{}
	if a != nil {
		methods = a.AuthMethods()
	}
	writeJSON(w, http.StatusOK, methods)
}

func (s *Server) handlePluginAuth(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	owner, err := s.resolveClientID(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var input pluginAuthRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid authentication request")
		return
	}
	var attempt *pluginAuthAttempt
	if input.Operation == "start" {
		a, release, err := s.requestPluginActivation(r, input.WorkingDirectory)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		integration := ""
		if a != nil {
			for _, method := range a.AuthMethods()[input.Plugin] {
				if method.ID == input.Method {
					integration = method.Integration
				}
			}
		}
		if integration == "" {
			release()
			s.writeError(w, http.StatusNotFound, "authentication method is unavailable")
			return
		}
		ctx, cancel := context.WithTimeout(s.shutdownCtx, 10*time.Minute)
		workCtx, workCancel := context.WithCancel(ctx)
		attempt = &pluginAuthAttempt{owner: owner, activation: a, cancel: cancel, workCtx: workCtx, workCancel: workCancel, done: make(chan struct{}), release: sync.OnceFunc(release), status: pluginAuthStatus{ID: store.NewID("auth"), Plugin: input.Plugin, Method: input.Method, Integration: integration, Status: "pending"}}
		s.pluginAuthMu.Lock()
		if s.shutdownCtx.Err() != nil || len(s.pluginAuth) >= 128 {
			s.pluginAuthMu.Unlock()
			workCancel()
			cancel()
			release()
			s.writeError(w, http.StatusServiceUnavailable, "authentication is unavailable")
			return
		}
		s.pluginAuth[attempt.status.ID] = attempt
		s.pluginAuthMu.Unlock()
		attempt.mu.Lock()
		context.AfterFunc(ctx, func() { s.closePluginAuth(attempt) })
	} else {
		s.pluginAuthMu.Lock()
		attempt = s.pluginAuth[input.Attempt]
		s.pluginAuthMu.Unlock()
		if attempt == nil || attempt.owner != owner {
			s.writeError(w, http.StatusNotFound, "authentication attempt not found")
			return
		}
		if input.Operation == "cancel" {
			attempt.workCancel()
		}
		attempt.mu.Lock()
	}
	defer attempt.mu.Unlock()
	if attempt.closed {
		s.writeError(w, http.StatusGone, "authentication attempt expired")
		return
	}
	if input.Operation != "start" && input.Operation != "status" && input.Operation != "complete" && input.Operation != "cancel" {
		s.writeError(w, http.StatusBadRequest, "invalid authentication operation")
		return
	}
	if attempt.status.Status != "pending" {
		writeJSON(w, http.StatusOK, attempt.status)
		return
	}
	ctx, cancel := context.WithTimeout(attempt.workCtx, 30*time.Second)
	stop := context.AfterFunc(r.Context(), cancel)
	result, callErr := attempt.activation.Authenticate(ctx, attempt.status.Plugin, protocol.AuthCall{Method: attempt.status.Method, Operation: input.Operation, Input: input.Input, State: attempt.state})
	if callErr == nil {
		callErr = ctx.Err()
	}
	stop()
	cancel()
	if attempt.workCtx.Err() != nil {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _ = attempt.activation.Authenticate(cleanupCtx, attempt.status.Plugin, protocol.AuthCall{Method: attempt.status.Method, Operation: "cancel", State: attempt.state})
		cleanupCancel()
		result = protocol.AuthResult{Status: "cancelled"}
		callErr = nil
	}
	if callErr == nil && result.Status != "pending" && result.Status != "connected" && result.Status != "cancelled" {
		callErr = errors.New("invalid authentication status")
	}
	if callErr == nil && result.Status == "connected" {
		if result.Credential == nil {
			callErr = errors.New("authentication completed without a credential")
		} else {
			state := s.pluginState
			if state.Root == "" {
				state, callErr = pluginhost.DefaultState()
			}
			if callErr == nil {
				raw, marshalErr := json.Marshal(map[string]any{"key": attempt.status.Integration, "value": result.Credential})
				callErr = marshalErr
				if callErr == nil {
					_, callErr = state.Credential(pluginhost.WithIdentity(r.Context(), protocol.Identity{ClientID: owner}), attempt.status.Plugin, "credential.set", raw)
				}
			}
		}
	}
	if callErr != nil {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _ = attempt.activation.Authenticate(cleanupCtx, attempt.status.Plugin, protocol.AuthCall{Method: attempt.status.Method, Operation: "cancel", State: attempt.state})
		cleanupCancel()
		attempt.status.Status = "failed"
		attempt.state = nil
		attempt.release()
		attempt.activation = nil
		// Plugin errors can contain provider response bodies or secrets.
		s.writeError(w, http.StatusBadRequest, "plugin authentication failed")
		return
	}
	attempt.state = append(json.RawMessage(nil), result.State...)
	attempt.status.Status, attempt.status.URL, attempt.status.Instructions = result.Status, result.URL, result.Instructions
	if result.Status != "pending" {
		attempt.state = nil
		attempt.release()
		attempt.activation = nil
	}
	writeJSON(w, http.StatusOK, attempt.status)
}

func (s *Server) closePluginAuth(attempt *pluginAuthAttempt) {
	attempt.mu.Lock()
	defer attempt.mu.Unlock()
	if attempt.closed {
		return
	}
	attempt.closed = true
	defer close(attempt.done)
	if attempt.status.Status == "pending" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _ = attempt.activation.Authenticate(ctx, attempt.status.Plugin, protocol.AuthCall{Method: attempt.status.Method, Operation: "cancel", State: attempt.state})
		cancel()
	}
	attempt.state = nil
	attempt.release()
	attempt.workCancel()
	attempt.cancel()
	s.pluginAuthMu.Lock()
	delete(s.pluginAuth, attempt.status.ID)
	s.pluginAuthMu.Unlock()
}

func (s *Server) waitPluginAuth(ctx context.Context) error {
	s.pluginAuthMu.Lock()
	attempts := make([]*pluginAuthAttempt, 0, len(s.pluginAuth))
	for _, attempt := range s.pluginAuth {
		attempts = append(attempts, attempt)
	}
	s.pluginAuthMu.Unlock()
	for _, attempt := range attempts {
		select {
		case <-attempt.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
