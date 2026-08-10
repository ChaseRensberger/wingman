package main

import (
	"testing"

	daemonconfig "github.com/chaserensberger/wingman/internal/config"
)

func TestClientsCommandHierarchy(t *testing.T) {
	cmd := newCommand(daemonconfig.Config{})
	clients := cmd.Command("clients")
	if clients == nil {
		t.Fatal("clients command is missing")
	}
	for _, name := range []string{"create"} {
		if clients.Command(name) == nil {
			t.Errorf("clients %s command is missing", name)
		}
	}
}
