package config

import (
	"github.com/chaserensberger/wingman/plugins/orientation"
	"path/filepath"
	"testing"
)

func TestReferencesValidateAndNormalize(t *testing.T) {
	cfg := Default()
	cfg.References = []orientation.Reference{{Name: "manual", Path: "~/manual", Description: "Manuals"}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	normalized, err := cfg.Normalize(home)
	if err != nil || normalized.References[0].Path != filepath.Join(home, "manual") || cfg.References[0].Path != "~/manual" {
		t.Fatalf("normalized=%v err=%v", normalized.References, err)
	}
	for _, refs := range [][]orientation.Reference{
		{{Path: home}}, {{Name: "manual"}}, {{Name: "manual", Path: "relative"}},
		{{Name: "manual", Path: home}, {Name: "manual", Path: home}},
	} {
		cfg.References = refs
		if err := cfg.Validate(); err == nil {
			t.Fatalf("accepted invalid references: %#v", refs)
		}
	}
}
