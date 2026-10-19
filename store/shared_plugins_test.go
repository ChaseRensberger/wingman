package store

import (
	"reflect"
	"testing"
)

func TestSharedPluginsMigrationPreservesAgents(t *testing.T) {
	db := testMigrationDB(t)
	if _, err := db.Exec(migrationsTable); err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations[:3] {
		if err := applyMigration(db, migration); err != nil {
			t.Fatal(err)
		}
	}
	data := &SQLiteStore{db: db}
	agent := &Agent{Name: "Custom", Instructions: "Keep these instructions", Tools: []string{"read"}}
	if err := data.CreateAgent(agent); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE agents SET plugins_json = '["wingcode"]' WHERE id = ?`, agent.ID); err != nil {
		t.Fatal(err)
	}
	if err := runMigrations(db); err != nil {
		t.Fatal(err)
	}
	if schemaHasColumn(t, db, "agents", "plugins_json") {
		t.Fatal("agent plugin selection remains in the schema")
	}
	got, err := data.GetAgent(agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, agent) {
		t.Fatalf("agent changed during migration: got=%+v want=%+v", got, agent)
	}
	if err := runMigrations(db); err != nil {
		t.Fatal(err)
	}
}
