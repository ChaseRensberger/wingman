package permission

import (
	"encoding/json"
	"testing"
)

func TestEvaluateLastMatchingRuleWins(t *testing.T) {
	rules := Ruleset{
		{Action: "bash", Resource: "*", Effect: EffectAsk},
		{Action: "bash", Resource: "git *", Effect: EffectAllow},
		{Action: "bash", Resource: "git push *", Effect: EffectDeny},
	}

	if got := Evaluate("bash", "git status --short", rules, EffectAsk).Effect; got != EffectAllow {
		t.Fatalf("git status effect = %q, want %q", got, EffectAllow)
	}
	if got := Evaluate("bash", "git push origin main", rules, EffectAsk).Effect; got != EffectDeny {
		t.Fatalf("git push effect = %q, want %q", got, EffectDeny)
	}
}

func TestEvaluateDefaultWhenNoRuleMatches(t *testing.T) {
	if got := Evaluate("read", "README.md", nil, EffectAllow).Effect; got != EffectAllow {
		t.Fatalf("default effect = %q, want %q", got, EffectAllow)
	}
}

func TestEvaluateWildcardMatchesAcrossSlashes(t *testing.T) {
	rules := Ruleset{
		{Action: "read", Resource: "*.env", Effect: EffectAsk},
		{Action: "read", Resource: "docs/**/*.md", Effect: EffectAllow},
	}
	if got := Evaluate("read", "packages/app/.env", rules, EffectDeny).Effect; got != EffectAsk {
		t.Fatalf("nested env effect = %q, want %q", got, EffectAsk)
	}
	if got := Evaluate("read", "docs/reference/config.md", rules, EffectDeny).Effect; got != EffectAllow {
		t.Fatalf("nested docs effect = %q, want %q", got, EffectAllow)
	}
}

func TestUnmarshalObjectSyntaxPreservesOrder(t *testing.T) {
	var rules Ruleset
	data := []byte(`{
		"bash": {
			"*": "ask",
			"git *": "allow",
			"git push *": "deny"
		},
		"edit": "ask"
	}`)
	if err := json.Unmarshal(data, &rules); err != nil {
		t.Fatal(err)
	}
	want := Ruleset{
		{Action: "bash", Resource: "*", Effect: EffectAsk},
		{Action: "bash", Resource: "git *", Effect: EffectAllow},
		{Action: "bash", Resource: "git push *", Effect: EffectDeny},
		{Action: "edit", Resource: "*", Effect: EffectAsk},
	}
	if len(rules) != len(want) {
		t.Fatalf("len = %d, want %d", len(rules), len(want))
	}
	for i := range want {
		if rules[i] != want[i] {
			t.Fatalf("rule %d = %#v, want %#v", i, rules[i], want[i])
		}
	}
}

func TestUnmarshalStringSyntax(t *testing.T) {
	var rules Ruleset
	if err := json.Unmarshal([]byte(`"deny"`), &rules); err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0] != (Rule{Action: "*", Resource: "*", Effect: EffectDeny}) {
		t.Fatalf("rules = %#v", rules)
	}
}

func TestUnmarshalArraySyntaxValidatesEffects(t *testing.T) {
	var rules Ruleset
	err := json.Unmarshal([]byte(`[{"action":"bash","resource":"*","effect":"alow"}]`), &rules)
	if err == nil {
		t.Fatal("expected invalid effect error")
	}
}
