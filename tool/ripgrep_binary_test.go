package tool

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func ripgrepTestArchive(t *testing.T, extension, name, content string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if extension == "zip" {
		archive := zip.NewWriter(&buffer)
		file, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
		if err := archive.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		compressed := gzip.NewWriter(&buffer)
		archive := tar.NewWriter(compressed)
		if err := archive.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
		if err := archive.Close(); err != nil {
			t.Fatal(err)
		}
		if err := compressed.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return buffer.Bytes()
}

func TestRipgrepPrefersSystemBinary(t *testing.T) {
	dir := t.TempDir()
	name := "rg"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(dir, name)
	if err := os.WriteFile(binary, []byte("system ripgrep"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	setOutputCacheDir(t, "")
	t.Setenv("LOCALAPPDATA", "")
	got, err := ripgrepBinary(t.Context())
	if err != nil || got != binary {
		t.Fatalf("system binary = %q, %v; want %q", got, err, binary)
	}
}

func TestRipgrepDownloadAndCache(t *testing.T) {
	for _, extension := range []string{"tar.gz", "zip"} {
		t.Run(extension, func(t *testing.T) {
			release := ripgrepRelease{target: "test-platform", extension: extension}
			name := "rg"
			if extension == "zip" {
				name += ".exe"
			}
			stem := "ripgrep-" + ripgrepVersion + "-" + release.target
			archive := ripgrepTestArchive(t, extension, stem+"/"+name, "verified executable")
			release.sha256 = fmt.Sprintf("%x", sha256.Sum256(archive))
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.URL.Path != "/"+stem+"."+extension {
					t.Errorf("download path = %q", r.URL.Path)
				}
				_, _ = w.Write(archive)
			}))
			defer server.Close()
			installer := ripgrepInstaller{cache: t.TempDir(), release: release, client: server.Client(), baseURL: server.URL}
			var wg sync.WaitGroup
			for range 12 {
				wg.Go(func() {
					binary, err := installer.binary(t.Context())
					if err != nil {
						t.Error(err)
						return
					}
					data, err := os.ReadFile(binary)
					if err != nil || string(data) != "verified executable" {
						t.Errorf("binary = %q, %v", data, err)
					}
				})
			}
			wg.Wait()
			if requests.Load() != 1 {
				t.Fatalf("downloads = %d; want 1", requests.Load())
			}
			server.Close()
			binary, err := installer.binary(t.Context())
			if err != nil {
				t.Fatalf("offline cache reuse: %v", err)
			}
			info, err := os.Stat(binary)
			if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0755) {
				t.Fatalf("executable permissions: %v, %v", info, err)
			}
			entries, err := os.ReadDir(filepath.Dir(binary))
			if err != nil || len(entries) != 1 || entries[0].Name() != name {
				t.Fatalf("cache contains temporary files: %v, %v", entries, err)
			}
		})
	}
}

func TestRipgrepDownloadFailures(t *testing.T) {
	stem := "ripgrep-" + ripgrepVersion + "-test-platform"
	good := ripgrepTestArchive(t, "tar.gz", stem+"/rg", "verified executable")
	for _, tc := range []struct {
		name, want string
		status     int
		body       []byte
		checksum   string
	}{
		{"HTTP failure", "HTTP 503", 503, nil, ""},
		{"checksum mismatch", "checksum mismatch", 200, good, strings.Repeat("0", 64)},
		{"invalid archive", "gzip", 200, []byte("not a gzip archive"), ""},
		{"unexpected path", "does not contain", 200, ripgrepTestArchive(t, "tar.gz", "../rg", "wrong file"), ""},
		{"empty executable", "invalid ripgrep executable size", 200, ripgrepTestArchive(t, "tar.gz", stem+"/rg", ""), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write(tc.body)
			}))
			defer server.Close()
			digest := tc.checksum
			if digest == "" {
				digest = fmt.Sprintf("%x", sha256.Sum256(tc.body))
			}
			cache := t.TempDir()
			installer := ripgrepInstaller{cache: cache, release: ripgrepRelease{"test-platform", "tar.gz", digest}, client: server.Client(), baseURL: server.URL}
			if _, err := installer.binary(t.Context()); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v; want %q", err, tc.want)
			}
			entries, err := os.ReadDir(filepath.Join(cache, "wingman", "bin", stem))
			if err != nil || len(entries) != 0 {
				t.Fatalf("failed install left files: %v, %v", entries, err)
			}
		})
	}
}

func TestRipgrepDownloadCancellationAndRetry(t *testing.T) {
	archive := ripgrepTestArchive(t, "tar.gz", "ripgrep-"+ripgrepVersion+"-test-platform/rg", "verified executable")
	started := make(chan struct{})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			close(started)
			<-r.Context().Done()
			return
		}
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	installer := ripgrepInstaller{cache: t.TempDir(), release: ripgrepRelease{"test-platform", "tar.gz", fmt.Sprintf("%x", sha256.Sum256(archive))}, client: server.Client(), baseURL: server.URL}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := installer.binary(ctx); done <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("download did not start")
	}
	waiting, stop := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer stop()
	if _, err := installer.binary(waiting); err == nil {
		t.Fatal("waiting download ignored cancellation")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("download ignored cancellation")
	}
	if _, err := installer.binary(t.Context()); err != nil {
		t.Fatalf("retry after canceled download: %v", err)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d; want 2", requests.Load())
	}
}

func TestRipgrepDownloadIntegration(t *testing.T) {
	if os.Getenv("WINGMAN_TEST_RIPGREP_DOWNLOAD") != "1" {
		t.Skip("set WINGMAN_TEST_RIPGREP_DOWNLOAD=1 to test the upstream download")
	}
	t.Setenv("PATH", t.TempDir())
	cache := t.TempDir()
	setOutputCacheDir(t, cache)
	t.Setenv("LOCALAPPDATA", cache)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "example.go"), []byte("needle\n"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := NewGrepTool().Execute(t.Context(), Invocation{WorkDir: dir, Input: map[string]any{"pattern": "needle"}})
	if err != nil || result.Metadata["matches"] != 1 {
		t.Fatalf("grep with automatic download = %#v, %v", result, err)
	}
	result, err = NewGlobTool().Execute(t.Context(), Invocation{WorkDir: dir, Input: map[string]any{"pattern": "*.go"}})
	if err != nil || result.Metadata["count"] != 1 {
		t.Fatalf("glob with cached binary = %#v, %v", result, err)
	}
	binary, err := ripgrepBinary(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	version, err := exec.CommandContext(t.Context(), binary, "--version").Output()
	if err != nil || !strings.HasPrefix(string(version), "ripgrep "+ripgrepVersion) {
		t.Fatalf("downloaded version = %q, %v", version, err)
	}
	t.Logf("downloaded ripgrep %s; grep and glob passed without rg on PATH", ripgrepVersion)
}
