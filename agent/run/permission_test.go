package run

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/permission"
	"github.com/chaserensberger/wingman/tool"
)

func TestCheckPermissionDenyReturnsStructuredMetadata(t *testing.T) {
	r := &runner{cfg: Config{Permissions: permission.Ruleset{{Action: "bash", Resource: "*", Effect: permission.EffectDeny}}}}
	decision := r.checkPermission(ToolCall{ID: "call_1", Name: "bash", Args: map[string]any{"command": "pwd"}})
	if decision.action != "bash" || decision.denyResource != "pwd" || len(decision.askResources) != 0 {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestCheckPermissionCollectsAskResources(t *testing.T) {
	r := &runner{cfg: Config{WorkDir: t.TempDir(), Permissions: permission.Ruleset{{Action: "edit", Resource: "*", Effect: permission.EffectAsk}}}}
	decision := r.checkPermission(ToolCall{Name: "apply_patch", Args: map[string]any{"patchText": "*** Begin Patch\n*** Add File: a.go\n+x\n*** Add File: b.go\n+y\n*** End Patch"}, Tool: tool.NewApplyPatchTool()})
	if decision.action != "edit" || len(decision.askResources) != 2 || decision.askResources[0] != "a.go" || decision.askResources[1] != "b.go" {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestPermissionTargetMapsMutatingToolsToEdit(t *testing.T) {
	action, resources, err := permissionTarget(ToolCall{Name: "write", Args: map[string]any{"filePath": "docs/index.md"}}, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if action != "edit" || len(resources) != 1 || resources[0] != "docs/index.md" {
		t.Fatalf("target = %s %#v", action, resources)
	}
}

func TestPermissionTargetUsesApplyPatchResources(t *testing.T) {
	workDir := t.TempDir()
	action, resources, err := permissionTarget(ToolCall{
		Name: "apply_patch",
		Args: map[string]any{"patchText": "*** Begin Patch\n*** Add File: docs/a.md\n+hello\n*** Update File: src/b.go\n*** Move to: src/c.go\n@@\n-old\n+new\n*** End Patch"},
		Tool: tool.NewApplyPatchTool(),
	}, workDir)
	if err != nil {
		t.Fatal(err)
	}
	if action != "edit" {
		t.Fatalf("action = %q, want edit", action)
	}
	want := []string{"docs/a.md", "src/b.go", "src/c.go"}
	if len(resources) != len(want) {
		t.Fatalf("resources = %#v, want %#v", resources, want)
	}
	for i := range want {
		if resources[i] != want[i] {
			t.Fatalf("resources = %#v, want %#v", resources, want)
		}
	}
}

func TestCheckPermissionDenyApplyPatchByPath(t *testing.T) {
	workDir := t.TempDir()
	r := &runner{cfg: Config{
		WorkDir: workDir,
		Permissions: permission.Ruleset{
			{Action: "edit", Resource: "*", Effect: permission.EffectAllow},
			{Action: "edit", Resource: "src/*", Effect: permission.EffectDeny},
		},
	}}
	decision := r.checkPermission(ToolCall{
		ID:   "call_1",
		Name: "apply_patch",
		Args: map[string]any{"patchText": "*** Begin Patch\n*** Add File: src/main.go\n+package main\n*** End Patch"},
		Tool: tool.NewApplyPatchTool(),
	})
	if decision.action != "edit" || decision.denyResource != "src/main.go" {
		t.Fatalf("decision = %#v", decision)
	}
}

type permissionPrompterFunc func(context.Context, PermissionRequestInfo) (PermissionResponse, error)

func (f permissionPrompterFunc) Request(ctx context.Context, info PermissionRequestInfo) (PermissionResponse, error) {
	return f(ctx, info)
}

func TestPermissionPromptAllowsAfterReply(t *testing.T) {
	for _, response := range []PermissionResponse{PermissionResponseOnce, PermissionResponseAlways} {
		t.Run(string(response), func(t *testing.T) {
			var order []string
			lifecycle := toolUseLifecycleFuncs{
				propose:   func(context.Context, ToolUseProposeInfo) (string, error) { return "", nil },
				authorize: func(context.Context, ToolUseAuthorizeInfo) error { order = append(order, "authorize"); return nil },
				start:     func(context.Context, ToolUseStartInfo) error { order = append(order, "start"); return nil },
				finish:    func(context.Context, ToolUseFinishInfo) error { order = append(order, "finish"); return nil },
			}
			r := &runner{cfg: Config{
				Permissions: permission.Ruleset{{Action: "test", Resource: "*", Effect: permission.EffectAsk}}, ToolUseLifecycle: lifecycle,
				PermissionPrompter: permissionPrompterFunc(func(_ context.Context, info PermissionRequestInfo) (PermissionResponse, error) {
					order = append(order, "prompt")
					if info.Step != 2 || info.Ordinal != 3 || info.ToolUseID != "tlu_1" || info.CallID != "call_1" || info.MessageID != "message_1" || info.PartID != "part_1" || info.ModelCallID != "model_call_1" || info.Action != "test" || len(info.Resources) != 1 || info.Resources[0] != "*" {
						t.Fatalf("request = %#v", info)
					}
					return response, nil
				}),
			}, eventCh: make(chan Event, 4)}
			call := permissionTestCall(func(context.Context, tool.Invocation) (tool.Result, error) {
				order = append(order, "execute")
				return tool.Result{}, nil
			})
			call.Step, call.Ordinal = 2, 3
			if _, err := r.executeOne(context.Background(), call); err != nil {
				t.Fatal(err)
			}
			want := []string{"prompt", "authorize", "start", "execute", "finish"}
			if len(order) != len(want) {
				t.Fatalf("order = %#v", order)
			}
			for i := range want {
				if order[i] != want[i] {
					t.Fatalf("order = %#v", order)
				}
			}
		})
	}
}

func TestPermissionPromptDeclinesWithoutExecution(t *testing.T) {
	for _, tc := range []struct {
		name, errorType string
		prompter        PermissionPrompter
	}{
		{name: "nil", errorType: "permission_unavailable"},
		{name: "reject", errorType: "permission_denied", prompter: permissionPrompterFunc(func(context.Context, PermissionRequestInfo) (PermissionResponse, error) {
			return PermissionResponseReject, nil
		})},
		{name: "invalid", errorType: "permission_invalid_response", prompter: permissionPrompterFunc(func(context.Context, PermissionRequestInfo) (PermissionResponse, error) {
			return "invalid", nil
		})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var executed, authorized, started int
			var finished ToolUseFinishInfo
			r := permissionTestRunner(tc.prompter, toolUseLifecycleFuncs{
				propose:   func(context.Context, ToolUseProposeInfo) (string, error) { return "", nil },
				authorize: func(context.Context, ToolUseAuthorizeInfo) error { authorized++; return nil },
				start:     func(context.Context, ToolUseStartInfo) error { started++; return nil },
				finish:    func(_ context.Context, info ToolUseFinishInfo) error { finished = info; return nil },
			})
			res, err := r.executeOne(context.Background(), permissionTestCall(func(context.Context, tool.Invocation) (tool.Result, error) { executed++; return tool.Result{}, nil }))
			if err != nil {
				t.Fatal(err)
			}
			if executed != 0 || authorized != 0 || started != 0 || finished.Status != ToolUseStatusDeclined || finished.ErrorType != tc.errorType {
				t.Fatalf("executed=%d authorized=%d started=%d finished=%#v", executed, authorized, started, finished)
			}
			if tc.name == "reject" && res.Metadata["error_type"] != "permission_denied" {
				t.Fatalf("reject metadata = %#v", res.Metadata)
			}
		})
	}
}

func TestPermissionPromptInterruptsOnContextEnd(t *testing.T) {
	for _, tc := range []struct {
		name, errorType string
		ctx             context.Context
	}{
		{name: "cancel", errorType: "permission_interrupted", ctx: canceledContext()},
		{name: "timeout", errorType: "permission_timeout", ctx: timedOutContext()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var executed int
			var finished ToolUseFinishInfo
			r := permissionTestRunner(permissionPrompterFunc(func(ctx context.Context, _ PermissionRequestInfo) (PermissionResponse, error) {
				<-ctx.Done()
				return "", ctx.Err()
			}), toolUseLifecycleFuncs{
				propose: func(context.Context, ToolUseProposeInfo) (string, error) { return "", nil }, authorize: func(context.Context, ToolUseAuthorizeInfo) error { t.Fatal("authorized"); return nil }, start: func(context.Context, ToolUseStartInfo) error { t.Fatal("started"); return nil }, finish: func(_ context.Context, info ToolUseFinishInfo) error { finished = info; return nil },
			})
			if _, err := r.executeOne(tc.ctx, permissionTestCall(func(context.Context, tool.Invocation) (tool.Result, error) { executed++; return tool.Result{}, nil })); err == nil {
				t.Fatal("expected interruption error")
			}
			if executed != 0 || finished.Status != ToolUseStatusInterrupted || finished.ErrorType != tc.errorType {
				t.Fatalf("executed=%d finished=%#v", executed, finished)
			}
		})
	}
}

func TestPermissionPrompterFailureAndDeny(t *testing.T) {
	var executed, prompted int
	var failed ToolUseFinishInfo
	r := permissionTestRunner(permissionPrompterFunc(func(context.Context, PermissionRequestInfo) (PermissionResponse, error) {
		prompted++
		return "", errors.New("prompt failed")
	}), toolUseLifecycleFuncs{propose: func(context.Context, ToolUseProposeInfo) (string, error) { return "", nil }, authorize: func(context.Context, ToolUseAuthorizeInfo) error { t.Fatal("authorized"); return nil }, start: func(context.Context, ToolUseStartInfo) error { t.Fatal("started"); return nil }, finish: func(_ context.Context, info ToolUseFinishInfo) error { failed = info; return nil }})
	if _, err := r.executeOne(context.Background(), permissionTestCall(func(context.Context, tool.Invocation) (tool.Result, error) { executed++; return tool.Result{}, nil })); err == nil {
		t.Fatal("expected prompt error")
	}
	if executed != 0 || prompted != 1 || failed.Status != ToolUseStatusFailed || failed.ErrorType != "permission_prompt" {
		t.Fatalf("executed=%d prompted=%d finished=%#v", executed, prompted, failed)
	}

	r.cfg.Permissions[0].Effect = permission.EffectDeny
	if _, err := r.executeOne(context.Background(), permissionTestCall(func(context.Context, tool.Invocation) (tool.Result, error) { executed++; return tool.Result{}, nil })); err != nil {
		t.Fatal(err)
	}
	if prompted != 1 || executed != 0 {
		t.Fatalf("deny prompted=%d executed=%d", prompted, executed)
	}
}

func permissionTestRunner(prompter PermissionPrompter, lifecycle ToolUseLifecycle) *runner {
	return &runner{cfg: Config{Permissions: permission.Ruleset{{Action: "test", Resource: "*", Effect: permission.EffectAsk}}, PermissionPrompter: prompter, ToolUseLifecycle: lifecycle}, eventCh: make(chan Event, 4)}
}

func permissionTestCall(execute func(context.Context, tool.Invocation) (tool.Result, error)) ToolCall {
	return ToolCall{ID: "call_1", ToolUseID: "tlu_1", Name: "test", Args: map[string]any{"value": true}, MessageID: "message_1", PartID: "part_1", ModelCallID: "model_call_1", Tool: tool.NewFuncTool("test", "test", tool.Definition{Name: "test", InputSchema: tool.InputSchema{Type: "object"}}, execute)}
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
func timedOutContext() context.Context {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	return ctx
}
