package main

import (
	"path/filepath"
	"testing"
)

func TestConfigDirDefaultsToDotConfig(t *testing.T) {
	t.Setenv("SUDO_USER", "")
	t.Setenv("HOME", "/tmp/wingman-home")
	t.Setenv("XDG_CONFIG_HOME", "")

	dir, err := configDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("/tmp/wingman-home", ".config"); dir != want {
		t.Errorf("configDir() = %q, want %q", dir, want)
	}
}

func TestConfigDirUsesXDGConfigHome(t *testing.T) {
	t.Setenv("SUDO_USER", "")
	t.Setenv("XDG_CONFIG_HOME", "/tmp/wingman-config")

	dir, err := configDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := "/tmp/wingman-config"; dir != want {
		t.Errorf("configDir() = %q, want %q", dir, want)
	}
}
