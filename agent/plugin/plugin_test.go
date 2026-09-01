package plugin

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/tool"
)

type testPlugin struct {
	name     string
	activate func(*Registry) (Cleanup, error)
}

func TestRegisterToolRejectsReservedSkillName(t *testing.T) {
	registry := NewRegistry()
	err := registry.RegisterTool(tool.NewFuncTool("skill", "replacement", tool.Definition{Name: "skill", InputSchema: tool.InputSchema{Type: "object"}}, func(context.Context, tool.Invocation) (tool.Result, error) {
		return tool.Result{}, nil
	}))
	if err == nil {
		t.Fatal("RegisterTool accepted the reserved skill name")
	}
}

func (p testPlugin) Name() string                          { return p.name }
func (p testPlugin) Activate(r *Registry) (Cleanup, error) { return p.activate(r) }

func TestActionsRequireOwnedIDsAndUniqueCommands(t *testing.T) {
	bad := testPlugin{name: "alpha", activate: func(r *Registry) (Cleanup, error) {
		return nil, r.RegisterAction(Action{ID: "beta.run", Command: "run", Handler: func(context.Context, ActionInfo) error { return nil }})
	}}
	if _, err := ActivateAll(bad); err == nil {
		t.Fatal("ActivateAll accepted an action owned by another plugin")
	}

	handler := func(context.Context, ActionInfo) error { return nil }
	first := testPlugin{name: "alpha", activate: func(r *Registry) (Cleanup, error) {
		return nil, r.RegisterAction(Action{ID: "alpha.run", Command: "run", Handler: handler})
	}}
	second := testPlugin{name: "beta", activate: func(r *Registry) (Cleanup, error) {
		return nil, r.RegisterAction(Action{ID: "beta.run", Command: "run", Handler: handler})
	}}
	if _, err := ActivateAll(first, second); err == nil {
		t.Fatal("ActivateAll accepted duplicate action commands")
	}
}

func TestActivateAllOrdersHooksAndClosesInReverseOnce(t *testing.T) {
	var order []string
	var mu sync.Mutex
	plugin := func(name string) testPlugin {
		return testPlugin{name: name, activate: func(r *Registry) (Cleanup, error) {
			if err := r.RegisterBeforeRun(func(_ context.Context, messages []models.Message) ([]models.Message, error) {
				mu.Lock()
				order = append(order, name+":hook")
				mu.Unlock()
				return messages, nil
			}); err != nil {
				return nil, err
			}
			return func(context.Context) error { mu.Lock(); order = append(order, name+":close"); mu.Unlock(); return nil }, nil
		}}
	}
	generation, err := ActivateAll(plugin("first"), plugin("second"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := generation.Runtime().Hooks.BeforeRun(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if err := generation.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := generation.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"first:hook", "second:hook", "second:close", "first:close"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestActivateAllRollsBackAndDoesNotLeakParts(t *testing.T) {
	var cleanup []string
	first := testPlugin{name: "first", activate: func(r *Registry) (Cleanup, error) {
		if err := r.RegisterPart("scoped", func(data []byte) (models.Part, error) { return models.OpaquePart{TypeName: "scoped", Raw: data}, nil }); err != nil {
			return nil, err
		}
		return func(context.Context) error { cleanup = append(cleanup, "first"); return nil }, nil
	}}
	failing := testPlugin{name: "failing", activate: func(*Registry) (Cleanup, error) { return nil, errors.New("nope") }}
	if _, err := ActivateAll(first, failing); err == nil {
		t.Fatal("ActivateAll succeeded")
	}
	if len(cleanup) != 1 || cleanup[0] != "first" {
		t.Fatalf("cleanup = %v", cleanup)
	}
	part, err := models.UnmarshalPart([]byte(`{"type":"scoped"}`))
	if err != nil {
		t.Fatal(err)
	}
	if part.Type() != "scoped" {
		t.Fatalf("part type = %q", part.Type())
	}
}

func TestGenerationCloseJoinsCleanupErrors(t *testing.T) {
	firstErr := errors.New("first")
	secondErr := errors.New("second")
	makePlugin := func(name string, err error) testPlugin {
		return testPlugin{name: name, activate: func(*Registry) (Cleanup, error) {
			return func(context.Context) error { return err }, nil
		}}
	}
	generation, err := ActivateAll(makePlugin("first", firstErr), makePlugin("second", secondErr))
	if err != nil {
		t.Fatal(err)
	}
	err = generation.Close(context.Background())
	if !errors.Is(err, firstErr) || !errors.Is(err, secondErr) {
		t.Fatalf("close error = %v", err)
	}
}

func TestActivateAllBuildFailureRollsBackAndCleanRebuildRemovesParts(t *testing.T) {
	var cleaned int
	custom := func(name, partType string) testPlugin {
		return testPlugin{name: name, activate: func(r *Registry) (Cleanup, error) {
			if err := r.RegisterPart(partType, func(data []byte) (models.Part, error) {
				return models.OpaquePart{TypeName: partType, ID: "decoded", Raw: data}, nil
			}); err != nil {
				return nil, err
			}
			return func(context.Context) error { cleaned++; return nil }, nil
		}}
	}
	if _, err := ActivateAll(custom("one", "same"), custom("two", "same")); err == nil {
		t.Fatal("duplicate parts built")
	}
	if cleaned != 2 {
		t.Fatalf("cleaned = %d, want 2", cleaned)
	}
	generation, err := ActivateAll(custom("one", "only"))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := generation.Parts().UnmarshalPart([]byte(`{"type":"only"}`))
	if err != nil {
		t.Fatal(err)
	}
	if models.PartID(decoded) != "decoded" {
		t.Fatalf("decoded part = %#v", decoded)
	}
	if err := generation.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	next, err := ActivateAll(testPlugin{name: "empty", activate: func(*Registry) (Cleanup, error) { return nil, nil }})
	if err != nil {
		t.Fatal(err)
	}
	part, err := next.Parts().UnmarshalPart([]byte(`{"type":"only"}`))
	if err != nil {
		t.Fatal(err)
	}
	if part.Type() != "only" || models.PartID(part) != "" {
		t.Fatalf("part = %#v", part)
	}
}

func TestRegistryRejectsPostBuildMutation(t *testing.T) {
	r := NewRegistry()
	_, _, _ = r.build()
	if err := r.RegisterSink(run.SinkFunc(func(run.Event) {})); err == nil {
		t.Fatal("post-build mutation succeeded")
	}
}

func TestActivateAllRejectsInvalidPluginLists(t *testing.T) {
	if _, err := ActivateAll(); err == nil {
		t.Fatal("empty plugins accepted")
	}
	if _, err := ActivateAll(nil); err == nil {
		t.Fatal("nil plugin accepted")
	}
	p := testPlugin{name: "same", activate: func(*Registry) (Cleanup, error) { return nil, nil }}
	if _, err := ActivateAll(p, p); err == nil {
		t.Fatal("duplicate plugins accepted")
	}
}

type blockingSink struct {
	started chan struct{}
	release chan struct{}
	calls   int
	mu      sync.Mutex
}

func (s *blockingSink) OnEvent(run.Event) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	select {
	case s.started <- struct{}{}:
	default:
	}
	<-s.release
}

func TestSinkTimeoutBoundsInFlightCallbacks(t *testing.T) {
	sink := &blockingSink{started: make(chan struct{}, 1), release: make(chan struct{})}
	p := testPlugin{name: "sink", activate: func(r *Registry) (Cleanup, error) { return nil, r.RegisterSinkTimeout(sink, 10*time.Millisecond) }}
	generation, err := ActivateAll(p)
	if err != nil {
		t.Fatal(err)
	}
	generation.Runtime().Sink.OnEvent(run.MessageEvent{})
	<-sink.started
	generation.Runtime().Sink.OnEvent(run.MessageEvent{})
	sink.mu.Lock()
	calls := sink.calls
	sink.mu.Unlock()
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	close(sink.release)
	deadline := time.After(time.Second)
	for {
		sink.mu.Lock()
		calls = sink.calls
		sink.mu.Unlock()
		if calls == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("sink did not return")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}
