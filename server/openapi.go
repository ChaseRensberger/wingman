package server

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"

	"github.com/chaserensberger/wingman/api"
	"github.com/chaserensberger/wingman/models"
)

const openAPIVersion = "0.0.0"

type rootResponse struct {
	Name             string `json:"name"`
	Status           string `json:"status"`
	Health           string `json:"health"`
	Console          string `json:"console"`
	RestartAvailable bool   `json:"restart_available"`
}

type providerOAuthRequest struct {
	Method string `json:"method"`
}

// OpenAPIDocument returns the exact OpenAPI document published by a new server.
func OpenAPIDocument() ([]byte, error) {
	s := New(Config{})
	return json.MarshalIndent(s.protocol.OpenAPI(), "", "  ")
}

func (s *Server) setupOpenAPI() {
	config := huma.DefaultConfig("Wingman API", openAPIVersion)
	config.DocsPath = ""
	config.SchemasPath = ""
	config.CreateHooks = nil
	config.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"basicAuth": {Type: "http", Scheme: "basic", Description: "Daemon username and password"},
	}
	s.protocol = humachi.New(s.router, config)

	registry := s.protocol.OpenAPI().Components.Schemas
	registry.Schema(reflect.TypeFor[api.SessionEvent](), true, "SessionEvent")
	registry.Map()["SessionEvent"] = sessionEventSchema(registry)
	registry.Schema(reflect.TypeFor[api.RunStreamEvent](), true, "RunStreamEvent")
	registry.Map()["RunStreamEvent"] = runStreamEventSchema(registry)
	customizeMessageSchemas(registry)
}

func (s *Server) registerJSON(method, path, operationID, summary string, request any, status int, response any, handler http.HandlerFunc) {
	s.registerJSONWithParameters(method, path, operationID, summary, request, status, response, nil, handler)
}

func (s *Server) registerJSONWithParameters(method, path, operationID, summary string, request any, status int, response any, parameters []*huma.Param, handler http.HandlerFunc) {
	op := &huma.Operation{
		Method:      method,
		Path:        path,
		OperationID: operationID,
		Summary:     summary,
		Parameters:  append(operationParameters(path), parameters...),
		Responses: map[string]*huma.Response{
			strconv.Itoa(status): jsonResponse(http.StatusText(status), schemaFor(s.protocol, response)),
			"default":            jsonResponse("Request failed", schemaFor(s.protocol, api.ErrorResponse{})),
		},
	}
	setOperationSecurity(op)
	if request != nil {
		op.RequestBody = &huma.RequestBody{
			Required: true,
			Content: map[string]*huma.MediaType{
				"application/json": {Schema: schemaFor(s.protocol, request)},
			},
		}
	}
	s.registerOperation(op, handler)
}

func (s *Server) registerJSONStatuses(method, path, operationID, summary string, request any, responses map[int]any, handler http.HandlerFunc) {
	op := &huma.Operation{Method: method, Path: path, OperationID: operationID, Summary: summary, Parameters: operationParameters(path), Responses: map[string]*huma.Response{
		"default": jsonResponse("Request failed", schemaFor(s.protocol, api.ErrorResponse{})),
	}}
	setOperationSecurity(op)
	for status, response := range responses {
		op.Responses[strconv.Itoa(status)] = jsonResponse(http.StatusText(status), schemaFor(s.protocol, response))
	}
	if request != nil {
		op.RequestBody = &huma.RequestBody{Required: true, Content: map[string]*huma.MediaType{"application/json": {Schema: schemaFor(s.protocol, request)}}}
	}
	s.registerOperation(op, handler)
}

func (s *Server) registerErrorOnly(method, path, operationID, summary string, handler http.HandlerFunc) {
	op := &huma.Operation{Method: method, Path: path, OperationID: operationID, Summary: summary, Parameters: operationParameters(path), Responses: map[string]*huma.Response{
		"default": jsonResponse("Request failed", schemaFor(s.protocol, api.ErrorResponse{})),
	}}
	setOperationSecurity(op)
	s.registerOperation(op, handler)
}

func (s *Server) registerBinary(method, path, operationID, summary, contentType string, handler http.HandlerFunc) {
	op := &huma.Operation{Method: method, Path: path, OperationID: operationID, Summary: summary, Parameters: operationParameters(path), Responses: map[string]*huma.Response{
		"200":     {Description: http.StatusText(http.StatusOK), Content: map[string]*huma.MediaType{contentType: {Schema: &huma.Schema{Type: huma.TypeString, Format: "binary"}}}},
		"default": jsonResponse("Request failed", schemaFor(s.protocol, api.ErrorResponse{})),
	}}
	setOperationSecurity(op)
	s.registerOperation(op, handler)
}

func setOperationSecurity(op *huma.Operation) {
	switch op.Path {
	case "/health":
		op.Security = []map[string][]string{}
	default:
		op.Security = []map[string][]string{{"basicAuth": {}}}
	}
}

func (s *Server) registerSessionEvents() {
	op := &huma.Operation{
		Method:      http.MethodGet,
		Path:        "/sessions/{id}/events",
		OperationID: "streamSessionEvents",
		Summary:     "Stream session events",
		Parameters: append(operationParameters("/sessions/{id}/events"),
			queryParameter("after", huma.TypeInteger, "Exclusive durable event cursor"),
			queryParameter("limit", huma.TypeInteger, "Maximum replay page size"),
			&huma.Param{Name: "Last-Event-ID", In: "header", Description: "Exclusive durable event cursor", Schema: &huma.Schema{Type: huma.TypeInteger, Format: "int64"}},
		),
		Responses: map[string]*huma.Response{
			"200":     streamResponse("Session event stream", &huma.Schema{Type: huma.TypeArray, Items: &huma.Schema{Ref: "#/components/schemas/SessionEvent"}}),
			"default": jsonResponse("Request failed", schemaFor(s.protocol, api.ErrorResponse{})),
		},
	}
	setOperationSecurity(op)
	s.registerOperation(op, s.handleSessionEvents)
}

func (s *Server) registerRunStream() {
	op := &huma.Operation{
		Method:      http.MethodPost,
		Path:        "/run",
		OperationID: "runAgent",
		Summary:     "Run one ephemeral agent turn",
		RequestBody: &huma.RequestBody{Required: true, Content: map[string]*huma.MediaType{
			"application/json": {Schema: schemaFor(s.protocol, api.RunRequest{})},
		}},
		Responses: map[string]*huma.Response{
			"200":     streamResponse("Run event stream", &huma.Schema{Type: huma.TypeArray, Items: &huma.Schema{Ref: "#/components/schemas/RunStreamEvent"}}),
			"default": jsonResponse("Request failed", schemaFor(s.protocol, api.ErrorResponse{})),
		},
	}
	setOperationSecurity(op)
	s.registerOperation(op, s.handleRun)
}

func (s *Server) registerOperation(op *huma.Operation, handler http.HandlerFunc) {
	s.protocol.OpenAPI().AddOperation(op)
	s.protocol.Adapter().Handle(op, func(ctx huma.Context) {
		r, w := humachi.Unwrap(ctx)
		handler(w, r)
	})
}

func schemaFor(protocol huma.API, value any) *huma.Schema {
	return protocol.OpenAPI().Components.Schemas.Schema(reflect.TypeOf(value), true, "")
}

func jsonResponse(description string, schema *huma.Schema) *huma.Response {
	return &huma.Response{Description: description, Content: map[string]*huma.MediaType{
		"application/json": {Schema: schema},
	}}
}

func streamResponse(description string, schema *huma.Schema) *huma.Response {
	return &huma.Response{Description: description, Content: map[string]*huma.MediaType{
		"text/event-stream": {Schema: schema},
	}}
}

func operationParameters(path string) []*huma.Param {
	params := []*huma.Param{{
		Name:        "X-Wingman-Client",
		In:          "header",
		Description: "Client identity for resource attribution and scoping",
		Schema:      &huma.Schema{Type: huma.TypeString},
	}}
	for _, part := range strings.Split(path, "/") {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			params = append(params, &huma.Param{
				Name:     strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}"),
				In:       "path",
				Required: true,
				Schema:   &huma.Schema{Type: huma.TypeString},
			})
		}
	}
	return params
}

func queryParameter(name, typ, description string) *huma.Param {
	return &huma.Param{Name: name, In: "query", Description: description, Schema: &huma.Schema{Type: typ}}
}

type eventVariant struct {
	types []string
	data  any
}

func sessionEventSchema(registry huma.Registry) *huma.Schema {
	return discriminatedEventSchema(registry, "Session event", []eventVariant{
		{[]string{"session.run.queued", "session.run.started", "session.run.completed", "session.run.failed", "session.run.aborted"}, api.RunEventData{}},
		{[]string{"session.step.started", "session.step.completed"}, api.StepEventData{}},
		{[]string{"session.text.delta", "session.reasoning.delta", "session.tool.input.delta"}, api.ContentDeltaEventData{}},
		{[]string{"session.text.completed", "session.reasoning.completed"}, api.ContentCompletedEventData{}},
		{[]string{"session.message.created"}, api.MessageCreatedEventData{}},
		{[]string{"session.tool.called", "session.tool.updated", "session.tool.progress", "session.tool.completed", "session.tool.failed"}, api.ToolEventData{}},
		{[]string{"session.permission.requested", "session.permission.resolved"}, api.PermissionEventData{}},
		{[]string{"session.structured_output.completed"}, api.StructuredOutputEventData{}},
		{[]string{"session.events.synchronized"}, api.EventsSynchronizedEventData{}},
		{[]string{"session.events.resync_required"}, api.EventsResyncRequiredEventData{}},
	}, true)
}

func runStreamEventSchema(registry huma.Registry) *huma.Schema {
	return discriminatedEventSchema(registry, "Run stream event", []eventVariant{
		{[]string{"iteration_start"}, api.RunIterationStartEventData{}},
		{[]string{"iteration_end"}, api.RunIterationEndEventData{}},
		{[]string{"message"}, api.RunMessageEventData{}},
		{[]string{"tool_proposed"}, api.RunToolProposedEventData{}},
		{[]string{"tool_authorized"}, api.RunToolAuthorizedEventData{}},
		{[]string{"tool_start"}, api.RunToolStartEventData{}},
		{[]string{"tool_progress"}, api.RunToolProgressEventData{}},
		{[]string{"tool_end"}, api.RunToolEndEventData{}},
		{[]string{"stream_part"}, api.RunStreamPartEventData{}},
		{[]string{"compaction", "context_transformed"}, api.RunContextTransformedEventData{}},
		{[]string{"error"}, api.RunErrorEventData{}},
		{[]string{"structured_output"}, api.RunStructuredOutputEventData{}},
		{[]string{"done"}, api.RunDoneEventData{}},
	}, false)
}

func discriminatedEventSchema(registry huma.Registry, title string, variants []eventVariant, session bool) *huma.Schema {
	oneOf := make([]*huma.Schema, 0, len(variants))
	for _, variant := range variants {
		properties := map[string]*huma.Schema{
			"type": {Type: huma.TypeString, Enum: stringsToAny(variant.types)},
			"data": registry.Schema(reflect.TypeOf(variant.data), true, ""),
		}
		required := []string{"type", "data"}
		if session {
			properties["id"] = &huma.Schema{Type: huma.TypeString}
			properties["schema_version"] = &huma.Schema{Type: huma.TypeInteger, Const: api.CurrentSessionEventSchemaVersion}
			properties["time"] = &huma.Schema{Type: huma.TypeString}
			properties["cursor"] = registry.Schema(reflect.TypeFor[api.SessionEventCursor](), true, "")
			required = append(required, "id", "schema_version")
		} else {
			properties["version"] = &huma.Schema{Type: huma.TypeInteger}
			required = append(required, "version")
		}
		oneOf = append(oneOf, &huma.Schema{Type: huma.TypeObject, Properties: properties, Required: required})
	}
	return &huma.Schema{
		Title:         title,
		OneOf:         oneOf,
		Discriminator: &huma.Discriminator{PropertyName: "type"},
	}
}

func stringsToAny(values []string) []any {
	result := make([]any, len(values))
	for i, value := range values {
		result[i] = value
	}
	return result
}

func customizeMessageSchemas(registry huma.Registry) {
	types := []struct {
		name string
		typ  reflect.Type
		kind string
	}{
		{"TextPart", reflect.TypeFor[models.TextPart](), "text"},
		{"ImagePart", reflect.TypeFor[models.ImagePart](), "image"},
		{"ReasoningPart", reflect.TypeFor[models.ReasoningPart](), "reasoning"},
		{"ToolPart", reflect.TypeFor[models.ToolPart](), "tool"},
		{"ToolCallPart", reflect.TypeFor[models.ToolCallPart](), "tool_call"},
		{"ToolResultPart", reflect.TypeFor[models.ToolResultPart](), "tool_result"},
	}

	parts := make([]*huma.Schema, 0, len(types)+1)
	for _, part := range types {
		registry.Schema(part.typ, true, part.name)
		schema := registry.Map()[part.name]
		schema.Properties["type"] = &huma.Schema{Type: huma.TypeString, Const: part.kind}
		schema.Required = append(schema.Required, "type")
		parts = append(parts, &huma.Schema{Ref: "#/components/schemas/" + part.name})
	}
	parts = append(parts, &huma.Schema{
		Type:                 huma.TypeObject,
		Properties:           map[string]*huma.Schema{"type": {Type: huma.TypeString}},
		Required:             []string{"type"},
		AdditionalProperties: true,
	})
	registry.Map()["Part"] = &huma.Schema{Title: "Message part", AnyOf: parts}

	toolResult := registry.Map()["ToolResultPart"]
	toolResult.Properties["output"] = &huma.Schema{Type: huma.TypeArray, Items: &huma.Schema{Ref: "#/components/schemas/Part"}}

	registry.Schema(reflect.TypeFor[models.Message](), true, "Message")
	message := registry.Map()["Message"]
	message.Properties["content"] = &huma.Schema{Type: huma.TypeArray, Items: &huma.Schema{Ref: "#/components/schemas/Part"}}
}
