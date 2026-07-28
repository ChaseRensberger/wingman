package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUpdateAvailable(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    bool
	}{
		{"dev", "v0.2.0", true},
		{"0.1.0", "v0.2.0", true},
		{"0.2.0", "v0.2.0", false},
		{"0.3.0", "v0.2.0", false},
	}
	for _, test := range tests {
		got, err := updateAvailable(test.current, test.latest)
		if err != nil {
			t.Fatalf("updateAvailable(%q, %q): %v", test.current, test.latest, err)
		}
		if got != test.want {
			t.Errorf("updateAvailable(%q, %q) = %t, want %t", test.current, test.latest, got, test.want)
		}
	}
}

func TestShouldInstallUpdate(t *testing.T) {
	tests := []struct {
		requestedVersion string
		available        bool
		want             bool
	}{
		{"", false, false},
		{"", true, true},
		{"0.2.0", false, true},
		{"0.1.0", false, true},
	}
	for _, test := range tests {
		if got := shouldInstallUpdate(test.requestedVersion, test.available); got != test.want {
			t.Errorf("shouldInstallUpdate(%q, %t) = %t, want %t", test.requestedVersion, test.available, got, test.want)
		}
	}
}

func TestInstallReleaseVerifiesAndReplacesBinary(t *testing.T) {
	archive := testArchive(t, "new wingman binary")
	archiveName := fmt.Sprintf("wingman_0.2.0_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(archive)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/archive":
			_, _ = w.Write(archive)
		case "/checksums":
			_, _ = fmt.Fprintf(w, "%x  %s\n", sum, archiveName)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	exe := filepath.Join(dir, "wingman")
	if err := os.WriteFile(exe, []byte("old binary"), 0755); err != nil {
		t.Fatal(err)
	}
	release := release{TagName: "v0.2.0", Assets: []releaseAsset{
		{Name: archiveName, DownloadURL: server.URL + "/archive"},
		{Name: "checksums.txt", DownloadURL: server.URL + "/checksums"},
	}}
	if err := installRelease(context.Background(), server.Client(), release, exe); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new wingman binary" {
		t.Errorf("updated binary = %q", got)
	}
}

func TestInstallReleaseRejectsChecksumMismatch(t *testing.T) {
	archive := testArchive(t, "new wingman binary")
	archiveName := fmt.Sprintf("wingman_0.2.0_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/archive" {
			_, _ = w.Write(archive)
			return
		}
		_, _ = fmt.Fprintf(w, "%064d  %s\n", 0, archiveName)
	}))
	defer server.Close()

	dir := t.TempDir()
	exe := filepath.Join(dir, "wingman")
	if err := os.WriteFile(exe, []byte("old binary"), 0755); err != nil {
		t.Fatal(err)
	}
	release := release{TagName: "v0.2.0", Assets: []releaseAsset{
		{Name: archiveName, DownloadURL: server.URL + "/archive"},
		{Name: "checksums.txt", DownloadURL: server.URL + "/checksums"},
	}}
	if err := installRelease(context.Background(), server.Client(), release, exe); err == nil {
		t.Fatal("installRelease() succeeded with a mismatched checksum")
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old binary" {
		t.Errorf("binary changed after checksum failure: %q", got)
	}
}

func testArchive(t *testing.T, contents string) []byte {
	t.Helper()
	var compressed bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressed)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "wingman", Mode: 0755, Size: int64(len(contents))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write([]byte(contents)); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return compressed.Bytes()
}
