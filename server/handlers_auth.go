package server

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
)

type loginRequest struct {
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var request loginRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || subtle.ConstantTimeCompare([]byte(request.Password), []byte(s.password)) != 1 {
		w.Header().Set("WWW-Authenticate", `Basic realm="wingman"`)
		s.writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	s.setConsoleSession(w, r)
	w.WriteHeader(http.StatusNoContent)
}
