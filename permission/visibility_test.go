package permission

import "testing"

func TestWhollyDeniedPreservesResourceExceptions(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rules Ruleset
		want  bool
	}{
		{"default", nil, false},
		{"all denied", Ruleset{{"*", "*", EffectDeny}}, true},
		{"empty resource denied", Ruleset{{Action: "edit", Effect: EffectDeny}}, true},
		{"empty resource overrides exception", Ruleset{{"edit", "docs/*", EffectAllow}, {Action: "edit", Effect: EffectDeny}}, true},
		{"empty resource allow", Ruleset{{"edit", "*", EffectDeny}, {Action: "edit", Effect: EffectAllow}}, false},
		{"empty resource ask", Ruleset{{"edit", "*", EffectDeny}, {Action: "edit", Effect: EffectAsk}}, false},
		{"partial deny", Ruleset{{"edit", "src/*", EffectDeny}}, false},
		{"allow exception", Ruleset{{"edit", "*", EffectDeny}, {"edit", "docs/*", EffectAllow}}, false},
		{"ask exception", Ruleset{{"edit", "*", EffectDeny}, {"edit", "docs/*", EffectAsk}}, false},
		{"later deny", Ruleset{{"edit", "docs/*", EffectAllow}, {"edit", "*", EffectDeny}}, true},
		{"narrower deny", Ruleset{{"edit", "*", EffectDeny}, {"edit", "docs/*", EffectDeny}}, true},
		{"other action", Ruleset{{"read", "*", EffectDeny}}, false},
		{"wildcard action", Ruleset{{"ed*", "*", EffectDeny}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := WhollyDenied("edit", tc.rules); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
