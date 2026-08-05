package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestOpenAPIRepresentativeContract(t *testing.T) {
	s := New(Config{})
	response := httptest.NewRecorder()
	s.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}

	var document map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if document["openapi"] != "3.1.0" {
		t.Fatalf("openapi = %v", document["openapi"])
	}
	paths := document["paths"].(map[string]any)
	for _, path := range []string{"/health", "/auth/sessions", "/auth/sessions/{id}", "/clients", "/clients/{id}/token", "/agents", "/agents/{id}", "/sessions/{id}/events", "/sessions/{id}/events/history", "/run"} {
		if paths[path] == nil {
			t.Errorf("missing path %s", path)
		}
	}

	components := document["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	for _, name := range []string{"ErrorResponse", "Part", "SessionEvent", "RunStreamEvent"} {
		schema, ok := schemas[name].(map[string]any)
		if !ok {
			t.Fatalf("missing schema %s", name)
		}
		variants, _ := schema["oneOf"].([]any)
		if name == "Part" {
			variants, _ = schema["anyOf"].([]any)
		}
		if name != "ErrorResponse" && len(variants) == 0 {
			t.Errorf("schema %s has no variants", name)
		}
	}
}

func TestOpenAPIAuthenticationContract(t *testing.T) {
	document, err := OpenAPIDocument()
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(document, &decoded); err != nil {
		t.Fatal(err)
	}
	paths := decoded["paths"].(map[string]any)
	operation := func(path, method string) map[string]any {
		return paths[path].(map[string]any)[method].(map[string]any)
	}
	rootSecurity := operation("/clients/{id}/token", "post")["security"].([]any)
	if len(rootSecurity) != 2 || rootSecurity[0].(map[string]any)["rootBearer"] == nil || rootSecurity[1].(map[string]any)["consoleSession"] == nil {
		t.Fatalf("client token rotation security = %#v", rootSecurity)
	}
	readySecurity := operation("/ready", "get")["security"].([]any)
	if len(readySecurity) != 2 {
		t.Fatalf("readiness security = %#v", readySecurity)
	}
}

func TestOpenAPIEndpointMatchesGeneratedDocument(t *testing.T) {
	s := New(Config{})
	response := httptest.NewRecorder()
	s.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	document, err := OpenAPIDocument()
	if err != nil {
		t.Fatal(err)
	}
	var published, generated any
	if err := json.Unmarshal(response.Body.Bytes(), &published); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(document, &generated); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(published, generated) {
		t.Fatal("published OpenAPI document differs from generated document")
	}
}

func TestOpenAPIMatchesRegisteredAPIRoutes(t *testing.T) {
	s := New(Config{})
	document, err := OpenAPIDocument()
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(document, &decoded); err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	for path, item := range decoded.Paths {
		for method := range item {
			documented[strings.ToUpper(method)+" "+path] = true
		}
	}

	registered := map[string]bool{}
	err = chi.Walk(s.router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if strings.HasPrefix(route, "/openapi") || strings.HasPrefix(route, "/console") {
			return nil
		}
		registered[method+" "+route] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for route := range registered {
		if !documented[route] {
			t.Errorf("registered route is undocumented: %s", route)
		}
	}
	for route := range documented {
		if !registered[route] {
			t.Errorf("documented route is unregistered: %s", route)
		}
	}
}

func TestOpenAPIRoutePreservesHandlerBehavior(t *testing.T) {
	s := New(Config{})
	response := httptest.NewRecorder()
	s.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))
	if response.Code != http.StatusOK || response.Body.String() != "{\"status\":\"ok\"}\n" {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
}
