package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/store/memory"
)

func TestToolCatalogIsUniqueOrderedAndIncludesTraits(t *testing.T) {
	s := New(Config{Store: memory.NewStore()})
	response := httptest.NewRecorder()
	s.router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/tools", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var catalog toolCatalogResponse
	if err := json.NewDecoder(response.Body).Decode(&catalog); err != nil {
		t.Fatal(err)
	}
	seen := map[string]struct{}{}
	for i, item := range catalog.Tools {
		if i > 0 && catalog.Tools[i-1].Name >= item.Name {
			t.Fatalf("catalog is not strictly ordered: %#v", catalog.Tools)
		}
		if _, exists := seen[item.Name]; exists {
			t.Fatalf("duplicate catalog item %q", item.Name)
		}
		seen[item.Name] = struct{}{}
	}
	if item := catalogTool(catalog.Tools, "read"); item == nil || !item.DirectoryScoped {
		t.Fatalf("read catalog item = %#v", item)
	}
}

func TestAgentWritesRejectUnknownAndDuplicateTools(t *testing.T) {
	data := memory.NewStore()
	s := New(Config{Store: data})
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{name: "unknown", body: `{"name":"bad","tools":["missing"]}`, want: "unavailable"},
		{name: "duplicate", body: `{"name":"bad","tools":["read","read"]}`, want: "more than once"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/agents", strings.NewReader(tc.body))
			s.router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), tc.want) {
				t.Fatalf("status/body = %d/%s", response.Code, response.Body.String())
			}
		})
	}

	agent := &store.Agent{ID: "agt_update_tools", Name: "agent", Tools: []string{"read"}}
	if err := data.CreateAgent(agent); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/agents/"+agent.ID, strings.NewReader(`{"tools":["missing"]}`))
	s.router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("update status = %d: %s", response.Code, response.Body.String())
	}
}

func catalogTool(items []toolCatalogItem, name string) *toolCatalogItem {
	for i := range items {
		if items[i].Name == name {
			return &items[i]
		}
	}
	return nil
}
