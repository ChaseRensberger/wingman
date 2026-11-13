package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/chaserensberger/wingman/execution"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/plugin/protocol"
	"github.com/chaserensberger/wingman/pluginhost"
	"github.com/chaserensberger/wingman/store"
	"github.com/go-chi/chi/v5"
)

func (s *Server) modelProviders() *provider.Registry {
	if s.scopes != nil {
		return s.scopes.Providers()
	}
	return s.providers
}

func (s *Server) requestProviders(w http.ResponseWriter, r *http.Request) (*provider.Registry, func(), bool) {
	activation, release, err := s.requestPluginActivation(r, r.URL.Query().Get("working_directory"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return nil, func() {}, false
	}
	if activation == nil {
		return s.providers, release, true
	}
	registry, err := activation.ProviderRegistry(r.Context(), s.providers)
	if err != nil {
		release()
		s.writeError(w, http.StatusBadRequest, err.Error())
		return nil, func() {}, false
	}
	return registry, release, true
}

func (s *Server) requestPluginActivation(r *http.Request, directory string) (*pluginhost.Activation, func(), error) {
	identity := protocol.Identity{}
	if !s.Ephemeral() {
		client, err := s.resolveClientID(r)
		if err != nil {
			return nil, nil, err
		}
		identity.ClientID = client
	}
	scope, release, err := s.executionScope(r.Context(), directory)
	if err != nil {
		return nil, nil, err
	}
	if scope != nil {
		identity.WorkDir = scope.WorkDir()
	}
	activation, err := s.acquirePlugins(r.Context(), scope, identity)
	if err != nil {
		release()
		return nil, nil, err
	}
	return activation, func() {
		if activation != nil {
			_ = activation.Close(context.Background())
		}
		release()
	}, nil
}

func (s *Server) acquirePlugins(ctx context.Context, scope *execution.Scope, identity protocol.Identity) (*pluginhost.Activation, error) {
	if scope == nil || scope.Plugins() == nil {
		return nil, nil
	}
	scope.Plugins().SetHostHandler(s.pluginHostCall)
	return scope.AcquirePlugins(identity)
}

func (s *Server) preparePluginPrompt(ctx context.Context, sess *store.Session, agentID, requestID, message string) (string, error) {
	scope, release, err := s.executionScope(ctx, sess.WorkDir)
	if err != nil {
		return "", err
	}
	defer release()
	workDir := sess.WorkDir
	if scope != nil {
		workDir = scope.WorkDir()
	}
	activation, err := s.acquirePlugins(ctx, scope, protocol.Identity{ClientID: sess.ClientID, SessionID: sess.ID, AgentID: agentID, WorkDir: workDir})
	if err != nil {
		return "", err
	}
	if activation == nil {
		return message, ctx.Err()
	}
	defer activation.Close(context.Background())
	return activation.PreparePrompt(ctx, requestID, message)
}

func (s *Server) pluginHostCall(ctx context.Context, method string, input json.RawMessage) (any, error) {
	if strings.HasPrefix(method, "events.") {
		return s.pluginEventCall(ctx, method, input)
	}
	if strings.HasPrefix(method, "storage.") || strings.HasPrefix(method, "credential.") {
		state := s.pluginState
		if state.Root == "" {
			var err error
			state, err = pluginhost.DefaultState()
			if err != nil {
				return nil, err
			}
		}
		owner := pluginhost.PluginID(ctx)
		if strings.HasPrefix(method, "credential.") {
			return state.Credential(ctx, owner, method, input)
		}
		return state.Storage(ctx, owner, method, input)
	}
	if method != "api.request" {
		return nil, fmt.Errorf("unknown host operation %q", method)
	}
	identity := pluginhost.Identity(ctx)
	if identity.ClientID == "" {
		return nil, errors.New("API callbacks require a client-owned invocation")
	}
	var inputRequest protocol.HTTPRequest
	if err := json.Unmarshal(input, &inputRequest); err != nil {
		return nil, err
	}
	uri, err := url.ParseRequestURI(inputRequest.Path)
	if err != nil || uri.IsAbs() || uri.Host != "" || !strings.HasPrefix(uri.Path, "/") {
		return nil, errors.New("host API path must be server-relative")
	}
	allowed := false
	for _, prefix := range []string{"/sessions", "/agents", "/workspaces", "/tools", "/mcp", "/actions", "/catalog", "/provider", "/rpc-plugins"} {
		if uri.Path == prefix || strings.HasPrefix(uri.Path, prefix+"/") {
			allowed = true
			break
		}
	}
	if !allowed || strings.Contains(uri.Path, "/auth") || strings.Contains(uri.Path, "/oauth") || uri.Path == "/rpc-plugins/reload" || strings.HasSuffix(uri.Path, "/events") {
		return nil, errors.New("host API operation is not available to plugins")
	}
	if strings.HasPrefix(uri.Path, "/rpc-plugins") && uri.Path != "/rpc-plugins/call" {
		return nil, errors.New("plugin management requires a user request")
	}
	ctx = context.WithValue(ctx, chi.RouteCtxKey, (*chi.Context)(nil))
	request, err := http.NewRequestWithContext(ctx, inputRequest.Method, inputRequest.Path, bytes.NewReader(inputRequest.Body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Wingman-Client", identity.ClientID)
	// The bridge supplies authentication internally. Credentials never cross the plugin connection.
	if s.password != "" {
		request.SetBasicAuth(s.username, s.password)
	}
	response := httptest.NewRecorder()
	s.router.ServeHTTP(response, request)
	if !json.Valid(response.Body.Bytes()) {
		return nil, errors.New("host API operation did not return JSON")
	}
	return protocol.HTTPResponse{Status: response.Code, Body: response.Body.Bytes()}, nil
}
