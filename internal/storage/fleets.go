package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

func (s *JSONStore) fleetPath(id string) string {
	return filepath.Join(s.basePath, "fleets", id+".json")
}

func (s *JSONStore) CreateFleet(fleet *Fleet) error {
	if fleet.ID == "" {
		fleet.ID = NewID()
	}
	now := Now()
	fleet.CreatedAt = now
	fleet.UpdatedAt = now
	fleet.Status = FleetStatusStopped

	path := s.fleetPath(fleet.ID)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("fleet already exists: %s", fleet.ID)
	}

	return s.writeJSON(path, fleet)
}

func (s *JSONStore) GetFleet(id string) (*Fleet, error) {
	var fleet Fleet
	if err := s.readJSON(s.fleetPath(id), &fleet); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("fleet not found: %s", id)
		}
		return nil, err
	}
	return &fleet, nil
}

func (s *JSONStore) ListFleets() ([]*Fleet, error) {
	files, err := s.listDir(filepath.Join(s.basePath, "fleets"))
	if err != nil {
		return nil, err
	}

	fleets := make([]*Fleet, 0, len(files))
	for _, file := range files {
		var fleet Fleet
		if err := s.readJSON(file, &fleet); err != nil {
			continue
		}
		fleets = append(fleets, &fleet)
	}
	return fleets, nil
}

func (s *JSONStore) UpdateFleet(fleet *Fleet) error {
	path := s.fleetPath(fleet.ID)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("fleet not found: %s", fleet.ID)
	}

	fleet.UpdatedAt = Now()
	return s.writeJSON(path, fleet)
}

func (s *JSONStore) DeleteFleet(id string) error {
	path := s.fleetPath(id)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("fleet not found: %s", id)
	}
	return s.deleteJSON(path)
}
