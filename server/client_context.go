package server

import (
	"context"
	"fmt"
	"net/http"
)

type authPrincipal struct {
	authenticated bool
	cookie        bool
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
	if s.password == "" || principal.authenticated {
		return true
	}
	w.Header().Set("WWW-Authenticate", `Basic realm="wingman"`)
	s.writeError(w, http.StatusUnauthorized, "root authentication required")
	return false
}

func (s *Server) resolveClientID(r *http.Request) (string, error) {
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
