package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestCreateWorkspaceDerivesNameFromDirectory(t *testing.T) {
	dir := t.TempDir()
	server := New(Config{Store: memory.NewStore()})
	request := httptest.NewRequest(http.MethodPost, "/workspaces", strings.NewReader(`{"path":"`+dir+`"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
	}
	var workspace store.Workspace
	if err := json.NewDecoder(response.Body).Decode(&workspace); err != nil {
		t.Fatal(err)
	}
	if workspace.Name != filepath.Base(dir) || workspace.Path != dir {
		t.Fatalf("workspace = %#v, want name and path derived from %q", workspace, dir)
	}
}

func TestCreateWorkspaceExpandsHomeDirectory(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{Store: memory.NewStore()})
	request := httptest.NewRequest(http.MethodPost, "/workspaces", strings.NewReader(`{"path":"~"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
	}
	var workspace store.Workspace
	if err := json.NewDecoder(response.Body).Decode(&workspace); err != nil {
		t.Fatal(err)
	}
	if workspace.Path != home {
		t.Fatalf("workspace path = %q, want %q", workspace.Path, home)
	}
}

func TestListDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	server := New(Config{Store: memory.NewStore()})
	request := httptest.NewRequest(http.MethodGet, "/filesystem/directories?path="+dir, nil)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var listing directoryListing
	if err := json.NewDecoder(response.Body).Decode(&listing); err != nil {
		t.Fatal(err)
	}
	if listing.Path != dir || len(listing.Entries) != 1 || listing.Entries[0].Name != "alpha" {
		t.Fatalf("listing = %#v, want only alpha directory", listing)
	}
}
