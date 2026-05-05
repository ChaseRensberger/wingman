package hookid

import (
	"reflect"
	"testing"

	"github.com/chaserensberger/wingman/agent/loop"
)

func TestRegistryMatchesLoopHook(t *testing.T) {
	expected := make(map[string]struct{})

	hooksType := reflect.TypeOf(loop.Hooks{})
	for i := 0; i < hooksType.NumField(); i++ {
		field := hooksType.Field(i)
		if field.IsExported() {
			expected["Hooks."+field.Name] = struct{}{}
		}
	}

	sinkType := reflect.TypeOf((*loop.Sink)(nil)).Elem()
	for i := 0; i < sinkType.NumMethod(); i++ {
		method := sinkType.Method(i)
		expected["Sink."+method.Name] = struct{}{}
	}

	all := All()
	seen := make(map[string]struct{})
	seenIDs := make(map[ID]struct{})

	for _, h := range all {
		if h.ID == "" {
			t.Errorf("registry entry has empty ID")
		}
		if h.GoSymbol == "" {
			t.Errorf("registry entry for ID %q has empty GoSymbol", h.ID)
		}
		if h.Description == "" {
			t.Errorf("registry entry for ID %q has empty Description", h.ID)
		}

		if _, ok := seenIDs[h.ID]; ok {
			t.Errorf("duplicate registry ID: %q", h.ID)
		}
		seenIDs[h.ID] = struct{}{}

		if _, ok := expected[h.GoSymbol]; !ok {
			t.Errorf("registry entry %q has unexpected GoSymbol %q", h.ID, h.GoSymbol)
		}
		seen[h.GoSymbol] = struct{}{}
	}

	for sym := range expected {
		if _, ok := seen[sym]; !ok {
			t.Errorf("missing registry entry for GoSymbol %q", sym)
		}
	}
}

func TestLookup(t *testing.T) {
	got, ok := Lookup(ToolBefore)
	if !ok {
		t.Fatalf("Lookup(%q) returned ok=false; want true", ToolBefore)
	}
	if got.ID != ToolBefore {
		t.Errorf("Lookup(%q) returned ID %q; want %q", ToolBefore, got.ID, ToolBefore)
	}

	_, ok = Lookup("unknown.hook")
	if ok {
		t.Errorf("Lookup(\"unknown.hook\") returned ok=true; want false")
	}
}

func TestIDs(t *testing.T) {
	all := All()
	ids := IDs()

	if len(ids) != len(all) {
		t.Fatalf("IDs() returned %d entries, All() returned %d; want equal", len(ids), len(all))
	}

	for i, id := range ids {
		if id != all[i].ID {
			t.Errorf("IDs()[%d] = %q, All()[%d].ID = %q; want equal", i, id, i, all[i].ID)
		}
	}
}

func TestAllReturnsCopy(t *testing.T) {
	original := All()
	if len(original) == 0 {
		t.Fatal("All() returned empty slice; want non-empty")
	}

	original[0].ID = "mutated.id"

	after := All()
	if after[0].ID == "mutated.id" {
		t.Error("mutating slice returned by All() affected subsequent All() call; want independent copy")
	}
}
