package store

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestAgentPluginsPersistAndDefaultsAreExplicit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agents.db")
	data, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = data.Close() }()
	agents, err := data.ListAgents()
	if err != nil {
		t.Fatal(err)
	}
	for _, agent := range agents {
		if got, want := slices.Contains(agent.Plugins, "wingcode"), agent.Name == "Build" || agent.Name == "Plan"; got != want {
			t.Fatalf("default plugins for %s: %v", agent.Name, agent.Plugins)
		}
	}
	a := &Agent{Name: "Renamed coder", Plugins: []string{"wingcode"}}
	if err := data.CreateAgent(a); err != nil {
		t.Fatal(err)
	}
	if err := data.Close(); err != nil {
		t.Fatal(err)
	}
	data, err = NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := data.GetAgent(a.ID)
	if err != nil || !slices.Equal(got.Plugins, a.Plugins) {
		t.Fatalf("reopened=%#v err=%v", got, err)
	}
	got.Plugins = []string{}
	if err := data.UpdateAgent(got); err != nil {
		t.Fatal(err)
	}
	got, err = data.GetAgent(a.ID)
	if err != nil || len(got.Plugins) != 0 {
		t.Fatalf("cleared=%#v err=%v", got, err)
	}
}
