package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "wingman.json")

	configJSON := `{
		"model": "anthropic/claude-opus-4",
		"providers": {
			"anthropic": {
				"model": "claude-opus-4",
				"max_tokens": 4096
			}
		},
		"tools": {
			"bash": true,
			"write": false
		}
	}`

	if err := os.WriteFile(configPath, []byte(configJSON), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFile(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Model != "anthropic/claude-opus-4" {
		t.Errorf("expected model 'anthropic/claude-opus-4', got %q", cfg.Model)
	}

	if cfg.Providers["anthropic"].MaxTokens != 4096 {
		t.Errorf("expected max_tokens 4096, got %d", cfg.Providers["anthropic"].MaxTokens)
	}

	if !cfg.IsToolEnabled("bash") {
		t.Error("bash should be enabled")
	}

	if cfg.IsToolEnabled("write") {
		t.Error("write should be disabled")
	}
}

func TestIsToolEnabled(t *testing.T) {
	cfg := &Config{
		Tools: map[string]bool{
			"bash":  true,
			"write": false,
		},
	}

	if !cfg.IsToolEnabled("bash") {
		t.Error("bash should be enabled")
	}

	if cfg.IsToolEnabled("write") {
		t.Error("write should be disabled")
	}

	if !cfg.IsToolEnabled("unknown") {
		t.Error("unknown tools should be enabled by default")
	}
}

func TestGetProviderConfig(t *testing.T) {
	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"anthropic": {
				Model:     "claude-sonnet-4",
				MaxTokens: 8192,
			},
		},
	}

	p := cfg.GetProviderConfig("anthropic")
	if p.Model != "claude-sonnet-4" {
		t.Errorf("expected 'claude-sonnet-4', got %q", p.Model)
	}

	p = cfg.GetProviderConfig("nonexistent")
	if p.Model != "" {
		t.Error("nonexistent provider should return empty config")
	}
}

func TestSave(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "new", "wingman.json")

	cfg := &Config{
		Model: "test-model",
		Tools: map[string]bool{
			"bash": true,
		},
	}

	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	loaded, err := LoadFile(configPath)
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}

	if loaded.Model != "test-model" {
		t.Errorf("expected 'test-model', got %q", loaded.Model)
	}
}
