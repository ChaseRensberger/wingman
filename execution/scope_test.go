package execution

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	wingmcp "github.com/chaserensberger/wingman/mcp"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/pluginhost"
	"github.com/chaserensberger/wingman/tool"
)

func TestManagerCanonicalizesPinsAndEvictsScopes(t *testing.T) {
	registry, err := provider.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	var constructions atomic.Int32
	m, err := newManager(Config{Providers: registry, DisablePlugins: true, IdleTimeout: 10 * time.Millisecond}, factories{
		newPlugins: pluginhost.New,
		newMCP: func(context.Context, wingmcp.Config) *wingmcp.Manager {
			constructions.Add(1)
			return wingmcp.New(context.Background(), wingmcp.Config{})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	dir := t.TempDir()
	link := filepath.Join(t.TempDir(), "scope")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	first, err := m.Acquire(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Acquire(context.Background(), link)
	if err != nil {
		t.Fatal(err)
	}
	canonicalDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first.Scope() != second.Scope() || first.Scope().ID() != canonicalDir {
		t.Fatalf("canonical scopes differ: %p %p %q", first.Scope(), second.Scope(), first.Scope().ID())
	}
	_ = first.Close(context.Background())
	_ = second.Close(context.Background())
	waitForScope(t, func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.scopes[canonicalDir] == nil
	})
	third, err := m.Acquire(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if third.Scope() == first.Scope() {
		t.Fatal("idle scope was not evicted")
	}
	if got := constructions.Load(); got != 3 {
		t.Fatalf("MCP constructions = %d, want default plus two directory generations", got)
	}
	_ = third.Close(context.Background())
}

func waitForScope(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !ready() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for scope state")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestManagerKeepsDirectorylessScopePinned(t *testing.T) {
	registry, err := provider.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(Config{Providers: registry, DisablePlugins: true, IdleTimeout: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	first, err := m.Acquire(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Close(context.Background())
	time.Sleep(5 * time.Millisecond)
	second, err := m.Acquire(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if first.Scope() != second.Scope() {
		t.Fatal("directoryless scope was evicted")
	}
	_ = second.Close(context.Background())
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Acquire(context.Background(), ""); err == nil {
		t.Fatal("Acquire succeeded after Close")
	}
}

func TestManagerRejectsInvalidDirectories(t *testing.T) {
	registry, err := provider.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(Config{Providers: registry, DisablePlugins: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	if _, err := m.Acquire(context.Background(), filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("Acquire accepted missing directory")
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Acquire(context.Background(), file); err == nil {
		t.Fatal("Acquire accepted file")
	}
}

func TestManagerKeepsProjectPluginDirectoriesScopeLocal(t *testing.T) {
	registry, err := provider.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	global := t.TempDir()
	var discovered [][]string
	m, err := newManager(Config{Providers: registry, PluginDirs: []string{global}}, factories{
		newPlugins: func(ctx context.Context, dirs []string) (*pluginhost.Manager, error) {
			discovered = append(discovered, append([]string(nil), dirs...))
			return pluginhost.New(ctx, nil)
		},
		newMCP: wingmcp.New,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	firstDir := t.TempDir()
	secondDir := t.TempDir()
	first, err := m.Acquire(context.Background(), firstDir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Acquire(context.Background(), secondDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered) != 3 {
		t.Fatalf("plugin generations = %d, want default plus two directory scopes", len(discovered))
	}
	wantFirst := []string{global, pluginhost.LocalPluginDir(first.Scope().WorkDir())}
	wantSecond := []string{global, pluginhost.LocalPluginDir(second.Scope().WorkDir())}
	if !reflect.DeepEqual(discovered[1], wantFirst) || !reflect.DeepEqual(discovered[2], wantSecond) {
		t.Fatalf("plugin dirs = %#v, want %#v then %#v", discovered, wantFirst, wantSecond)
	}
	_ = first.Close(context.Background())
	_ = second.Close(context.Background())
}

func TestManagerRejectsInvalidInitialToolGeneration(t *testing.T) {
	registry, err := provider.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewManager(Config{
		Providers: registry, DisablePlugins: true,
		NativeTools: []tool.Tool{tool.NewReadTool(), tool.NewReadTool()},
	})
	if err == nil {
		t.Fatal("NewManager accepted duplicate tools")
	}
}

func TestCloseCancelsConstructionWithoutWaitingForCacheLock(t *testing.T) {
	registry, err := provider.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	var constructions atomic.Int32
	m, err := newManager(Config{Providers: registry, DisablePlugins: true}, factories{
		newPlugins: pluginhost.New,
		newMCP: func(ctx context.Context, cfg wingmcp.Config) *wingmcp.Manager {
			if constructions.Add(1) > 1 {
				close(started)
				<-ctx.Done()
			}
			return wingmcp.New(context.Background(), cfg)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	acquired := make(chan error, 1)
	go func() {
		_, err := m.Acquire(context.Background(), dir)
		acquired <- err
	}()
	<-started
	closed := make(chan error, 1)
	go func() { closed <- m.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close blocked behind scope construction")
	}
	if err := <-acquired; err == nil {
		t.Fatal("Acquire succeeded during manager close")
	}
}

func TestCanceledOnlyAcquireRollsBackConstruction(t *testing.T) {
	registry, err := provider.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	stopped := make(chan struct{})
	var constructions atomic.Int32
	m, err := newManager(Config{Providers: registry, DisablePlugins: true}, factories{
		newPlugins: pluginhost.New,
		newMCP: func(ctx context.Context, cfg wingmcp.Config) *wingmcp.Manager {
			if constructions.Add(1) > 1 {
				close(started)
				<-ctx.Done()
				close(stopped)
			}
			return wingmcp.New(context.Background(), cfg)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	acquired := make(chan error, 1)
	go func() {
		_, err := m.Acquire(ctx, dir)
		acquired <- err
	}()
	<-started
	cancel()
	if err := <-acquired; !errors.Is(err, context.Canceled) {
		t.Fatalf("Acquire error = %v, want canceled", err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("abandoned construction did not stop")
	}
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	_, exists := m.scopes[canonical]
	m.mu.Unlock()
	if exists {
		t.Fatal("abandoned scope remained cached")
	}
}

func TestCanceledWaiterDoesNotAbortSharedConstruction(t *testing.T) {
	registry, err := provider.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var constructions atomic.Int32
	m, err := newManager(Config{Providers: registry, DisablePlugins: true}, factories{
		newPlugins: pluginhost.New,
		newMCP: func(_ context.Context, cfg wingmcp.Config) *wingmcp.Manager {
			if constructions.Add(1) > 1 {
				close(started)
				<-release
			}
			return wingmcp.New(context.Background(), cfg)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	dir := t.TempDir()
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, err := m.Acquire(firstCtx, dir)
		firstDone <- err
	}()
	<-started
	secondDone := make(chan struct {
		lease *Lease
		err   error
	}, 1)
	go func() {
		lease, err := m.Acquire(context.Background(), dir)
		secondDone <- struct {
			lease *Lease
			err   error
		}{lease: lease, err: err}
	}()
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	waitForScope(t, func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		owned := m.scopes[canonical]
		return owned != nil && owned.waiters == 2
	})
	cancelFirst()
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("first Acquire error = %v, want canceled", err)
	}
	close(release)
	second := <-secondDone
	if second.err != nil {
		t.Fatal(second.err)
	}
	if got := constructions.Load(); got != 2 {
		t.Fatalf("scope constructions = %d, want default plus one shared candidate", got)
	}
	_ = second.lease.Close(context.Background())
}
