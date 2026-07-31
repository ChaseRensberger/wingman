package tool

import (
	"context"
	"errors"
	"testing"
)

func TestComposeRejectsInvalidAndDuplicateTools(t *testing.T) {
	valid := func(name string) Tool {
		return NewFuncTool(name, name, Definition{Name: name, InputSchema: InputSchema{Type: "object"}}, func(context.Context, Invocation) (Result, error) {
			return Result{}, nil
		})
	}
	var nilTool *FuncTool
	for _, tc := range []struct {
		name  string
		tools []Tool
		want  error
	}{
		{name: "nil", tools: []Tool{nilTool}, want: ErrInvalidTool},
		{name: "empty name", tools: []Tool{valid("")}, want: ErrInvalidTool},
		{name: "mismatch", tools: []Tool{NewFuncTool("runtime", "", Definition{Name: "declared"}, nil)}, want: ErrInvalidTool},
		{name: "duplicate", tools: []Tool{valid("same"), valid("same")}, want: ErrDuplicateTool},
		{name: "invalid input schema", tools: []Tool{NewFuncTool("bad_input", "", Definition{Name: "bad_input", RawInputSchema: map[string]any{"type": "invalid"}}, nil)}, want: ErrInvalidTool},
		{name: "invalid output schema", tools: []Tool{NewFuncTool("bad_output", "", Definition{Name: "bad_output", OutputSchema: map[string]any{"type": "invalid"}}, nil)}, want: ErrInvalidTool},
		{name: "missing permission action", tools: []Tool{NewFuncTool("bad_permission", "", Definition{Name: "bad_permission", Permission: &PermissionTarget{}}, nil)}, want: ErrInvalidTool},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Compose(tc.tools)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Compose() error = %v, want errors.Is(_, %v)", err, tc.want)
			}
		})
	}
}

func TestRegistryCatalogIsOrderedByName(t *testing.T) {
	makeTool := func(name string) Tool {
		return NewFuncTool(name, name, Definition{Name: name}, func(context.Context, Invocation) (Result, error) { return Result{}, nil })
	}
	registry, err := Compose([]Tool{makeTool("zebra"), makeTool("alpha")})
	if err != nil {
		t.Fatal(err)
	}
	tools := registry.List()
	defs := registry.Definitions()
	if tools[0].Name() != "alpha" || tools[1].Name() != "zebra" || defs[0].Name != "alpha" || defs[1].Name != "zebra" {
		t.Fatalf("catalog = %#v, definitions = %#v", tools, defs)
	}
}

func TestDefinitionTraitsRespectOptionalInterfaces(t *testing.T) {
	declared := NewFuncTool("declared", "", Definition{
		Name:            "declared",
		Sequential:      true,
		DirectoryScoped: true,
		Permission:      &PermissionTarget{Action: "read", ResourceFields: []string{"path", "paths"}},
		InputSchema:     InputSchema{Type: "object"},
	}, func(context.Context, Invocation) (Result, error) { return Result{}, nil })
	if !IsSequential(declared) || !IsDirectoryScoped(declared) {
		t.Fatal("declared execution traits were not applied")
	}
	check, declaredPermission, err := PermissionFor(declared, Invocation{Input: map[string]any{"path": "one", "paths": []string{"two", "three"}}})
	if err != nil || !declaredPermission || check.Action != "read" || len(check.Resources) != 3 {
		t.Fatalf("PermissionFor() = %#v, %v, %v", check, declaredPermission, err)
	}
	_, _, err = PermissionFor(declared, Invocation{Input: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	check, _, _ = PermissionFor(declared, Invocation{Input: map[string]any{}})
	if len(check.Resources) != 1 || check.Resources[0] != "*" {
		t.Fatalf("fallback resources = %#v", check.Resources)
	}

	override := traitOverrideTool{Tool: declared}
	if IsSequential(override) || !IsDirectoryScoped(override) {
		t.Fatal("optional execution traits did not take precedence")
	}
	check, declaredPermission, err = PermissionFor(override, Invocation{})
	if err != nil || !declaredPermission || check.Action != "override" || len(check.Resources) != 1 || check.Resources[0] != "resource" {
		t.Fatalf("override PermissionFor() = %#v, %v, %v", check, declaredPermission, err)
	}
}

type traitOverrideTool struct{ Tool }

func (traitOverrideTool) Sequential() bool { return false }
func (traitOverrideTool) DirectoryScoped() {}
func (traitOverrideTool) Permission(Invocation) (PermissionCheck, error) {
	return PermissionCheck{Action: "override", Resources: []string{"resource"}}, nil
}
