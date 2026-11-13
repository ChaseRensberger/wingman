package provider

import (
	"testing"

	"github.com/chaserensberger/wingman/models"
)

func TestSourcesRemovalAndOwnedSnapshots(t *testing.T) {
	base, err := NewRegistry(map[string]ProviderConfig{"custom": {Models: map[string]models.ModelInfo{"one": {ID: "one", Provider: "custom", API: models.APIOpenAICompletions, BaseURL: "https://example.com"}}}})
	if err != nil {
		t.Fatal(err)
	}
	sources := base.Sources()
	source := sources["custom"]
	source.Meta.Name = "Renamed"
	sources["custom"] = source
	delete(sources["custom"].Models, "one")
	filtered, err := base.WithSources(sources)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := filtered.Catalog().Get("custom", "one"); ok {
		t.Fatal("removed model remains")
	}
	meta, err := filtered.Get("custom")
	if err != nil || meta.Name != "Renamed" {
		t.Fatalf("provider metadata = %+v, %v", meta, err)
	}
	for _, meta := range filtered.Catalog().ListProviders() {
		if meta.ID == "custom" && meta.Name != "Renamed" {
			t.Fatalf("catalog metadata disagrees with provider: %+v", meta)
		}
	}
	ref := models.ModelRef{Provider: "custom", ID: "one", API: models.APIOpenAICompletions, BaseURL: "https://example.com"}
	if _, err := resolveModelInfo(filtered.Catalog(), ref); err == nil {
		t.Fatal("explicit route bypassed removed model")
	}
	if _, ok := base.Catalog().Get("custom", "one"); !ok {
		t.Fatal("base mutated")
	}
	delete(sources, "custom")
	removed, err := base.WithSources(sources)
	if err != nil {
		t.Fatal(err)
	}
	if removed.IsValid("custom") {
		t.Fatal("removed provider remains")
	}
	ref.ID = "unknown"
	if _, err := resolveModelInfo(removed.Catalog(), ref); err == nil {
		t.Fatal("explicit route bypassed removed provider")
	}
	if !filtered.IsValid("custom") {
		t.Fatal("previous snapshot mutated")
	}
}
