package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/store"
)

const (
	defaultPermissionTimeout           = 5 * time.Minute
	defaultPermissionResolutionTimeout = 5 * time.Second
)

type permissionRequestManager struct {
	server            *Server
	timeout           time.Duration
	resolutionTimeout time.Duration
	mu                sync.Mutex
	waiters           map[string]chan store.PermissionRequest
}

func newPermissionRequestManager(server *Server, timeout time.Duration) *permissionRequestManager {
	if timeout <= 0 {
		timeout = defaultPermissionTimeout
	}
	return &permissionRequestManager{server: server, timeout: timeout, resolutionTimeout: defaultPermissionResolutionTimeout, waiters: make(map[string]chan store.PermissionRequest)}
}

func (m *permissionRequestManager) prompter(sessionID, runID string) run.PermissionPrompter {
	return permissionPrompter{manager: m, sessionID: sessionID, runID: runID}
}

type permissionPrompter struct {
	manager          *permissionRequestManager
	sessionID, runID string
}

func (p permissionPrompter) Request(ctx context.Context, info run.PermissionRequestInfo) (run.PermissionResponse, error) {
	grants, err := p.manager.server.store.ListPermissionGrants(ctx, p.sessionID)
	if err != nil {
		return "", err
	}
	granted := make(map[string]bool, len(grants))
	for _, grant := range grants {
		if grant.Action == info.Action {
			granted[grant.Resource] = true
		}
	}
	allGranted := true
	for _, resource := range info.Resources {
		if !granted[resource] {
			allGranted = false
			break
		}
	}
	if allGranted {
		return run.PermissionResponseAlways, nil
	}

	requestID := store.NewID(store.PrefixPermissionRequest)
	waiter := make(chan store.PermissionRequest, 1)
	p.manager.mu.Lock()
	p.manager.waiters[requestID] = waiter
	p.manager.mu.Unlock()
	defer p.manager.removeWaiter(requestID)

	transition, err := p.manager.server.store.CreatePermissionRequest(ctx, store.PermissionRequest{
		ID: requestID, SessionID: p.sessionID, RunID: p.runID, ToolUseID: info.ToolUseID, CallID: info.CallID,
		Action: info.Action, Resources: append([]string(nil), info.Resources...),
	})
	if err != nil {
		return "", err
	}
	if transition.Changed {
		p.manager.server.events.publish(transition.Event)
	}

	waitCtx, cancel := context.WithTimeout(ctx, p.manager.timeout)
	defer cancel()
	select {
	case request := <-waiter:
		return permissionOutcome(request)
	case <-waitCtx.Done():
		status, errorType, errorMessage := store.PermissionRequestStatusInterrupted, "permission_interrupted", "permission request interrupted"
		if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
			status, errorType, errorMessage = store.PermissionRequestStatusTimedOut, "permission_timeout", "permission request timed out"
		}
		resolveCtx, resolveCancel := context.WithTimeout(context.Background(), p.manager.resolutionTimeout)
		request, err := p.manager.resolve(resolveCtx, p.sessionID, requestID, status, "", errorType, errorMessage)
		resolveCancel()
		if err != nil {
			return "", err
		}
		return permissionOutcome(request)
	case <-p.manager.server.ShutdownCtx().Done():
		resolveCtx, resolveCancel := context.WithTimeout(context.Background(), p.manager.resolutionTimeout)
		request, err := p.manager.resolve(resolveCtx, p.sessionID, requestID, store.PermissionRequestStatusInterrupted, "", "permission_interrupted", "permission request interrupted")
		resolveCancel()
		if err != nil {
			return "", err
		}
		return permissionOutcome(request)
	}
}

func (m *permissionRequestManager) removeWaiter(requestID string) {
	m.mu.Lock()
	delete(m.waiters, requestID)
	m.mu.Unlock()
}

func (m *permissionRequestManager) resolve(ctx context.Context, sessionID, requestID, status, response, errorType, errorMessage string) (store.PermissionRequest, error) {
	transition, err := m.server.store.ResolvePermissionRequest(ctx, store.PermissionRequestResolution{
		SessionID: sessionID, RequestID: requestID, Status: status, Response: response, ErrorType: errorType, ErrorMessage: errorMessage,
	})
	if err != nil {
		if !errors.Is(err, store.ErrPermissionRequestTransitionConflict) {
			return store.PermissionRequest{}, err
		}
		request, getErr := m.server.store.GetPermissionRequest(ctx, sessionID, requestID)
		if getErr != nil {
			return store.PermissionRequest{}, getErr
		}
		return *request, nil
	}
	if transition.Changed {
		m.server.events.publish(transition.Event)
	}
	m.notify(transition.Request)
	return transition.Request, nil
}

func (m *permissionRequestManager) notify(request store.PermissionRequest) {
	m.mu.Lock()
	waiter := m.waiters[request.ID]
	m.mu.Unlock()
	if waiter != nil {
		select {
		case waiter <- request:
		default:
		}
	}
}

func permissionOutcome(request store.PermissionRequest) (run.PermissionResponse, error) {
	switch request.Status {
	case store.PermissionRequestStatusApproved:
		return run.PermissionResponse(request.Response), nil
	case store.PermissionRequestStatusRejected:
		return run.PermissionResponseReject, nil
	case store.PermissionRequestStatusTimedOut:
		return "", context.DeadlineExceeded
	case store.PermissionRequestStatusInterrupted:
		return "", context.Canceled
	default:
		return "", errors.New("permission request is not resolved")
	}
}

type permissionReplyRequest struct {
	Response string `json:"response"`
}

func (s *Server) handleListPermissionRequests(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	sessionID := chi.URLParam(r, "id")
	if _, ok := s.authorizeSessionForRequest(w, r, sessionID); !ok {
		return
	}
	requests, err := s.store.ListPermissionRequests(r.Context(), sessionID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if requests == nil {
		requests = []store.PermissionRequest{}
	}
	writeJSON(w, http.StatusOK, requests)
}

func (s *Server) handleListPermissionGrants(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	sessionID := chi.URLParam(r, "id")
	if _, ok := s.authorizeSessionForRequest(w, r, sessionID); !ok {
		return
	}
	grants, err := s.store.ListPermissionGrants(r.Context(), sessionID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if grants == nil {
		grants = []store.PermissionGrant{}
	}
	writeJSON(w, http.StatusOK, grants)
}

func (s *Server) handleReplyPermissionRequest(w http.ResponseWriter, r *http.Request) {
	if s.Ephemeral() {
		s.ephemeralNotImplemented(w)
		return
	}
	sessionID, requestID := chi.URLParam(r, "id"), chi.URLParam(r, "requestID")
	if _, ok := s.authorizeSessionForRequest(w, r, sessionID); !ok {
		return
	}
	var reply permissionReplyRequest
	if err := json.NewDecoder(r.Body).Decode(&reply); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	status := store.PermissionRequestStatusApproved
	if reply.Response == store.PermissionResponseReject {
		status = store.PermissionRequestStatusRejected
	}
	if reply.Response != store.PermissionResponseOnce && reply.Response != store.PermissionResponseAlways && reply.Response != store.PermissionResponseReject {
		s.writeError(w, http.StatusBadRequest, "response must be once, always, or reject")
		return
	}
	transition, err := s.store.ResolvePermissionRequest(r.Context(), store.PermissionRequestResolution{SessionID: sessionID, RequestID: requestID, Status: status, Response: reply.Response})
	if errors.Is(err, store.ErrPermissionRequestNotFound) {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if errors.Is(err, store.ErrPermissionRequestTransitionConflict) {
		s.writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !transition.Changed {
		if transition.Request.Status == status && transition.Request.Response == reply.Response {
			s.permissionRequests.notify(transition.Request)
			writeJSON(w, http.StatusOK, transition.Request)
			return
		}
		s.writeError(w, http.StatusConflict, "permission request is already resolved")
		return
	}
	s.events.publish(transition.Event)
	s.permissionRequests.notify(transition.Request)
	writeJSON(w, http.StatusOK, transition.Request)
}
