package session

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

type lifecyclePlugin struct {
	name     string
	activate func(*plugin.Registry) (plugin.Cleanup, error)
}

func (p lifecyclePlugin) Name() string { return p.name }
func (p lifecyclePlugin) Activate(r *plugin.Registry) (plugin.Cleanup, error) {
	return p.activate(r)
}

func TestSessionActivatesPluginsOnceAndClosesOnce(t *testing.T) {
	var activations, cleanups int
	p := lifecyclePlugin{name: "lifecycle", activate: func(*plugin.Registry) (plugin.Cleanup, error) {
		activations++
		return func(context.Context) error { cleanups++; return nil }, nil
	}}
	client := &sequencedToolClient{messages: []models.Message{
		{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "one"}}},
		{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "two"}}},
	}}
	sess := New(WithClient(client), WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}), WithPlugin(p))
	if _, err := sess.Run(context.Background(), "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.Run(context.Background(), "second"); err != nil {
		t.Fatal(err)
	}
	if activations != 1 || cleanups != 0 {
		t.Fatalf("activations/cleanups = %d/%d", activations, cleanups)
	}
	if err := sess.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := sess.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cleanups != 1 {
		t.Fatalf("cleanups = %d, want 1", cleanups)
	}
	if _, err := sess.Run(context.Background(), "closed"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Run() error = %v, want ErrClosed", err)
	}
}

func TestSetPluginsStagesReplacementAndPreservesPreviousOnFailure(t *testing.T) {
	var oldCleanups int
	old := lifecyclePlugin{name: "old", activate: func(*plugin.Registry) (plugin.Cleanup, error) {
		return func(context.Context) error { oldCleanups++; return nil }, nil
	}}
	sess := New()
	if err := sess.SetPlugins(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	previous := sess.generation
	failing := lifecyclePlugin{name: "failing", activate: func(*plugin.Registry) (plugin.Cleanup, error) {
		return nil, errors.New("activation failed")
	}}
	if err := sess.SetPlugins(context.Background(), failing); err == nil {
		t.Fatal("SetPlugins succeeded")
	}
	if sess.generation != previous || oldCleanups != 0 {
		t.Fatalf("previous generation changed or cleaned: generation=%p previous=%p cleanups=%d", sess.generation, previous, oldCleanups)
	}
	if err := sess.SetPlugins(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sess.generation != nil || oldCleanups != 1 {
		t.Fatalf("generation/cleanups = %p/%d", sess.generation, oldCleanups)
	}
}

func TestSetPluginsRejectsRecursiveReplacement(t *testing.T) {
	sess := New()
	recursive := lifecyclePlugin{name: "recursive"}
	recursive.activate = func(*plugin.Registry) (plugin.Cleanup, error) {
		return nil, sess.SetPlugins(context.Background(), recursive)
	}
	if err := sess.SetPlugins(context.Background(), recursive); !errors.Is(err, ErrPluginReplacementInProgress) {
		t.Fatalf("SetPlugins() error = %v", err)
	}
	if sess.generation != nil {
		t.Fatal("failed recursive replacement changed generation")
	}
}

func TestConcurrentCloseWaitsForCleanup(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	p := lifecyclePlugin{name: "blocking-cleanup", activate: func(*plugin.Registry) (plugin.Cleanup, error) {
		return func(context.Context) error {
			close(started)
			<-release
			return nil
		}, nil
	}}
	sess := New()
	if err := sess.SetPlugins(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	first := make(chan error, 1)
	go func() { first <- sess.Close(context.Background()) }()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := sess.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("concurrent Close() error = %v", err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
}

func TestSetPluginsWaitsForActiveRunBeforeCleanup(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	client := &blockingRequestClient{entered: started, release: release}
	var mu sync.Mutex
	cleaned := false
	p := lifecyclePlugin{name: "active", activate: func(*plugin.Registry) (plugin.Cleanup, error) {
		return func(context.Context) error { mu.Lock(); cleaned = true; mu.Unlock(); return nil }, nil
	}}
	sess := New(WithClient(client), WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}), WithPlugin(p))
	runDone := make(chan error, 1)
	go func() { _, err := sess.Run(context.Background(), "run"); runDone <- err }()
	<-started
	replaceDone := make(chan error, 1)
	go func() { replaceDone <- sess.SetPlugins(context.Background()) }()
	time.Sleep(10 * time.Millisecond)
	mu.Lock()
	wasCleaned := cleaned
	mu.Unlock()
	if wasCleaned {
		t.Fatal("plugin cleaned while run was active")
	}
	close(release)
	if err := <-runDone; err == nil {
		t.Fatal("run unexpectedly succeeded")
	}
	if err := <-replaceDone; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !cleaned {
		t.Fatal("plugin was not cleaned after run")
	}
}

func TestPluginPartDecodersAreActiveBeforeHydration(t *testing.T) {
	data := memory.NewStore()
	storedSession := &store.Session{ID: "ses_plugin_parts"}
	if err := data.CreateSession(storedSession); err != nil {
		t.Fatal(err)
	}
	if err := data.SaveMessage(context.Background(), store.StoredMessage{
		ID: "msg_custom", SessionID: storedSession.ID, Idx: 0, Role: string(models.RoleAssistant), Revision: 1,
		State: string(models.MessageStateCompleted), Parts: []store.StoredPart{{ID: "prt_custom", MessageID: "msg_custom", Sequence: 0, Kind: "custom", PayloadJSON: []byte(`{"type":"custom","value":"raw"}`)}},
	}); err != nil {
		t.Fatal(err)
	}
	p := lifecyclePlugin{name: "custom-part", activate: func(registry *plugin.Registry) (plugin.Cleanup, error) {
		return nil, registry.RegisterPart("custom", func([]byte) (models.Part, error) {
			return models.OpaquePart{TypeName: "custom", Raw: []byte(`{"type":"custom","value":"decoded"}`)}, nil
		})
	}}
	client := &sequencedToolClient{messages: []models.Message{{Role: models.RoleAssistant, Content: models.Content{models.TextPart{Text: "done"}}}}}
	sess := New(WithID(storedSession.ID), WithStore(data), WithClient(client), WithModelRef(models.ModelRef{Provider: "test", ID: "model"}, models.ModelInfo{}), WithPlugin(p))
	if _, err := sess.Run(context.Background(), "continue"); err != nil {
		t.Fatal(err)
	}
	part := sess.History()[0].Content[0].(models.OpaquePart)
	if string(part.Raw) != `{"type":"custom","value":"decoded"}` {
		t.Fatalf("hydrated part = %s", part.Raw)
	}
}
