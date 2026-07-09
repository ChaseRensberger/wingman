// Package permission evaluates Wingman tool permission rules.
package permission

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Effect is the decision a rule makes for an action/resource pair.
type Effect string

const (
	EffectAllow Effect = "allow"
	EffectAsk   Effect = "ask"
	EffectDeny  Effect = "deny"
)

// Rule controls one action/resource pattern.
type Rule struct {
	Action   string `json:"action"`
	Resource string `json:"resource"`
	Effect   Effect `json:"effect"`
}

// Ruleset is an ordered list of permission rules. Later matching rules win.
type Ruleset []Rule

// Decision is the evaluated effect and the rule that produced it, if any.
type Decision struct {
	Effect Effect
	Rule   *Rule
}

// Evaluate returns the last matching rule for action/resource. If no rule
// matches, defaultEffect is returned.
func Evaluate(action, resource string, rules Ruleset, defaultEffect Effect) Decision {
	decision := Decision{Effect: defaultEffect}
	for i := range rules {
		rule := rules[i]
		if !match(rule.Action, action) || !match(rule.Resource, resource) {
			continue
		}
		decision.Effect = rule.Effect
		decision.Rule = &rule
	}
	return decision
}

// Merge concatenates rulesets, preserving order.
func Merge(sets ...Ruleset) Ruleset {
	var total int
	for _, set := range sets {
		total += len(set)
	}
	out := make(Ruleset, 0, total)
	for _, set := range sets {
		out = append(out, set...)
	}
	return out
}

// UnmarshalJSON accepts both normalized rule arrays and ergonomic config forms:
// "allow", {"bash":"ask"}, or {"bash":{"*":"ask","git *":"allow"}}.
func (r *Ruleset) UnmarshalJSON(data []byte) error {
	var rules []Rule
	if err := json.Unmarshal(data, &rules); err == nil {
		for i := range rules {
			if err := normalizeRule(&rules[i]); err != nil {
				return err
			}
		}
		*r = rules
		return nil
	}

	var effect Effect
	if err := json.Unmarshal(data, &effect); err == nil {
		if err := validateEffect(effect); err != nil {
			return err
		}
		*r = Ruleset{{Action: "*", Resource: "*", Effect: effect}}
		return nil
	}

	rules, err := parseObjectRules(data)
	if err != nil {
		return err
	}
	*r = rules
	return nil
}

func parseObjectRules(data []byte) (Ruleset, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("permissions must be an effect, object, or rule array")
	}
	var rules Ruleset
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		action, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("permission action must be a string")
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		var effect Effect
		if err := json.Unmarshal(raw, &effect); err == nil {
			if err := validateEffect(effect); err != nil {
				return nil, err
			}
			rule := Rule{Action: action, Resource: "*", Effect: effect}
			if err := normalizeRule(&rule); err != nil {
				return nil, err
			}
			rules = append(rules, rule)
			continue
		}
		resourceRules, err := parseResourceRules(action, raw)
		if err != nil {
			return nil, err
		}
		rules = append(rules, resourceRules...)
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return rules, nil
}

func parseResourceRules(action string, data []byte) (Ruleset, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("permission %q: expected effect or resource map", action)
	}
	var rules Ruleset
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		resource, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("permission %q resource must be a string", action)
		}
		var effect Effect
		if err := dec.Decode(&effect); err != nil {
			return nil, fmt.Errorf("permission %q resource %q: %w", action, resource, err)
		}
		if err := validateEffect(effect); err != nil {
			return nil, err
		}
		rule := Rule{Action: action, Resource: ExpandHome(resource), Effect: effect}
		if err := normalizeRule(&rule); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return rules, nil
}

func normalizeRule(rule *Rule) error {
	if rule.Action == "" {
		rule.Action = "*"
	}
	if rule.Resource == "" {
		rule.Resource = "*"
	}
	rule.Resource = ExpandHome(rule.Resource)
	return validateEffect(rule.Effect)
}

func validateEffect(effect Effect) error {
	switch effect {
	case EffectAllow, EffectAsk, EffectDeny:
		return nil
	default:
		return fmt.Errorf("unknown permission effect %q", effect)
	}
}

func match(pattern, value string) bool {
	if pattern == "" {
		pattern = "*"
	}
	if pattern == "*" || pattern == value {
		return true
	}
	return wildcardMatch(pattern, value)
}

func wildcardMatch(pattern, value string) bool {
	pi, vi := 0, 0
	star, match := -1, 0
	for vi < len(value) {
		if pi < len(pattern) && (pattern[pi] == '?' || pattern[pi] == value[vi]) {
			pi++
			vi++
			continue
		}
		if pi < len(pattern) && pattern[pi] == '*' {
			star = pi
			match = vi
			pi++
			continue
		}
		if star != -1 {
			pi = star + 1
			match++
			vi = match
			continue
		}
		return false
	}
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern)
}

// ExpandHome expands leading ~ and $HOME in resource patterns when HOME is set.
func ExpandHome(pattern string) string {
	home := strings.TrimSpace(filepath.ToSlash(filepath.Clean(getenvHome())))
	if home == "." || home == "" {
		return pattern
	}
	slashHome := filepath.ToSlash(home)
	switch {
	case pattern == "~":
		return slashHome
	case strings.HasPrefix(pattern, "~/"):
		return slashHome + "/" + strings.TrimPrefix(pattern, "~/")
	case pattern == "$HOME":
		return slashHome
	case strings.HasPrefix(pattern, "$HOME/"):
		return slashHome + "/" + strings.TrimPrefix(pattern, "$HOME/")
	default:
		return pattern
	}
}

var getenvHome = func() string { return os.Getenv("HOME") }
