package server

import (
	"fmt"
	"net/http"
)

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
