package server

import (
	"context"
	"fmt"
	"net/http"
)

type principalKind uint8

const (
	rootPrincipal principalKind = iota + 1
	sessionPrincipal
)

type authPrincipal struct {
	kind     principalKind
	clientID string
	owner    bool
	cookie   bool
}

type principalContextKey struct{}

func withPrincipal(ctx context.Context, principal authPrincipal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func principalFromRequest(r *http.Request) authPrincipal {
	principal, _ := r.Context().Value(principalContextKey{}).(authPrincipal)
	return principal
}

func (s *Server) requireRoot(w http.ResponseWriter, r *http.Request) bool {
	principal := principalFromRequest(r)
	if principal.kind == rootPrincipal || principal.owner {
		return true
	}
	w.Header().Set("WWW-Authenticate", `Bearer realm="wingman"`)
	s.writeError(w, http.StatusUnauthorized, "root authentication required")
	return false
}

func (s *Server) resolveClientID(r *http.Request) (string, error) {
	if principal := principalFromRequest(r); principal.kind == sessionPrincipal {
		return principal.clientID, nil
	}
	clientID := r.Header.Get("X-Wingman-Client")
	if clientID == "" {
		client, err := s.store.EnsureDefaultClient()
		if err != nil {
			return "", err
		}
		return client.ID, nil
	}
	if _, err := s.store.GetClient(clientID); err != nil {
		return "", fmt.Errorf("client not found: %s", clientID)
	}
	return clientID, nil
}
