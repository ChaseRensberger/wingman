package run

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/permission"
	"github.com/chaserensberger/wingman/tool"
)

func TestOutsideDirectoryPermissions(t *testing.T) {
	for _, test := range []struct {
		name, action string
		tool         tool.Tool
		input        func(string) map[string]any
		changes      bool
	}{
		{"read", "read", tool.NewReadTool(), func(path string) map[string]any { return map[string]any{"filePath": path} }, false},
		{"read directory", "read", tool.NewReadTool(), func(path string) map[string]any { return map[string]any{"filePath": filepath.Dir(path)} }, false},
		{"write", "edit", tool.NewWriteTool(), func(path string) map[string]any { return map[string]any{"filePath": path, "content": "after\n"} }, true},
		{"edit", "edit", tool.NewEditTool(), func(path string) map[string]any {
			return map[string]any{"filePath": path, "oldString": "before", "newString": "after"}
		}, true},
		{"patch", "edit", tool.NewApplyPatchTool(), func(path string) map[string]any {
			return map[string]any{"patchText": "*** Begin Patch\n*** Update File: " + path + "\n@@\n-before\n+after\n*** End Patch"}
		}, true},
		{"glob", "read", tool.NewGlobTool(), func(path string) map[string]any { return map[string]any{"path": filepath.Dir(path), "pattern": "*.md"} }, false},
		{"grep", "read", tool.NewGrepTool(), func(path string) map[string]any {
			return map[string]any{"path": filepath.Dir(path), "pattern": "before"}
		}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, mode := range []string{"unavailable", "reject", "once", "allow rule", "deny rule"} {
				t.Run(mode, func(t *testing.T) {
					root := t.TempDir()
					workDir, outside := filepath.Join(root, "work"), filepath.Join(root, "reference")
					for _, path := range []string{workDir, outside} {
						if err := os.Mkdir(path, 0755); err != nil {
							t.Fatal(err)
						}
					}
					path := filepath.Join(outside, "file.md")
					if err := os.WriteFile(path, []byte("before\n"), 0644); err != nil {
						t.Fatal(err)
					}
					outside, err := filepath.EvalSymlinks(outside)
					if err != nil {
						t.Fatal(err)
					}
					boundary := filepath.ToSlash(filepath.Join(outside, "*"))
					prompts := 0
					r := &runner{cfg: Config{WorkDir: workDir}, eventCh: make(chan Event, 16)}
					if mode != "unavailable" {
						r.cfg.PermissionPrompter = permissionPrompterFunc(func(_ context.Context, info PermissionRequestInfo) (PermissionResponse, error) {
							prompts++
							if info.Action != test.action || len(info.Resources) != 1 || info.Resources[0] != boundary {
								t.Fatalf("request = %#v; want %s %s", info, test.action, boundary)
							}
							if data, err := os.ReadFile(path); err != nil || string(data) != "before\n" {
								t.Fatalf("file changed before approval: %q, %v", data, err)
							}
							if mode == "reject" {
								return PermissionResponseReject, nil
							}
							return PermissionResponseOnce, nil
						})
					}
					if mode == "allow rule" || mode == "deny rule" {
						effect := permission.EffectAllow
						if mode == "deny rule" {
							effect = permission.EffectDeny
						}
						r.cfg.Permissions = permission.Ruleset{{Action: test.action, Resource: boundary, Effect: effect}}
					}
					call := ToolCall{ID: "call_outside", Name: test.tool.Name(), Tool: test.tool, Args: test.input(filepath.Join("..", "reference", "file.md"))}
					result, err := r.executeOne(context.Background(), call)
					blocked := mode == "unavailable" || mode == "reject" || mode == "deny rule"
					if err != nil || result.IsError != blocked {
						t.Fatalf("executeOne() = %#v, %v; blocked=%v", result, err, blocked)
					}
					wantPrompts := 0
					if mode == "once" || mode == "reject" {
						wantPrompts = 1
					}
					if prompts != wantPrompts {
						t.Fatalf("prompts = %d; want %d", prompts, wantPrompts)
					}
					want := "before\n"
					if !blocked && test.changes {
						want = "after\n"
					}
					if data, err := os.ReadFile(path); err != nil || string(data) != want {
						t.Fatalf("file = %q, %v; want %q", data, err, want)
					}
				})
			}
		})
	}
}

func TestInsideDirectoryStillAllowsWithoutRules(t *testing.T) {
	workDir := t.TempDir()
	path := filepath.Join(workDir, "file.md")
	if err := os.WriteFile(path, []byte("inside"), 0644); err != nil {
		t.Fatal(err)
	}
	r := &runner{cfg: Config{WorkDir: workDir}, eventCh: make(chan Event, 8)}
	result, err := r.executeOne(context.Background(), ToolCall{ID: "call_inside", Name: "read", Tool: tool.NewReadTool(), Args: map[string]any{"filePath": path}})
	if err != nil || result.IsError || !strings.Contains(result.Output, "inside") {
		t.Fatalf("result = %#v, %v", result, err)
	}
}

func TestOutsideLinkRequestsTargetDirectory(t *testing.T) {
	workDir := t.TempDir()
	outside, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workDir, "reference")); err != nil {
		t.Skip(err)
	}
	r := &runner{cfg: Config{WorkDir: workDir}}
	decision := r.checkPermission(ToolCall{Name: "write", Tool: tool.NewWriteTool(), Args: map[string]any{"filePath": "reference/new.md", "content": "new"}})
	want := filepath.ToSlash(filepath.Join(outside, "*"))
	if decision.err != nil || len(decision.askResources) != 1 || decision.askResources[0] != want {
		t.Fatalf("decision = %#v; want %s", decision, want)
	}
}

func TestPatchChecksEveryPathBeforeApprovalOrMutation(t *testing.T) {
	workDir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "denied.md")
	_, resource, err := tool.ResolvePath(workDir, outside)
	if err != nil {
		t.Fatal(err)
	}
	r := &runner{cfg: Config{
		WorkDir: workDir,
		Permissions: permission.Ruleset{
			{Action: "edit", Resource: "*", Effect: permission.EffectAsk},
			{Action: "edit", Resource: resource, Effect: permission.EffectDeny},
		},
		PermissionPrompter: permissionPrompterFunc(func(context.Context, PermissionRequestInfo) (PermissionResponse, error) {
			t.Fatal("deny must prevent any approval request")
			return PermissionResponseAlways, nil
		}),
	}, eventCh: make(chan Event, 8)}
	patch := "*** Begin Patch\n*** Add File: inside.md\n+inside\n*** Add File: " + outside + "\n+outside\n*** End Patch"
	result, err := r.executeOne(context.Background(), ToolCall{ID: "call_patch", Name: "apply_patch", Tool: tool.NewApplyPatchTool(), Args: map[string]any{"patchText": patch}})
	if err != nil || !result.IsError || result.Metadata["permission"].(map[string]any)["effect"] != "deny" {
		t.Fatalf("result = %#v, %v", result, err)
	}
	for _, path := range []string{outside, filepath.Join(workDir, "inside.md")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("file created before denial: %s, %v", path, err)
		}
	}
}

func TestOutsideSearchKeepsPatternDenials(t *testing.T) {
	for _, search := range []tool.Tool{tool.NewGlobTool(), tool.NewGrepTool()} {
		t.Run(search.Name(), func(t *testing.T) {
			r := &runner{cfg: Config{
				WorkDir:     t.TempDir(),
				Permissions: permission.Ruleset{{Action: search.Name(), Resource: "*", Effect: permission.EffectDeny}},
				PermissionPrompter: permissionPrompterFunc(func(context.Context, PermissionRequestInfo) (PermissionResponse, error) {
					t.Fatal("denied search must not ask for directory access")
					return PermissionResponseAlways, nil
				}),
			}, eventCh: make(chan Event, 8)}
			result, err := r.executeOne(context.Background(), ToolCall{ID: "call_search", Name: search.Name(), Tool: search, Args: map[string]any{"path": t.TempDir(), "pattern": "secret"}})
			if err != nil || !result.IsError || result.Metadata["permission"].(map[string]any)["action"] != search.Name() {
				t.Fatalf("result = %#v, %v", result, err)
			}
		})
	}
}

func TestPatchMoveRequiresBothOutsideDirectories(t *testing.T) {
	workDir := t.TempDir()
	source := filepath.Join(t.TempDir(), "source.md")
	destination := filepath.Join(t.TempDir(), "destination.md")
	if err := os.WriteFile(source, []byte("before\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, sourceResource, err := tool.ResolvePath(workDir, source)
	if err != nil {
		t.Fatal(err)
	}
	_, destinationResource, err := tool.ResolvePath(workDir, destination)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.ToSlash(filepath.Join(filepath.Dir(filepath.FromSlash(sourceResource)), "*")),
		filepath.ToSlash(filepath.Join(filepath.Dir(filepath.FromSlash(destinationResource)), "*")),
	}
	prompts := 0
	r := &runner{cfg: Config{
		WorkDir: workDir,
		PermissionPrompter: permissionPrompterFunc(func(_ context.Context, info PermissionRequestInfo) (PermissionResponse, error) {
			prompts++
			if info.Action != "edit" || len(info.Resources) != 2 || info.Resources[0] != want[0] || info.Resources[1] != want[1] {
				t.Fatalf("request = %#v; want both directories %v", info, want)
			}
			if data, err := os.ReadFile(source); err != nil || string(data) != "before\n" {
				t.Fatalf("source changed before approval: %q, %v", data, err)
			}
			if _, err := os.Stat(destination); !os.IsNotExist(err) {
				t.Fatalf("destination created before approval: %v", err)
			}
			return PermissionResponseOnce, nil
		}),
	}, eventCh: make(chan Event, 8)}
	patch := "*** Begin Patch\n*** Update File: " + source + "\n*** Move to: " + destination + "\n@@\n-before\n+after\n*** End Patch"
	result, err := r.executeOne(context.Background(), ToolCall{ID: "call_move", Name: "apply_patch", Tool: tool.NewApplyPatchTool(), Args: map[string]any{"patchText": patch}})
	if err != nil || result.IsError || prompts != 1 {
		t.Fatalf("result = %#v, %v; prompts=%d", result, err, prompts)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source still exists: %v", err)
	}
	if data, err := os.ReadFile(destination); err != nil || string(data) != "after\n" {
		t.Fatalf("destination = %q, %v", data, err)
	}
}

func TestPatchDeduplicatesOutsideDirectoryRequests(t *testing.T) {
	workDir, outside := t.TempDir(), t.TempDir()
	patch := "*** Begin Patch\n*** Add File: " + filepath.Join(outside, "one.md") + "\n+one\n*** Add File: " + filepath.Join(outside, "two.md") + "\n+two\n*** End Patch"
	r := &runner{cfg: Config{WorkDir: workDir}}
	decision := r.checkPermission(ToolCall{Name: "apply_patch", Tool: tool.NewApplyPatchTool(), Args: map[string]any{"patchText": patch}})
	if decision.err != nil || decision.action != "edit" || len(decision.askResources) != 1 {
		t.Fatalf("decision = %#v; want one directory approval", decision)
	}
}

func TestOutsideApprovalDoesNotChangeBashPolicy(t *testing.T) {
	r := &runner{cfg: Config{WorkDir: t.TempDir()}}
	decision := r.checkPermission(ToolCall{Name: "bash", Tool: tool.NewBashTool(), Args: map[string]any{"command": "cat /outside/file.md"}})
	if decision.err != nil || decision.denyResource != "" || len(decision.askResources) != 0 {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestSavedOutputReferenceIsNotAFilePermissionPath(t *testing.T) {
	const reference = "tool-output:output_123"
	r := &runner{cfg: Config{WorkDir: t.TempDir(), Permissions: permission.Ruleset{{Action: "read", Resource: "tool-output:*", Effect: permission.EffectDeny}}}}
	decision := r.checkPermission(ToolCall{Name: "read", Tool: tool.NewReadTool(), Args: map[string]any{"filePath": reference}})
	if decision.err != nil || decision.denyResource != reference {
		t.Fatalf("decision = %#v; want the saved-output reference", decision)
	}
}

func TestPatchChecksOutsideSymlinkEntriesBeforeMutation(t *testing.T) {
	for _, operation := range []string{"delete", "replace", "move"} {
		t.Run(operation, func(t *testing.T) {
			for _, mode := range []string{"unavailable", "deny", "reject", "once"} {
				t.Run(mode, func(t *testing.T) {
					for _, linkedParent := range []bool{false, true} {
						name := "absolute path"
						if linkedParent {
							name = "linked parent"
						}
						t.Run(name, func(t *testing.T) {
							workDir := t.TempDir()
							outside, err := filepath.EvalSymlinks(t.TempDir())
							if err != nil {
								t.Fatal(err)
							}
							target := filepath.Join(workDir, "allowed.md")
							if err := os.WriteFile(target, []byte("before\n"), 0644); err != nil {
								t.Fatal(err)
							}
							link := filepath.Join(outside, "link.md")
							if err := os.Symlink(target, link); err != nil {
								t.Skip(err)
							}
							path := link
							if linkedParent {
								if err := os.Symlink(outside, filepath.Join(workDir, "external")); err != nil {
									t.Skip(err)
								}
								path = filepath.Join("external", "link.md")
							}
							patch := "*** Begin Patch\n*** Delete File: " + path + "\n"
							switch operation {
							case "replace":
								patch += "*** Add File: " + path + "\n+after\n"
							case "move":
								patch = "*** Begin Patch\n*** Update File: " + path + "\n*** Move to: moved.md\n@@\n-before\n+after\n"
							}
							patch += "*** End Patch"
							boundary := filepath.ToSlash(filepath.Join(outside, "*"))
							prompts := 0
							r := &runner{cfg: Config{WorkDir: workDir}, eventCh: make(chan Event, 8)}
							if mode == "deny" {
								r.cfg.Permissions = permission.Ruleset{
									{Action: "edit", Resource: "*", Effect: permission.EffectAsk},
									{Action: "edit", Resource: boundary, Effect: permission.EffectDeny},
								}
							}
							if mode != "unavailable" {
								r.cfg.PermissionPrompter = permissionPrompterFunc(func(_ context.Context, info PermissionRequestInfo) (PermissionResponse, error) {
									prompts++
									if info.Action != "edit" || len(info.Resources) != 1 || info.Resources[0] != boundary {
										t.Fatalf("request = %#v; want the outside directory entry", info)
									}
									if mode == "reject" {
										return PermissionResponseReject, nil
									}
									return PermissionResponseOnce, nil
								})
							}
							result, err := r.executeOne(t.Context(), ToolCall{ID: "call_link", Name: "apply_patch", Tool: tool.NewApplyPatchTool(), Args: map[string]any{"patchText": patch}})
							blocked := mode != "once"
							if err != nil || result.IsError != blocked {
								t.Fatalf("result = %#v, %v; blocked=%v", result, err, blocked)
							}
							wantPrompts := 0
							if mode == "reject" || mode == "once" {
								wantPrompts = 1
							}
							if prompts != wantPrompts {
								t.Fatalf("prompts = %d; want %d", prompts, wantPrompts)
							}
							info, statErr := os.Lstat(link)
							if blocked {
								if statErr != nil || info.Mode()&os.ModeSymlink == 0 {
									t.Fatalf("outside link changed before approval: %v", statErr)
								}
								if data, err := os.ReadFile(target); err != nil || string(data) != "before\n" {
									t.Fatalf("target changed before approval: %q, %v", data, err)
								}
								if _, err := os.Stat(filepath.Join(workDir, "moved.md")); !os.IsNotExist(err) {
									t.Fatalf("move destination changed before approval: %v", err)
								}
							} else if operation == "replace" {
								if statErr != nil || !info.Mode().IsRegular() {
									t.Fatalf("approved replacement failed: %v", statErr)
								}
								if data, err := os.ReadFile(link); err != nil || string(data) != "after\n" {
									t.Fatalf("replacement = %q, %v", data, err)
								}
							} else if !os.IsNotExist(statErr) {
								t.Fatalf("approved removal failed: %v", statErr)
							}
						})
					}
				})
			}
		})
	}
}

func TestDeclaredFilesystemTargetsStillResolvePaths(t *testing.T) {
	for _, action := range []string{"read", "edit"} {
		t.Run(action, func(t *testing.T) {
			workDir := t.TempDir()
			outside, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			declared := tool.NewFuncTool("declared", "", tool.Definition{
				Name: "declared", Permission: &tool.PermissionTarget{Action: action, ResourceFields: []string{"path"}},
			}, nil)
			r := &runner{cfg: Config{WorkDir: workDir}}
			inside := r.checkPermission(ToolCall{Name: "declared", Tool: declared, Args: map[string]any{"path": filepath.Join(workDir, "file.md")}})
			if inside.err != nil || len(inside.askResources) != 0 || inside.denyResource != "" {
				t.Fatalf("inside decision = %#v", inside)
			}
			external := r.checkPermission(ToolCall{Name: "declared", Tool: declared, Args: map[string]any{"path": filepath.Join(outside, "file.md")}})
			if external.err != nil || len(external.askResources) != 1 || external.askResources[0] != filepath.ToSlash(filepath.Join(outside, "*")) {
				t.Fatalf("outside decision = %#v", external)
			}
		})
	}
}
