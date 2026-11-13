package pluginhub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/chaserensberger/wingman/execution"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/plugin/protocol"
	"github.com/chaserensberger/wingman/pluginhost"
)

func TestInstallUpdateRollbackRemoveAndRestart(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "plugin")
	command := exec.Command("go", "build", "-o", binary, "../examples/plugins/projectcontext")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build plugin: %v\n%s", err, output)
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])
	var catalog Catalog
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/catalog.json" {
			_ = json.NewEncoder(w).Encode(catalog)
			return
		}
		_, _ = w.Write(data)
	}))
	defer server.Close()
	catalog = Catalog{Version: 1, Packages: []Package{{ID: "example/project-context", Name: "Project context", Releases: []Release{{Version: "v1.0.0", Protocol: protocol.Version, Artifacts: map[string]Artifact{runtime.GOOS + "/" + runtime.GOARCH: {URL: server.URL + "/plugin", SHA256: checksum}}}}}}}
	root := t.TempDir()
	providers, err := provider.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	scopes, err := execution.NewManager(execution.Config{Providers: providers, PluginDirs: []string{root}, PluginState: pluginhost.State{Root: t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	defer scopes.Close()
	lease, err := scopes.Acquire(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close(context.Background())
	manager := lease.Scope().Plugins()
	installer := Installer{Root: root, CatalogURL: server.URL + "/catalog.json", WingmanVersion: "v1.0.0", Client: server.Client(), Update: scopes.UpdatePlugins}
	installed, err := installer.Install(t.Context(), "example/project-context", false)
	if err != nil {
		t.Fatal(err)
	}
	if installed.Version != "v1.0.0" {
		t.Fatalf("installed = %#v", installed)
	}
	if len(manager.Tools()) != 1 {
		t.Fatal("installed plugin was not activated")
	}
	list, err := installer.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %#v, %v", list, err)
	}
	restarted, err := pluginhost.New(context.Background(), []string{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(restarted.Tools()) != 1 {
		t.Fatal("restart lost installed plugin")
	}
	_ = restarted.Close()
	installer.Update = func(ctx context.Context, change, rollback func() error) error {
		if err := change(); err != nil {
			return err
		}
		return errors.Join(errors.New("replacement rejected"), rollback())
	}
	before, _ := os.ReadFile(filepath.Join(installer.directory(installed.ID), "wingman-plugin.json"))
	if _, err := installer.Install(t.Context(), "example/project-context", true); err == nil {
		t.Fatal("activation failure was ignored")
	}
	after, _ := os.ReadFile(filepath.Join(installer.directory(installed.ID), "wingman-plugin.json"))
	if string(before) != string(after) {
		t.Fatal("failed update changed selected installation")
	}
	if len(manager.Tools()) != 1 {
		t.Fatal("failed update removed active tools")
	}
	catalog.Packages[0].Releases[0].Artifacts[runtime.GOOS+"/"+runtime.GOARCH] = Artifact{URL: server.URL + "/plugin", SHA256: "0000000000000000000000000000000000000000000000000000000000000000"}
	if _, err := installer.Install(t.Context(), "example/project-context", true); err == nil {
		t.Fatal("mutated release was accepted")
	}
	installer.Update = scopes.UpdatePlugins
	if err := installer.Remove(t.Context(), installed.ID); err != nil {
		t.Fatal(err)
	}
	list, err = installer.List()
	if err != nil || len(list) != 0 || len(manager.Tools()) != 0 {
		t.Fatalf("removed plugin remains: %#v, %v", list, err)
	}
}

func TestPackageRollbackExcludesConcurrentScopeConstruction(t *testing.T) {
	providers, err := provider.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	scopes, err := execution.NewManager(execution.Config{Providers: providers, PluginDirs: []string{root}, PluginState: pluginhost.State{Root: t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	defer scopes.Close()
	project := t.TempDir()
	manifest := filepath.Join(root, "wingman-plugin.json")
	published, restore := make(chan struct{}), make(chan struct{})
	updated := make(chan error, 1)
	go func() {
		updated <- scopes.UpdatePlugins(t.Context(), func() error {
			if err := os.WriteFile(manifest, []byte("invalid manifest"), 0600); err != nil {
				return err
			}
			close(published)
			<-restore
			return nil
		}, func() error { return os.Remove(manifest) })
	}()
	<-published
	acquired := make(chan error, 1)
	go func() {
		lease, err := scopes.Acquire(t.Context(), project)
		if lease != nil {
			_ = lease.Close(context.Background())
		}
		acquired <- err
	}()
	select {
	case err := <-acquired:
		close(restore)
		<-updated
		t.Fatalf("scope observed tentative manifest: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(restore)
	if err := <-updated; err == nil {
		t.Fatal("invalid manifest was accepted")
	}
	if err := <-acquired; err != nil {
		t.Fatalf("scope did not observe rollback: %v", err)
	}
}

func TestCatalogValidationAndResolution(t *testing.T) {
	artifact := Artifact{URL: "https://example.com/plugin", SHA256: "0000000000000000000000000000000000000000000000000000000000000000"}
	catalog := Catalog{Version: 1, Packages: []Package{{ID: "acme/search", Name: "Search", Description: "Find documents", Releases: []Release{
		{Version: "v2.0.0", Protocol: protocol.Version, MinimumWingman: "v2.0.0", Artifacts: map[string]Artifact{"linux/amd64": artifact}},
		{Version: "v1.0.0", Protocol: protocol.Version, Artifacts: map[string]Artifact{"linux/amd64": artifact}},
	}}}}
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	_, release, _, err := catalog.Resolve("acme/search", "linux/amd64", "v1.0.0")
	if err != nil || release.Version != "v1.0.0" {
		t.Fatalf("resolve = %#v, %v", release, err)
	}
	if len(catalog.Search("documents")) != 1 {
		t.Fatal("search did not match description")
	}
	for _, source := range []string{"../escape", "acme/search@latest", "missing/plugin", "acme/search@v9.0.0"} {
		if _, _, _, err := catalog.Resolve(source, "linux/amd64", "v1.0.0"); err == nil {
			t.Fatalf("accepted %q", source)
		}
	}
	if _, _, _, err := catalog.Resolve("acme/search", "unknown/platform", "v1.0.0"); err == nil {
		t.Fatal("accepted unsupported platform")
	}
	bad := catalog
	bad.Packages = append(append([]Package(nil), catalog.Packages...), catalog.Packages[0])
	if err := bad.Validate(); err == nil {
		t.Fatal("accepted duplicate identity")
	}
	if err := secureURL("http://example.com/plugin"); err == nil {
		t.Fatal("accepted insecure URL")
	}
}

type interruptedArtifact struct{ wrote bool }

func (r *interruptedArtifact) Read(p []byte) (int, error) {
	if !r.wrote {
		r.wrote = true
		return copy(p, "partial executable"), nil
	}
	return 0, io.ErrUnexpectedEOF
}

func TestArtifactWriteFailureDoesNotPublishPartialExecutable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin")
	for _, existing := range []bool{false, true} {
		if existing {
			if err := os.WriteFile(path, []byte("working"), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		if err := atomicWriteReader(path, &interruptedArtifact{}, 0o700); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("write error = %v", err)
		}
		data, err := os.ReadFile(path)
		if existing && (err != nil || string(data) != "working") {
			t.Fatalf("failed write changed working executable: %q, %v", data, err)
		}
		if !existing && !os.IsNotExist(err) {
			t.Fatalf("partial executable was published: %q, %v", data, err)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		want := 0
		if existing {
			want = 1
		}
		if len(entries) != want {
			t.Fatalf("failed write left staging files: %#v", entries)
		}
	}
	if err := atomicWriteMode(path, []byte("complete"), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "complete" {
		t.Fatalf("retry = %q, %v", data, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatalf("executable mode = %v", info.Mode())
	}
}
