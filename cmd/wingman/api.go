package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/urfave/cli/v3"
)

var apiMethods = map[string]struct{}{
	http.MethodDelete:  {},
	http.MethodGet:     {},
	http.MethodHead:    {},
	http.MethodOptions: {},
	http.MethodPatch:   {},
	http.MethodPost:    {},
	http.MethodPut:     {},
}

type openAPIDocument struct {
	Paths map[string]map[string]openAPIOperation `json:"paths"`
}

type openAPIOperation struct {
	OperationID string `json:"operationId"`
}

type apiRequest struct {
	Method string
	Path   string
}

type apiStatusError struct {
	status string
}

func (e *apiStatusError) Error() string {
	return "Wingman API returned " + e.status
}

func apiCommand() *cli.Command {
	return &cli.Command{
		Name:                      "api",
		Usage:                     "Make an authenticated request to the managed daemon",
		ArgsUsage:                 "<operation-id> | <method> <path>",
		DisableSliceFlagSeparator: true,
		Arguments: []cli.Argument{
			&cli.StringArgs{Name: "request", Min: 1, Max: 2},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "data", Aliases: []string{"d"}, Usage: "Request body"},
			&cli.StringSliceFlag{Name: "header", Aliases: []string{"H"}, Usage: "Request header in name:value form"},
			&cli.StringMapFlag{Name: "param", Usage: "OpenAPI path or query parameter in name=value form"},
		},
		Action: runAPI,
	}
}

func runAPI(ctx context.Context, cmd *cli.Command) error {
	client, err := discoverManagedDaemon(ctx)
	if err != nil {
		return err
	}
	return runAPIWithClient(ctx, cmd, client)
}

func runAPIWithClient(ctx context.Context, cmd *cli.Command, client rawDaemonClient) error {
	request, err := resolveAPIRequest(ctx, client, cmd.StringArgs("request"), cmd.StringMap("param"))
	if err != nil {
		return err
	}

	headers := make(http.Header)
	for _, value := range cmd.StringSlice("header") {
		name, headerValue, ok := strings.Cut(value, ":")
		if !ok || strings.TrimSpace(name) == "" {
			return fmt.Errorf("invalid header, expected name:value: %s", value)
		}
		headers.Set(strings.TrimSpace(name), strings.TrimSpace(headerValue))
	}
	var body io.Reader
	if cmd.IsSet("data") {
		body = strings.NewReader(cmd.String("data"))
		if headers.Get("Content-Type") == "" {
			headers.Set("Content-Type", "application/json")
		}
	}

	response, err := client.Do(ctx, request.Method, request.Path, body, headers)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if _, err := io.Copy(commandWriter(cmd), response.Body); err != nil {
		return fmt.Errorf("read Wingman API response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &apiStatusError{status: response.Status}
	}
	return nil
}

type rawDaemonClient interface {
	Do(context.Context, string, string, io.Reader, http.Header) (*http.Response, error)
}

func resolveAPIRequest(ctx context.Context, client rawDaemonClient, input []string, params map[string]string) (apiRequest, error) {
	if request, ok := rawAPIRequest(input); ok {
		return request, nil
	}
	if len(input) != 1 {
		return apiRequest{}, errors.New("expected an operation ID or an HTTP method and path")
	}

	response, err := client.Do(ctx, http.MethodGet, "/openapi.json", nil, http.Header{"Accept": []string{"application/json"}})
	if err != nil {
		return apiRequest{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return apiRequest{}, fmt.Errorf("load OpenAPI document: HTTP %d", response.StatusCode)
	}
	var document openAPIDocument
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&document); err != nil {
		return apiRequest{}, fmt.Errorf("decode OpenAPI document: %w", err)
	}
	return resolveAPIOperation(document, input[0], params)
}

func rawAPIRequest(input []string) (apiRequest, bool) {
	if len(input) != 2 || !strings.HasPrefix(input[1], "/") {
		return apiRequest{}, false
	}
	method := strings.ToUpper(input[0])
	if _, ok := apiMethods[method]; !ok {
		return apiRequest{}, false
	}
	return apiRequest{Method: method, Path: input[1]}, true
}

func resolveAPIOperation(document openAPIDocument, operationID string, params map[string]string) (apiRequest, error) {
	for path, operations := range document.Paths {
		for method, operation := range operations {
			upperMethod := strings.ToUpper(method)
			if _, ok := apiMethods[upperMethod]; !ok || operation.OperationID != operationID {
				continue
			}
			resolved, err := interpolateAPIPath(path, params)
			if err != nil {
				return apiRequest{}, err
			}
			return apiRequest{Method: upperMethod, Path: resolved}, nil
		}
	}
	return apiRequest{}, fmt.Errorf("operation not found: %s", operationID)
}

func interpolateAPIPath(path string, params map[string]string) (string, error) {
	used := make(map[string]struct{})
	var missing string
	resolved := path
	for {
		start := strings.IndexByte(resolved, '{')
		if start < 0 {
			break
		}
		endOffset := strings.IndexByte(resolved[start:], '}')
		if endOffset < 0 {
			break
		}
		end := start + endOffset
		name := resolved[start+1 : end]
		value, ok := params[name]
		if !ok {
			missing = name
			break
		}
		used[name] = struct{}{}
		resolved = resolved[:start] + url.PathEscape(value) + resolved[end+1:]
	}
	if missing != "" {
		return "", fmt.Errorf("missing path parameter: %s", missing)
	}
	query := make(url.Values)
	for name, value := range params {
		if _, ok := used[name]; !ok {
			query.Set(name, value)
		}
	}
	if encoded := query.Encode(); encoded != "" {
		resolved += "?" + encoded
	}
	return resolved, nil
}
