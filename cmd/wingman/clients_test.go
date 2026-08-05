package main

import (
	"context"
	"strings"
	"testing"

	daemonconfig "github.com/chaserensberger/wingman/internal/config"
)

func TestClientsCommandHierarchy(t *testing.T) {
	cmd := newCommand(daemonconfig.Config{})
	clients := cmd.Command("clients")
	if clients == nil {
		t.Fatal("clients command is missing")
	}
	for _, name := range []string{"create", "rotate"} {
		if clients.Command(name) == nil {
			t.Errorf("clients %s command is missing", name)
		}
	}
}

func TestClientRotateRequiresExactlyOneID(t *testing.T) {
	for _, args := range [][]string{{"wingman", "clients", "rotate"}, {"wingman", "clients", "rotate", "one", "two"}} {
		cmd := newCommand(daemonconfig.Config{})
		if err := cmd.Run(context.Background(), args); err == nil || !strings.Contains(err.Error(), "exactly one client ID") {
			t.Errorf("Run(%v) error = %v", args, err)
		}
	}
}
