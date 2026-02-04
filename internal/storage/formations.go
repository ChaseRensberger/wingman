package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

func (s *JSONStore) formationPath(id string) string {
	return filepath.Join(s.basePath, "formations", id+".json")
}

func (s *JSONStore) CreateFormation(formation *Formation) error {
	if formation.ID == "" {
		formation.ID = NewID()
	}
	now := Now()
	formation.CreatedAt = now
	formation.UpdatedAt = now
	formation.Status = FormationStatusStopped

	path := s.formationPath(formation.ID)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("formation already exists: %s", formation.ID)
	}

	return s.writeJSON(path, formation)
}

func (s *JSONStore) GetFormation(id string) (*Formation, error) {
	var formation Formation
	if err := s.readJSON(s.formationPath(id), &formation); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("formation not found: %s", id)
		}
		return nil, err
	}
	return &formation, nil
}

func (s *JSONStore) ListFormations() ([]*Formation, error) {
	files, err := s.listDir(filepath.Join(s.basePath, "formations"))
	if err != nil {
		return nil, err
	}

	formations := make([]*Formation, 0, len(files))
	for _, file := range files {
		var formation Formation
		if err := s.readJSON(file, &formation); err != nil {
			continue
		}
		formations = append(formations, &formation)
	}
	return formations, nil
}

func (s *JSONStore) UpdateFormation(formation *Formation) error {
	path := s.formationPath(formation.ID)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("formation not found: %s", formation.ID)
	}

	formation.UpdatedAt = Now()
	return s.writeJSON(path, formation)
}

func (s *JSONStore) DeleteFormation(id string) error {
	path := s.formationPath(id)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("formation not found: %s", id)
	}
	return s.deleteJSON(path)
}
