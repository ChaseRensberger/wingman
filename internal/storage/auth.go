package storage

import (
	"os"
	"path/filepath"
)

func (s *JSONStore) authPath() string {
	return filepath.Join(s.basePath, "auth.json")
}

func (s *JSONStore) GetAuth() (*Auth, error) {
	var auth Auth
	if err := s.readJSON(s.authPath(), &auth); err != nil {
		if os.IsNotExist(err) {
			return &Auth{Providers: make(map[string]AuthCredential)}, nil
		}
		return nil, err
	}
	return &auth, nil
}

func (s *JSONStore) SetAuth(auth *Auth) error {
	auth.UpdatedAt = Now()
	return s.writeJSON(s.authPath(), auth)
}
